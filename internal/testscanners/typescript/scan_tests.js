#!/usr/bin/env node
// Walks a directory's *.test.* / *.spec.* TS/JS files (skipping
// node_modules and dotdirs) and prints one JSON object per test/suite to
// stdout: assertion counts, and whether it's skipped / focused / leaks a
// temp dir. Needs the *target* repo's own installed `typescript` package
// (same resolution and TS7 fallback as internal/analyzers/typescript's
// complexity.js — see CLAUDE.md).
'use strict';

const path = require('path');
const fs = require('fs');

const dir = process.cwd();

function resolveClassicTs(pkg) {
  try {
    const pkgPath = require.resolve(pkg, { paths: [dir, process.cwd()] });
    const mod = require(pkgPath);
    return typeof mod.createSourceFile === 'function' ? mod : null;
  } catch (e) {
    return null;
  }
}

let ts = resolveClassicTs('typescript') || resolveClassicTs('@typescript/typescript6');
if (!ts) {
  console.error(
    'could not get a classic TypeScript compiler API (ts.createSourceFile) from ' +
      dir +
      ' — either "typescript" isn\'t installed there, or it resolved to TypeScript 7+ ' +
      '(no classic API) with no "@typescript/typescript6" fallback installed either. ' +
      'Add "@typescript/typescript6" as a devDependency there to scan a TS7 repo.'
  );
  process.exit(2);
}

function walk(d, out) {
  for (const entry of fs.readdirSync(d, { withFileTypes: true })) {
    if (entry.name === 'node_modules' || entry.name.startsWith('.')) continue;
    const full = path.join(d, entry.name);
    if (entry.isDirectory()) {
      walk(full, out);
    } else if (/\.(test|spec)\.(ts|tsx|js|jsx)$/.test(entry.name)) {
      out.push(full);
    }
  }
}

const scriptKindByExt = {
  '.ts': ts.ScriptKind.TS,
  '.tsx': ts.ScriptKind.TSX,
  '.js': ts.ScriptKind.JS,
  '.jsx': ts.ScriptKind.JSX,
};

const TEST_ROOTS = new Set(['it', 'test', 'fit', 'xit', 'xtest']);
// Modifiers that keep `test.<mod>(...)` a *test declaration*. Anything else
// on a test root — Playwright's test.beforeAll/afterAll/beforeEach/afterEach
// hooks, test.step(...) inside a test, test.use/setTimeout/slow/info — takes a
// callback too, but isn't a test: recording one would report a hook or step
// as a test with no assertions.
const TEST_MODS = new Set(['skip', 'only', 'todo', 'fixme', 'fails', 'failing', 'each', 'concurrent', 'sequential', 'runIf', 'skipIf', 'for']);
const SUITE_ROOTS = new Set(['describe', 'fdescribe', 'xdescribe']);
const ASSERT_HELPER = /^(assert|expect|verify|check|ensure|validate|should)/i;
const CLEANUP_CALLS = new Set(['rm', 'rmSync', 'rmdir', 'rmdirSync', 'unlink', 'unlinkSync', 'remove', 'removeSync', 'emptyDir', 'emptyDirSync']);
const TEMP_CALLS = new Set(['mkdtemp', 'mkdtempSync']);
const HOOKS = new Set(['afterEach', 'afterAll', 'after']);
const INTERACTION_MATCHER = /^(toHaveBeen|toBeCalled|toHaveReturned|toHaveLastReturned|toHaveNthReturned|toHaveLastCalled|toHaveNthCalled)/;

// calleeChain flattens `test.skip.each(table)` / `it.only` / `xit` into its
// root identifier and property names, looking through the `.each(...)(...)`
// call and `.each\`...\`` tagged-template forms.
function calleeChain(expr) {
  const mods = [];
  let cur = expr;
  for (;;) {
    if (ts.isCallExpression(cur)) cur = cur.expression;
    else if (ts.isTaggedTemplateExpression(cur)) cur = cur.tag;
    else if (ts.isPropertyAccessExpression(cur)) {
      mods.unshift(cur.name.text);
      cur = cur.expression;
    } else break;
  }
  return ts.isIdentifier(cur) ? { root: cur.text, mods } : null;
}

function matcherOf(callNode) {
  // expect(x).not.toHaveBeenCalled() → climb from the expect(...) call
  // through property accesses to the first `toXxx` matcher name.
  let cur = callNode;
  while (cur.parent && ts.isPropertyAccessExpression(cur.parent) && cur.parent.expression === cur) {
    cur = cur.parent;
    if (cur.name.text.startsWith('to')) return cur.name.text;
  }
  return '';
}

