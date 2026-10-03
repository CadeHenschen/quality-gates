package typescript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/arch"
	"git.roost-r.com/cadeh/quality-gates/internal/cycle"
	"git.roost-r.com/cadeh/quality-gates/internal/importers"
)

func TestImportRealFixtureFindsCycle(t *testing.T) {
	g, n, err := Importer{}.Import(importers.Options{Dir: "../../../testdata/cycle/typescript"})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if n != 3 {
		t.Fatalf("files analyzed = %d, want 3", n)
	}

	if !containsEdge(g, "pkg/a.ts", "pkg/b.ts") {
		t.Errorf("expected pkg/a.ts -> pkg/b.ts, got: %v", g.Edges)
	}
	if !containsEdge(g, "pkg/b.ts", "pkg/a.ts") {
		t.Errorf("expected pkg/b.ts -> pkg/a.ts, got: %v", g.Edges)
	}
	if !containsEdge(g, "standalone.ts", "pkg/a.ts") {
		t.Errorf("expected standalone.ts -> pkg/a.ts (nested relative path), got: %v", g.Edges)
	}
	if containsEdge(g, "standalone.ts", "react") {
		t.Errorf("bare import 'react' should have been dropped as external, got: %v", g.Edges["standalone.ts"])
	}

	cycles := cycle.FindCycles(g)
	if len(cycles) != 1 || len(cycles[0].Files) != 2 {
		t.Fatalf("got %+v, want exactly one 2-file cycle", cycles)
	}
}

// TestImportJavaScriptFixtureFindsCycle verifies the same importer walks
// and resolves plain .js files, not just .ts — codeExtensions already
// lists .js/.jsx and resolution is pure regex/extension matching with no
// TS-specific logic, so this should already pass. Per CLAUDE.md's own
// rule for this tool, new resolution behavior must be checked against a
// real cycle fixture, not just reasoned about — this is that check for JS.
func TestImportJavaScriptFixtureFindsCycle(t *testing.T) {
	g, n, err := Importer{}.Import(importers.Options{Dir: "../../../testdata/cycle/javascript"})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if n != 3 {
		t.Fatalf("files analyzed = %d, want 3", n)
	}

	if !containsEdge(g, "pkg/a.js", "pkg/b.js") {
		t.Errorf("expected pkg/a.js -> pkg/b.js, got: %v", g.Edges)
	}
	if !containsEdge(g, "pkg/b.js", "pkg/a.js") {
		t.Errorf("expected pkg/b.js -> pkg/a.js, got: %v", g.Edges)
	}
	if !containsEdge(g, "standalone.js", "pkg/a.js") {
		t.Errorf("expected standalone.js -> pkg/a.js (nested relative path), got: %v", g.Edges)
	}
	if containsEdge(g, "standalone.js", "react") {
		t.Errorf("bare import 'react' should have been dropped as external, got: %v", g.Edges["standalone.js"])
	}

	cycles := cycle.FindCycles(g)
	if len(cycles) != 1 || len(cycles[0].Files) != 2 {
		t.Fatalf("got %+v, want exactly one 2-file cycle", cycles)
	}
}

// TestImportJavaScriptResolvesRequire verifies CommonJS require() — the
// form a plain-JS repo is at least as likely to use as ES import — resolves
// the same way import/export specifiers do.
func TestImportJavaScriptResolvesRequire(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.js", "const { helperB } = require(\"./b\");\nmodule.exports.useB = () => helperB();\n")
	writeFile(t, dir, "b.js", "module.exports.helperB = () => \"b\";\n")

	g, n, err := Importer{}.Import(importers.Options{Dir: dir})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if n != 2 {
		t.Fatalf("files analyzed = %d, want 2", n)
	}
	if !containsEdge(g, "a.js", "b.js") {
		t.Errorf("expected a.js -> b.js (require), got: %v", g.Edges)
	}
}

func TestResolveIndexFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "a.ts", `import { x } from "./sub";`)
	writeFile(t, filepath.Join(dir, "sub"), "index.ts", "export const x = 1;")

	g, _, err := Importer{}.Import(importers.Options{Dir: dir})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if !containsEdge(g, "a.ts", "sub/index.ts") {
		t.Errorf("expected a.ts -> sub/index.ts, got: %v", g.Edges)
	}
}

func TestBareAndAliasedImportsDropped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.ts", "import x from \"react\";\nimport y from \"@/lib/foo\";\n")

	g, _, err := Importer{}.Import(importers.Options{Dir: dir})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(g.Edges["a.ts"]) != 0 {
		t.Errorf("expected no edges (bare/aliased imports aren't resolved), got: %v", g.Edges["a.ts"])
	}
}

func TestConfiguredPathsResolveAliasesAndFindCycle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "tsconfig.json", `{
  // JSONC syntax is accepted by TypeScript configs.
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@app/*": ["src/*",],
    },
  },
}`)
	writeFile(t, dir, "src/a.ts", `import { b } from "@app/b"; export const a = b;`)
	writeFile(t, dir, "src/b/index.ts", `import { a } from "@app/a"; export const b = a;`)

	g, n, err := Importer{}.Import(importers.Options{Dir: dir})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if n != 2 {
		t.Fatalf("files analyzed = %d, want 2", n)
	}
	if !containsEdge(g, "src/a.ts", "src/b/index.ts") || !containsEdge(g, "src/b/index.ts", "src/a.ts") {
		t.Fatalf("alias edges = %v, want mutual src/a.ts and src/b/index.ts imports", g.Edges)
	}
	if cycles := cycle.FindCycles(g); len(cycles) != 1 {
		t.Fatalf("cycles = %+v, want one alias-based cycle", cycles)
	}
}

