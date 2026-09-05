package main

import (
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/dupe"
)

// LoadFiles/Matches themselves are covered by internal/ratchet's own
// tests — this only needs to cover the tool-specific filtering logic
// built on top of them.
func TestFilterForRatchet(t *testing.T) {
	files := []dupe.FileTokens{
		{File: "a.ts", Tokens: toks("1", "2", "3", "4", "5")}, // 5 lines
		{File: "b.ts", Tokens: toks("1", "2", "3", "4", "5")}, // 5 lines
		{File: "c.ts", Tokens: toks("1", "2", "3", "4", "5")}, // 5 lines
	}
	clones := []dupe.Clone{
		{FileA: "a.ts", StartLineA: 1, EndLineA: 2, FileB: "b.ts", StartLineB: 1, EndLineB: 2}, // touches a, b
		{FileA: "b.ts", StartLineA: 3, EndLineA: 4, FileB: "c.ts", StartLineB: 3, EndLineB: 4}, // touches b, c
	}
	onlyFiles := map[string]bool{"a.ts": true} // only a.ts "changed"

	scopedFiles, scopedClones := filterForRatchet(files, clones, onlyFiles)

	if len(scopedFiles) != 1 || scopedFiles[0].File != "a.ts" {
		t.Errorf("scopedFiles = %+v, want only a.ts", scopedFiles)
	}
	// Only the first clone touches a.ts; the second (b.ts <-> c.ts)
	// doesn't touch any changed file and should be excluded.
	if len(scopedClones) != 1 || scopedClones[0].FileA != "a.ts" {
		t.Errorf("scopedClones = %+v, want only the a.ts<->b.ts clone", scopedClones)
	}
}

func toks(texts ...string) []dupe.Token {
	out := make([]dupe.Token, len(texts))
	for i, t := range texts {
		out[i] = dupe.Token{Text: t, Line: i + 1}
	}
	return out
}
