package main

import (
	"path/filepath"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/crap"
)

// LoadFiles/Matches themselves are covered by internal/ratchet's own
// tests — this only needs to cover the tool-specific filtering logic
// built on top of them.
func TestFilterFunctions(t *testing.T) {
	fns := []crap.Function{
		{File: "pages/Foo.tsx", Name: "Foo"},
		{File: "pages/Bar.tsx", Name: "Bar"},
	}
	dir := t.TempDir()
	set := map[string]bool{"pages/Foo.tsx": true}

	got := filterFunctions(fns, set, dir)
	if len(got) != 1 || got[0].Name != "Foo" {
		t.Errorf("filterFunctions = %+v, want only Foo", got)
	}
}

func TestFilterFileSizes(t *testing.T) {
	files := []crap.FileSize{
		{File: "types.go", Lines: 700},
		{File: "other.go", Lines: 800},
	}
	dir, err := filepath.Abs(filepath.Join("..", "..", "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	got := filterFileSizes(files, map[string]bool{"testdata/types.go": true}, dir)
	if len(got) != 1 || got[0].File != "types.go" {
		t.Errorf("filterFileSizes = %+v, want only types.go", got)
	}
}
