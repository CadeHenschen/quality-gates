#!/usr/bin/env node
// Walks a directory's .ts/.tsx/.js/.jsx source (skipping node_modules,
// dotdirs, .d.ts, and *.test.*/*.spec.*) and prints one JSON object per
// function/method/arrow-function to stdout, with cyclomatic complexity and
// its line range. Requires the *target* repo's own installed `typescript`
// package (resolved via require.resolve with the target dir in the search
// path) — crap-metric doesn't bundle or install TypeScript itself. This
// also backs plain-JavaScript analysis: TypeScript's classic parser reads
// .js/.jsx natively (the same engine editors use for JS IntelliSense), so
// a JS-only target repo still needs `typescript` (or the TS7 compat
// package below) as a devDependency purely to get that parser — see
// README/CLAUDE.md.
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

// TypeScript 7's package entrypoint is the new native-compiler (Go-based,
// "typescript-go"/tsgo) preview: as of 7.0 it drops the classic
// ts.createSourceFile compiler API from the main entry point entirely,
// exposing only explicitly-unstable `typescript/unstable/*` subpaths — a
// stable programmatic API isn't promised until 7.1. Every other AST-based
// tool (typescript-eslint, ts-morph, ts-jest, Angular, Vue, Svelte) is in
// the same boat and hasn't moved to it yet either.
//
// Rather than block on that, fall back to `@typescript/typescript6` —
// Microsoft's own official compat package, which re-exports the full
// classic API (verified: `require('@typescript/typescript6').createSourceFile`
// is a real function) specifically so tools like this one keep working for
// repos that jump to TS7 before the ecosystem catches up.
let ts = resolveClassicTs('typescript');
if (!ts) {
  ts = resolveClassicTs('@typescript/typescript6');
}
if (!ts) {
  console.error(
    'could not get a classic TypeScript compiler API (ts.createSourceFile) from ' +
      dir +
      ' — either "typescript" isn\'t installed there, or it resolved to TypeScript 7+ ' +
      '(no classic API) with no "@typescript/typescript6" fallback installed either. ' +
      'Add "@typescript/typescript6" as a devDependency there to analyze a TS7 repo.'
  );
  process.exit(2);
}

function walk(d, out) {
  for (const entry of fs.readdirSync(d, { withFileTypes: true })) {
    if (entry.name === 'node_modules' || entry.name.startsWith('.')) continue;
    const full = path.join(d, entry.name);
    if (entry.isDirectory()) {
      walk(full, out);
    } else if (
      /\.(ts|tsx|js|jsx)$/.test(entry.name) &&
      !/\.d\.ts$/.test(entry.name) &&
      !/\.(test|spec)\.(ts|tsx|js|jsx)$/.test(entry.name)
    ) {
      out.push(full);
    }
  }
}

function complexityOf(body) {
  let c = 1;
  (function visit(n) {
    switch (n.kind) {
      case ts.SyntaxKind.IfStatement:
      case ts.SyntaxKind.ForStatement:
      case ts.SyntaxKind.ForInStatement:
      case ts.SyntaxKind.ForOfStatement:
      case ts.SyntaxKind.WhileStatement:
      case ts.SyntaxKind.DoStatement:
      case ts.SyntaxKind.CaseClause:
      case ts.SyntaxKind.CatchClause:
      case ts.SyntaxKind.ConditionalExpression:
        c++;
        break;
      case ts.SyntaxKind.BinaryExpression:
        if (
          n.operatorToken.kind === ts.SyntaxKind.AmpersandAmpersandToken ||
          n.operatorToken.kind === ts.SyntaxKind.BarBarToken ||
          n.operatorToken.kind === ts.SyntaxKind.QuestionQuestionToken
        ) {
          c++;
        }
        break;
    }
    ts.forEachChild(n, visit);
  })(body);
  return c;
}