function lastName(expr) {
  if (ts.isIdentifier(expr)) return expr.text;
  if (ts.isPropertyAccessExpression(expr)) return expr.name.text;
  return '';
}

function analyzeBody(body) {
  let assertions = 0;
  let interactions = 0;
  let createdTemp = false;
  let cleaned = false;
  (function visit(n) {
    if (ts.isCallExpression(n)) {
      const callee = n.expression;
      const name = lastName(callee);
      const base = ts.isPropertyAccessExpression(callee) && ts.isIdentifier(callee.expression) ? callee.expression.text : '';
      if (ts.isIdentifier(callee) && (name === 'expect' || name === 'assert')) {
        // expect(x).matcher() / assert(x): one assertion per expect(...) call.
        assertions++;
        if (name === 'expect' && INTERACTION_MATCHER.test(matcherOf(n))) interactions++;
      } else if (base === 'assert') {
        assertions++; // assert.equal(...)
      } else if (base === 'expect') {
        // expect.assertions(n) / expect.hasAssertions(): declares intent, not a check itself.
      } else if (ASSERT_HELPER.test(name)) {
        assertions++; // supertest's .expect(200), assertFoo(...), verifyBar(...), ...
      } else if (TEMP_CALLS.has(name)) {
        createdTemp = true;
      } else if (CLEANUP_CALLS.has(name)) {
        cleaned = true;
      }
    }
    ts.forEachChild(n, visit);
  })(body);
  return { assertions, interactions, createdTemp, cleaned };
}

const files = [];
walk(dir, files);

const results = [];
for (const file of files) {
  const text = fs.readFileSync(file, 'utf8');
  const sf = ts.createSourceFile(file, text, ts.ScriptTarget.Latest, true, scriptKindByExt[path.extname(file)]);

  // A file-level afterEach/afterAll hook cleans up on the tests' behalf.
  let fileHasAfterHook = false;
  (function find(n) {
    if (ts.isCallExpression(n) && ts.isIdentifier(n.expression) && HOOKS.has(n.expression.text)) fileHasAfterHook = true;
    ts.forEachChild(n, find);
  })(sf);

  (function visit(n) {
    if (ts.isCallExpression(n)) {
      const chain = calleeChain(n.expression);
      const isSuite = chain && (SUITE_ROOTS.has(chain.root) || (chain.root === 'test' && chain.mods.includes('describe')));
      const isTest = chain && TEST_ROOTS.has(chain.root) && !isSuite && chain.mods.every((m) => TEST_MODS.has(m));
      const nameArg = n.arguments[0];
      const fnArg = n.arguments.find((a) => ts.isArrowFunction(a) || ts.isFunctionExpression(a));
      // The inner `.each(table)` call of `test.each(table)(name, fn)` has
      // neither a callback nor a string name, so it falls out here and only
      // the outer call is recorded.
      if (chain && (isSuite || isTest) && (fnArg || (nameArg && ts.isStringLiteralLike(nameArg)))) {
        const label = nameArg && (ts.isStringLiteralLike(nameArg) || ts.isTemplateExpression(nameArg)) ? nameArg.getText(sf).replace(/^['"`]|['"`]$/g, '') : '<anonymous>';
        const line = sf.getLineAndCharacterOfPosition(n.getStart(sf)).line + 1;
        const mods = chain.mods;
        const focused = mods.includes('only') || chain.root === 'fit' || chain.root === 'fdescribe';
        // skipIf/skip(cond) are conditional; .skip / xit / .todo / .fixme are not.
        const skipped = chain.root === 'xit' || chain.root === 'xtest' || chain.root === 'xdescribe' ||
          mods.includes('skip') || mods.includes('todo') || mods.includes('fixme');
        const base = {
          file: path.resolve(file),
          line,
          name: label,
          assertions: 0,
          interactions: 0,
          skipped,
          focused,
          expected_failure: mods.includes('failing') || mods.includes('fails'),
          uncleaned_temp: false,
        };
        if (isSuite) {
          if (skipped || focused) results.push({ ...base, suite: true });
        } else if (fnArg || skipped || focused) {
          const facts = fnArg ? analyzeBody(fnArg.body) : { assertions: 0, interactions: 0, createdTemp: false, cleaned: false };
          results.push({
            ...base,
            assertions: facts.assertions,
            interactions: facts.interactions,
            uncleaned_temp: facts.createdTemp && !facts.cleaned && !fileHasAfterHook,
          });
        }
      }
    }
    ts.forEachChild(n, visit);
  })(sf);
}

process.stdout.write(JSON.stringify(results));
