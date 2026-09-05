package dupe

import (
	"sort"
	"strings"
)

// occurrence is one place a given window of tokens appears.
type occurrence struct {
	file  int
	start int
}

// Find finds duplicate token blocks of at least minTokens tokens across
// files. For each file, left to right, it looks for the longest exact
// match starting at the current (not-yet-covered) position against any
// other not-yet-covered position (in this file or another), records it,
// marks both ranges covered, and jumps past the match — a greedy
// leftmost-longest strategy, the same shape real clone detectors (PMD-CPD,
// jscpd) use.
//
// Known limitation: a token block duplicated in 3+ places is only ever
// reported as one pair (the first match found); the other copies are left
// unmatched once their pair is covered. True N-way clone groups are a
// natural extension, not implemented here — most real duplication is
// pairwise, and the tool is about the numerical gate, not maximal
// analysis.
func Find(files []FileTokens, minTokens int) []Clone {
	if minTokens < 1 {
		minTokens = 1
	}

	buckets := map[string][]occurrence{}
	for fi, f := range files {
		for start := 0; start+minTokens <= len(f.Tokens); start++ {
			k := windowKey(f.Tokens, start, minTokens)
			buckets[k] = append(buckets[k], occurrence{file: fi, start: start})
		}
	}

	covered := make([]map[int]bool, len(files))
	for i := range covered {
		covered[i] = map[int]bool{}
	}

	var clones []Clone
	for fi, f := range files {
		pos := 0
		for pos+minTokens <= len(f.Tokens) {
			if covered[fi][pos] {
				pos++
				continue
			}

			k := windowKey(f.Tokens, pos, minTokens)
			bestLen := 0
			var best occurrence
			for _, o := range buckets[k] {
				if o.file == fi && o.start == pos {
					continue
				}
				if covered[o.file][o.start] {
					continue
				}
				length := extend(f.Tokens, pos, files[o.file].Tokens, o.start, maxExtend(fi, pos, o))
				if length > bestLen {
					bestLen = length
					best = o
				}
			}

			if bestLen < minTokens {
				pos++
				continue
			}

			for t := 0; t < bestLen; t++ {
				covered[fi][pos+t] = true
				covered[best.file][best.start+t] = true
			}
			clones = append(clones, buildClone(f, pos, files[best.file], best.start, bestLen))
			pos += bestLen
		}
	}

	sort.Slice(clones, func(i, j int) bool { return clones[i].Tokens > clones[j].Tokens })
	return clones
}

// extend grows a match forward token-by-token while it keeps matching,
// capped at maxLen so a within-file match can never grow long enough for
// its own range to touch or overlap the range it's matching against.
// Without that cap, a same-file match can spuriously "chase into" the
// other occurrence: past the true end of a duplicate block, the next
// tokens in A can coincidentally equal the next tokens in B simply
// because B's own trailing content (or whatever recurring boilerplate
// follows it, e.g. every function starting with the same keyword) lines
// up by chance — inflating the reported block well past what's actually
// duplicated between the two matched regions.
func extend(a []Token, ai int, b []Token, bi int, maxLen int) int {
	n := 0
	for n < maxLen && ai+n < len(a) && bi+n < len(b) && a[ai+n].Text == b[bi+n].Text {
		n++
	}
	return n
}

// maxExtend returns the longest a match starting at (fi, pos) vs o can
// grow before its range would touch or overlap o's range — unbounded
// (len of the longer file) for a cross-file match, where "overlap"
// doesn't apply.
func maxExtend(fi, pos int, o occurrence) int {
	if o.file != fi {
		return 1 << 30 // effectively unbounded; real match length is bounded by file length anyway
	}
	gap := pos - o.start
	if gap < 0 {
		gap = -gap
	}
	return gap
}

func windowKey(toks []Token, start, length int) string {
	var sb strings.Builder
	for i := 0; i < length; i++ {
		sb.WriteString(toks[start+i].Text)
		sb.WriteByte(0)
	}
	return sb.String()
}

func buildClone(fa FileTokens, ai int, fb FileTokens, bi int, length int) Clone {
	return Clone{
		FileA:      fa.File,
		StartLineA: fa.Tokens[ai].Line,
		EndLineA:   fa.Tokens[ai+length-1].Line,
		FileB:      fb.File,
		StartLineB: fb.Tokens[bi].Line,
		EndLineB:   fb.Tokens[bi+length-1].Line,
		Tokens:     length,
	}
}
