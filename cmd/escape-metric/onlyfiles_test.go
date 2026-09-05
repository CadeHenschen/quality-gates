package main

import (
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/escape"
)

// LoadFiles/Matches themselves are covered by internal/ratchet's own
// tests — this only needs to cover the tool-specific filtering logic
// built on top of them.
func TestFilterForRatchet(t *testing.T) {
	result := escape.Result{
		Hatches: []escape.Hatch{
			{File: "a.go", Line: 1, Pattern: "nolint"},
			{File: "b.go", Line: 2, Pattern: "nolint"},
		},
		TotalLines:  20,
		LinesByFile: map[string]int{"a.go": 10, "b.go": 10},
	}
	onlyFiles := map[string]bool{"a.go": true}

	hatches, lines := filterForRatchet(result, onlyFiles)
	if len(hatches) != 1 || hatches[0].File != "a.go" {
		t.Errorf("hatches = %+v, want only a.go's", hatches)
	}
	if lines != 10 {
		t.Errorf("lines = %d, want 10 (only a.go's own line count)", lines)
	}
}
