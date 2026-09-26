package python

import (
	"os"
	"path/filepath"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/cycle"
	"git.roost-r.com/cadeh/quality-gates/internal/importers"
)

func TestImportRealFixtureFindsCycle(t *testing.T) {
	g, n, err := Importer{}.Import(importers.Options{Dir: "../../../testdata/cycle/python"})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if n != 4 {
		t.Fatalf("files analyzed = %d, want 4", n)
	}

	// pkg/a.py <-> pkg/b.py is a real cycle: a imports b via "from .
	// import b", b imports a via "from .a import use_b".
	if !containsEdge(g, "pkg/a.py", "pkg/b.py") {
		t.Errorf("expected pkg/a.py -> pkg/b.py, got edges: %v", g.Edges)
	}
	if !containsEdge(g, "pkg/b.py", "pkg/a.py") {
		t.Errorf("expected pkg/b.py -> pkg/a.py, got edges: %v", g.Edges)
	}

	// standalone.py does a plain absolute "import pkg.a".
	if !containsEdge(g, "standalone.py", "pkg/a.py") {
		t.Errorf("expected standalone.py -> pkg/a.py, got edges: %v", g.Edges)
	}

	cycles := cycle.FindCycles(g)
	if len(cycles) != 1 {
		t.Fatalf("got %d cycles, want 1: %+v", len(cycles), cycles)
	}
	if len(cycles[0].Files) != 2 {
		t.Errorf("cycle files = %v, want exactly [pkg/a.py pkg/b.py]", cycles[0].Files)
	}
}

func TestResolveExternalImportsAreDropped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app.py", "import os\nimport sys\nfrom typing import List\n\ndef f(): pass\n")

	g, _, err := Importer{}.Import(importers.Options{Dir: dir})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(g.Edges["app.py"]) != 0 {
		t.Errorf("expected no edges (all imports are external), got: %v", g.Edges["app.py"])
	}
}

func TestNoCycleWhenOneWay(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.py", "import b\n")
	writeFile(t, dir, "b.py", "def f(): pass\n")

	g, n, err := Importer{}.Import(importers.Options{Dir: dir})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if n != 2 {
		t.Fatalf("files analyzed = %d, want 2", n)
	}
	if cycles := cycle.FindCycles(g); len(cycles) != 0 {
		t.Errorf("expected no cycles for a one-way import, got: %+v", cycles)
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

func FuzzResolveFileImports(f *testing.F) {
	f.Add("from . import b\n")
	f.Add("import b\n")
	f.Add("from ..x import y as z\n")
	f.Fuzz(func(t *testing.T, source string) {
		_, _, err := resolveSourceImports([]byte(source), "a.py", map[string]bool{"a.py": true, "b.py": true})
		if err != nil {
			t.Fatalf("resolveFileImports returned an unexpected filesystem error: %v", err)
		}
	})
}