func TestConfiguredPathsTryFallbackTargetsAndLeaveUnmappedPackagesExternal(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "tsconfig.json", `{"compilerOptions":{"baseUrl":".","paths":{"@lib/*":["missing/*.ts","src/*.ts"],"@exact":["src/exact.ts"]}}}`)
	writeFile(t, dir, "main.ts", `import x from "@lib/value"; import y from "@exact"; import React from "react";`)
	writeFile(t, dir, "src/value.ts", "export default 1;\n")
	writeFile(t, dir, "src/exact.ts", "export default 2;\n")

	g, _, err := Importer{}.Import(importers.Options{Dir: dir})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if !containsEdge(g, "main.ts", "src/value.ts") {
		t.Errorf("expected fallback alias edge, got %v", g.Edges["main.ts"])
	}
	if !containsEdge(g, "main.ts", "src/exact.ts") {
		t.Errorf("expected exact extension-bearing alias edge, got %v", g.Edges["main.ts"])
	}
	if len(g.Unresolved) != 0 {
		t.Errorf("unresolved imports = %+v, want none", g.Unresolved)
	}
}

func TestConfiguredUnresolvedAliasIsReported(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "tsconfig.json", `{"compilerOptions":{"paths":{"@/*":["src/*"]}}}`)
	writeFile(t, dir, "main.ts", `import x from "@/missing"; import React from "react";`)

	g, _, err := Importer{}.Import(importers.Options{Dir: dir})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(g.Unresolved) != 1 || g.Unresolved[0].File != "main.ts" || g.Unresolved[0].Specifier != "@/missing" {
		t.Fatalf("unresolved = %+v, want the configured alias", g.Unresolved)
	}
}

func TestConfiguredPathsUseBaseURLAndPreferMostSpecificPattern(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "tsconfig.json", `{"compilerOptions":{"baseUrl":"packages/web","paths":{"@/*":["fallback/*"],"@feature/*":["features/*"]}}}`)
	writeFile(t, dir, "main.ts", `import x from "@feature/card";`)
	writeFile(t, dir, "packages/web/features/card.ts", "export default 1;\n")
	writeFile(t, dir, "packages/web/fallback/feature/card.ts", "export default 2;\n")

	g, _, err := Importer{}.Import(importers.Options{Dir: dir})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if !containsEdge(g, "main.ts", "packages/web/features/card.ts") {
		t.Fatalf("edges = %v, want most-specific alias target", g.Edges["main.ts"])
	}
}

func TestConfiguredAliasEdgeCanViolateArchitectureRule(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "tsconfig.json", `{"compilerOptions":{"baseUrl":".","paths":{"@infra/*":["src/infra/*"]}}}`)
	writeFile(t, dir, "src/domain/service.ts", `import { load } from "@infra/db"; export const service = load;`)
	writeFile(t, dir, "src/infra/db.ts", "export const load = () => 1;\n")

	g, _, err := Importer{}.Import(importers.Options{Dir: dir})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	rules, err := arch.Compile([]arch.Rule{{Name: "domain cannot import infra", From: "src/domain", Deny: []string{"src/infra"}}}, nil)
	if err != nil {
		t.Fatalf("Compile rules: %v", err)
	}
	violations := rules.Check(arch.EdgesFromFileGraph(g))
	if len(violations) != 1 || violations[0].Import != "src/infra/db.ts" {
		t.Fatalf("violations = %+v, want domain-to-infra alias violation", violations)
	}
}

func TestInvalidTSConfigFailsImport(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "tsconfig.json", `{"compilerOptions":{"paths":{"@/*":"src/*"}}}`)
	writeFile(t, dir, "main.ts", `import x from "@/value";`)

	if _, _, err := (Importer{}).Import(importers.Options{Dir: dir}); err == nil {
		t.Fatal("Import succeeded with invalid paths mapping; want config error")
	}
}

func TestUnterminatedTSConfigBlockCommentFailsImport(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "tsconfig.json", `{"compilerOptions":{}} /* never closed`)
	writeFile(t, dir, "main.ts", "export const x = 1;\n")

	if _, _, err := (Importer{}).Import(importers.Options{Dir: dir}); err == nil || !strings.Contains(err.Error(), "unterminated block comment") {
		t.Fatalf("Import error = %v, want unterminated comment error", err)
	}
}

func TestDTSFilesSkipped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.ts", "export const x = 1;")
	writeFile(t, dir, "a.d.ts", "export declare const x: number;")

	_, n, err := Importer{}.Import(importers.Options{Dir: dir})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if n != 1 {
		t.Errorf("files analyzed = %d, want 1 (a.d.ts should be skipped)", n)
	}
}

func containsEdge(g cycle.Graph, from, to string) bool {
	for _, e := range g.Edges[from] {
		if e == to {
			return true
		}
	}
	return false
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
