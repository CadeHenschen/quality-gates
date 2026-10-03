package main

import (
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
