#!/usr/bin/env node
// Walks a directory's .ts/.tsx source (skipping node_modules, dotdirs,
// .d.ts, and *.test.ts/*.spec.ts) and prints one JSON object per
// function/method/arrow-function to stdout, with cyclomatic complexity and
// its line range. Requires the *target* repo's own installed `typescript`
// package (resolved via require.resolve with the target dir in the search
// path) — crap-metric doesn't bundle or install TypeScript itself.
'use strict';

const path = require('path');
const fs = require('fs');

const dir = process.argv[2];
if (!dir) {
  console.error('usage: complexity.js <dir>');
  process.exit(2);
}

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
      /\.(ts|tsx)$/.test(entry.name) &&
      !/\.d\.ts$/.test(entry.name) &&
      !/\.(test|spec)\.(ts|tsx)$/.test(entry.name)
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
    file.endsWith('x') ? ts.ScriptKind.TSX : ts.ScriptKind.TS
  );

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
        });
      }
    }
    ts.forEachChild(node, visit);
  })(sourceFile);
}

process.stdout.write(JSON.stringify(results));