// nestingDepthOf mirrors the Go analyzer's algorithm (see
// internal/analyzers/golang/size.go): each control-flow block adds one
// level, but a chained `else if` does NOT nest deeper than its `if` — it
// reads like a switch's cases, the same reasoning size-metric exists for.
// Nested function expressions/arrow functions are opaque here (their own
// internal nesting isn't walked), the safe direction for a generous gate.
function nestingDepthOf(body) {
  function stmtDepth(node, depth) {
    switch (node.kind) {
      case ts.SyntaxKind.IfStatement:
        return ifDepth(node, depth);
      case ts.SyntaxKind.ForStatement:
      case ts.SyntaxKind.ForInStatement:
      case ts.SyntaxKind.ForOfStatement:
      case ts.SyntaxKind.WhileStatement:
      case ts.SyntaxKind.DoStatement:
        return blockDepth(node.statement, depth + 1);
      case ts.SyntaxKind.SwitchStatement:
        return clausesDepth(node.caseBlock.clauses, depth);
      case ts.SyntaxKind.TryStatement: {
        let m = blockDepth(node.tryBlock, depth + 1);
        if (node.catchClause) m = Math.max(m, blockDepth(node.catchClause.block, depth + 1));
        if (node.finallyBlock) m = Math.max(m, blockDepth(node.finallyBlock, depth + 1));
        return m;
      }
      case ts.SyntaxKind.Block:
        return blockDepth(node, depth + 1);
      default:
        return depth;
    }
  }
  // blockDepth evaluates a statement position (a real Block, or a single
  // braceless statement, e.g. `if (x) foo();`) whose own contents sit at
  // depth — either walks the block's statements at that depth, or (no
  // braces) treats the lone statement as if it were the sole entry of one.
  function blockDepth(node, depth) {
    if (!node) return depth;
    if (node.kind === ts.SyntaxKind.Block) {
      let m = depth;
      for (const s of node.statements) {
        m = Math.max(m, stmtDepth(s, depth));
      }
      return m;
    }
    return stmtDepth(node, depth);
  }
  function ifDepth(node, depth) {
    let m = blockDepth(node.thenStatement, depth + 1);
    if (node.elseStatement) {
      if (node.elseStatement.kind === ts.SyntaxKind.IfStatement) {
        m = Math.max(m, ifDepth(node.elseStatement, depth)); // "else if": not deeper
      } else {
        m = Math.max(m, blockDepth(node.elseStatement, depth + 1));
      }
    }
    return m;
  }
  function clausesDepth(clauses, depth) {
    let m = depth;
    for (const c of clauses) {
      let cm = depth + 1;
      for (const s of c.statements) {
        cm = Math.max(cm, stmtDepth(s, depth + 1));
      }
      m = Math.max(m, cm);
    }
    return m;
  }
  return blockDepth(body, 0);
}

function paramCountOf(node) {
  return node.parameters ? node.parameters.length : 0;
}

function nameOf(node, sourceFile) {
  if (node.name) return node.name.getText(sourceFile);
  const p = node.parent;
  if (p) {
    if ((ts.isVariableDeclaration(p) || ts.isPropertyAssignment(p) || ts.isPropertyDeclaration(p)) && p.name) {
      return p.name.getText(sourceFile);
    }
  }
  return '<anonymous>';
}

const scriptKindByExt = {
  '.ts': ts.ScriptKind.TS,
  '.tsx': ts.ScriptKind.TSX,
  '.js': ts.ScriptKind.JS,
  '.jsx': ts.ScriptKind.JSX,
};

const files = [];
walk(dir, files);

const results = [];
for (const file of files) {
  const text = fs.readFileSync(file, 'utf8');
  const sourceFile = ts.createSourceFile(
    file,
    text,
    ts.ScriptTarget.Latest,
    true,
    scriptKindByExt[path.extname(file)]
  );
  const fileLines = text.length === 0 ? 0 : text.split('\n').length - (text.endsWith('\n') ? 1 : 0);

  (function visit(node) {
    if (
      ts.isFunctionDeclaration(node) ||
      ts.isMethodDeclaration(node) ||
      ts.isArrowFunction(node) ||
      ts.isFunctionExpression(node)
    ) {
      if (node.body) {
        const start = sourceFile.getLineAndCharacterOfPosition(node.getStart(sourceFile)).line + 1;
        const end = sourceFile.getLineAndCharacterOfPosition(node.getEnd()).line + 1;
        results.push({
          file: path.resolve(file),
          name: nameOf(node, sourceFile),
          start_line: start,
          end_line: end,
          complexity: complexityOf(node.body),
          param_count: paramCountOf(node),
          max_nesting_depth: nestingDepthOf(node.body),
          file_lines: fileLines,
        });
      }
    }
    ts.forEachChild(node, visit);
  })(sourceFile);
}

process.stdout.write(JSON.stringify(results));
