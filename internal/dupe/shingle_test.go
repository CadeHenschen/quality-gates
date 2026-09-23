package dupe

import "testing"

func toks(texts ...string) []Token {
	out := make([]Token, len(texts))
	for i, t := range texts {
		out[i] = Token{Text: t, Line: i + 1} // one token per line, for simple, readable expectations
	}
	return out
}

func TestFindCrossFileDuplicate(t *testing.T) {
	a := FileTokens{File: "a.go", Tokens: toks("func", "A", "(", "x", "int", ")", "{", "return", "x", "}")}
	b := FileTokens{File: "b.go", Tokens: toks("func", "B", "(", "x", "int", ")", "{", "return", "x", "}")}

	clones := Find([]FileTokens{a, b}, 5)
	if len(clones) != 1 {
		t.Fatalf("got %d clones, want 1: %+v", len(clones), clones)
	}

	c := clones[0]
	// "func"/"A" vs "func"/"B" differ, so the match starts at "(" (index 2).
	if c.FileA != "a.go" || c.StartLineA != 3 || c.EndLineA != 10 {
		t.Errorf("clone A side = %+v, want a.go lines 3-10", c)
	}
	if c.FileB != "b.go" || c.StartLineB != 3 || c.EndLineB != 10 {
		t.Errorf("clone B side = %+v, want b.go lines 3-10", c)
	}
	if c.Tokens != 8 {
		t.Errorf("Tokens = %d, want 8", c.Tokens)
	}
}

func TestFindNoDuplicateBelowMinTokens(t *testing.T) {
	a := FileTokens{File: "a.go", Tokens: toks("func", "A", "(", ")")}
	b := FileTokens{File: "b.go", Tokens: toks("func", "B", "(", ")")}

	clones := Find([]FileTokens{a, b}, 5) // longest possible match here is 2 tokens ("(",")")
	if len(clones) != 0 {
		t.Errorf("got %d clones, want 0 (below min-tokens): %+v", len(clones), clones)
	}
}

func TestFindNothingSharedAtAll(t *testing.T) {
	a := FileTokens{File: "a.go", Tokens: toks("package", "a", "func", "X", "(", ")")}
	b := FileTokens{File: "b.go", Tokens: toks("package", "b", "type", "Y", "struct", "{", "}")}

	clones := Find([]FileTokens{a, b}, 2)
	if len(clones) != 0 {
		t.Errorf("got %d clones, want 0: %+v", len(clones), clones)
	}
}

// TestFindSameFileBoundaryBleed documents a real, expected characteristic
// of raw-token (not AST/statement-boundary) matching: when two duplicated
// blocks in the same file are immediately followed by content that
// *also* happens to coincide (here, both functions are followed by
// "func" — the start of the next declaration), the match legitimately
// extends a token or two past what a human would call "the" duplicate,
// because that trailing content genuinely is identical token-for-token.
// This isn't a bug in the matcher; it's inherent to token-shingling
// without semantic boundaries (see README/CLAUDE.md) — asserted here so
// a future change to the algorithm doesn't silently alter this
// documented behavior without a human noticing.
func TestFindSameFileBoundaryBleed(t *testing.T) {
	f := FileTokens{File: "f.go", Tokens: toks(
		"func", "A", "(", "x", ")", "{", "return", "x", "}", // 0-8
		"func", "B", "(", "x", ")", "{", "return", "x", "}", // 9-17
		"func", "C", "(", ")", "{", "}", // 18-23: also starts with "func", extending the match by one token
	)}

	clones := Find([]FileTokens{f}, 5)
	if len(clones) != 1 {
		t.Fatalf("got %d clones, want 1: %+v", len(clones), clones)
	}

	c := clones[0]
	// True shared body is "(","x",")","{","return","x","}" (7 tokens, index
	// 2-8 vs 11-17) — but both are followed by "func" (the start of the
	// *next* function in each case), so the match bleeds one token further.
	if c.Tokens != 8 {
		t.Errorf("Tokens = %d, want 8 (7-token body + 1-token boundary bleed)", c.Tokens)
	}
}

// TestFindSameFilePreventsOverlap verifies the maxExtend cap: a same-file
// match is never allowed to grow long enough that its own range would
// touch or overlap the range of the occurrence it's matching against —
// which a naive unbounded extend() can do when a match sits close to its
// own duplicate (see shingle.go's maxExtend doc comment for how this was
// found).
func TestFindSameFilePreventsOverlap(t *testing.T) {
	// "X","X","X","X","X","X" repeated — every position matches every
	// other position trivially. Without the overlap cap, a match starting
	// at 0 could extend across the whole file, "overlapping" the position
	// it's supposedly distinct from.
	f := FileTokens{File: "f.go", Tokens: toks("X", "X", "X", "X", "X", "X", "X", "X")}

	clones := Find([]FileTokens{f}, 2)
	for _, c := range clones {
		if c.FileA == c.FileB && rangesOverlapForTest(c.StartLineA, c.EndLineA, c.StartLineB, c.EndLineB) {
			t.Errorf("clone has overlapping same-file ranges: %+v", c)
		}
	}
}

func rangesOverlapForTest(startA, endA, startB, endB int) bool {
	return startA <= endB && startB <= endA
}

func FuzzFind(f *testing.F) {
	f.Add("abcabc", "abcabc", 2)
	f.Add("", "", 0)
	f.Fuzz(func(t *testing.T, a, b string, minTokens int) {
		toTokens := func(text string) []Token {
			out := make([]Token, 0, len(text))
			for i, r := range text {
				out = append(out, Token{Text: string(r), Line: i + 1})
			}
			return out
		}
		clones := Find([]FileTokens{{File: "a", Tokens: toTokens(a)}, {File: "b", Tokens: toTokens(b)}}, minTokens)
		for _, clone := range clones {
			if clone.Tokens < 1 || clone.StartLineA > clone.EndLineA || clone.StartLineB > clone.EndLineB {
				t.Errorf("invalid clone from fuzz input: %+v", clone)
			}
		}
	})
}
