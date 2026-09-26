#!/usr/bin/env node
// Walks a directory's .ts/.tsx/.js/.jsx source (skipping node_modules,
// dotdirs, .d.ts, and *.test.*/*.spec.*) and prints one JSON object per
// file to stdout: {"file": ..., "tokens": [{"text": ..., "line": ...}, ...]}.
// Tokenizes via the *target* repo's own installed `typescript` package's
// scanner (classic API — same resolution + TS7 fallback as crap-metric's
// complexity.js; see that repo's CLAUDE.md for why the fallback exists).
// The scanner only distinguishes JSX-vs-not (not TS-vs-JS), so this same
// scanner already tokenizes plain .js/.jsx correctly once walked — see
// CLAUDE.md for why a JS-only target repo still needs `typescript`
// installed purely to get this scanner.
'use strict';

const path = require('path');
const fs = require('fs');

const dir = process.cwd();

function resolveClassicTs(pkg) {
  try {
    const pkgPath = require.resolve(pkg, { paths: [dir, process.cwd()] });
    const mod = require(pkgPath);
    return typeof mod.createScanner === 'function' ? mod : null;
  } catch (e) {
    return null;
  }
}

let ts = resolveClassicTs('typescript');
if (!ts) {
  ts = resolveClassicTs('@typescript/typescript6');
}
if (!ts) {
  console.error(
    'could not get a classic TypeScript compiler API (ts.createScanner) from ' +
      dir +
      ' — either "typescript" isn\'t installed there, or it resolved to TypeScript 7+ ' +
      '(no classic API) with no "@typescript/typescript6" fallback installed either.'
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

function lineStartsOf(text) {
  const starts = [0];
  for (let i = 0; i < text.length; i++) {
    if (text[i] === '\n') starts.push(i + 1);
  }
  return starts;
}

function tokenizeFile(file) {
  const text = fs.readFileSync(file, 'utf8');
  const lineStarts = lineStartsOf(text);
  let lineCursor = 0;

  function lineFor(offset) {
    while (lineCursor + 1 < lineStarts.length && lineStarts[lineCursor + 1] <= offset) {
      lineCursor++;
    }
    return lineCursor + 1;
  }

  const languageVariant = file.endsWith('x') ? ts.LanguageVariant.JSX : ts.LanguageVariant.Standard;
  const scanner = ts.createScanner(ts.ScriptTarget.Latest, /* skipTrivia */ true, languageVariant, text);

  const tokens = [];
  let kind = scanner.scan();
  while (kind !== ts.SyntaxKind.EndOfFileToken) {
    tokens.push({
      text: scanner.getTokenText(),
      line: lineFor(scanner.getTokenPos()),
    });
    kind = scanner.scan();
  }
  return tokens;
}

const files = [];
walk(dir, files);

const results = [];
for (const file of files) {
  const tokens = tokenizeFile(file);
  if (tokens.length > 0) {
    // Relative to dir, not the raw walked path — so e.g. `--dir
    // ../../src` reports "foo.ts", not "../../src/foo.ts" (which would
    // also break --only-files matching in the Go CLI, whose changed-file
    // list is relative to the repo root, not to wherever --dir's own
    // ".." components happen to point).
    results.push({ file: path.relative(dir, file), tokens });
  }
}

process.stdout.write(JSON.stringify(results));
