package typescript

import (
	"os"
	"path/filepath"
	"testing"

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
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
