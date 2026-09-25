// Package crap implements the CRAP (Change Risk Anti-Patterns) score:
// CRAP(m) = complexity(m)^2 * (1 - coverage(m))^3 + complexity(m)
package crap

import (
	"git.roost-r.com/cadeh/quality-gates/internal/reportio"
	"sort"
)

// Function is the normalized shape every language analyzer produces one of
// per function/method it finds.
type Function struct {
	File         string `json:"file"`
	Name         string `json:"name"`
	StartLine    int    `json:"start_line"`
	EndLine      int    `json:"end_line"`
	Complexity   int    `json:"complexity"`
	LinesTotal   int    `json:"lines_total"`
	LinesCovered int    `json:"lines_covered"`
	// UncoveredLines lists the line ranges within [StartLine, EndLine]
	// that measured as not covered, so a hotspot points straight at what
	// to test rather than just how risky it is. Omitted (nil) when the
	// analyzer wasn't given a coverage report, or the function is fully
	// covered.
	UncoveredLines []LineRange `json:"uncovered_lines,omitempty"`
	// Unmeasured is set when a coverage report was supplied but has no
	// entry at all for this function's file — typically source no test
	// ever loads. That is different from "measured, nothing coverable"
	// (LinesTotal == 0 in a file the report does cover): it means nobody
	// knows the coverage, and the honest reading is untested, so
	// Coverage() reports 0 instead of the empty-function 1.
	Unmeasured bool `json:"unmeasured,omitempty"`

	// ParamCount, MaxNestingDepth, and FileLines are size/shape facts —
	// orthogonal to CRAP's complexity x (1-coverage) risk score, and
	// gated separately (see SizeThresholds). A receiver (Go) or self/cls
	// (Python) doesn't count toward ParamCount.
	ParamCount int `json:"param_count"`
	// MaxNestingDepth is the deepest level of control-flow block nesting
	// (if/for/while/switch/...) found in the function's own body — 0 for
	// a flat function. A chained else-if reads like a switch's cases, so
	// it deliberately does NOT add a level on top of its "if" (only a
	// genuine nested block does); see each analyzer's size.go for the
	// per-language details of what that means for its grammar.
	MaxNestingDepth int `json:"max_nesting_depth"`
	// FileLines is the physical line count of the file this function
	// lives in (same value for every function in that file). Stamped
	// here, rather than tracked as a separate per-file report, to keep
	// size metrics as "extra fields on crap.Function" per CLAUDE.md,
	// mirroring escape-metric's LinesByFile in spirit but not shape.
	FileLines int `json:"file_lines"`
}

// LineCount is the function's own physical length: EndLine - StartLine +
// 1. Deliberately not a stored field — it's fully derived from StartLine/
// EndLine, which every analyzer already sets.
func (f Function) LineCount() int {
	return f.EndLine - f.StartLine + 1
}

// LineRange is an inclusive [Start, End] line span.
type LineRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// MergeLineRanges sorts a set of (possibly unordered, possibly duplicated)
// line numbers and merges consecutive ones into ranges, e.g. [19 20 21 25]
// -> [{19 21} {25 25}]. Analyzers that track coverage line-by-line (Python,
// TypeScript) use this to produce compact output instead of one range per
// line; the Go analyzer already gets multi-line ranges natively from `go
// tool cover`'s block granularity and doesn't need it.
func MergeLineRanges(lines []int) []LineRange {
	if len(lines) == 0 {
		return nil
	}
	sorted := append([]int(nil), lines...)
	sort.Ints(sorted)

	var out []LineRange
	start, end := sorted[0], sorted[0]
	for _, l := range sorted[1:] {
		switch l {
		case end, end + 1:
			end = l
		default:
			out = append(out, LineRange{Start: start, End: end})
			start, end = l, l
		}
	}
	return append(out, LineRange{Start: start, End: end})
}

// Coverage returns the function's line coverage ratio in [0, 1]. A function
// with no coverable lines (LinesTotal == 0) is treated as fully covered,
// since an empty function carries no risk from being untested — unless it
// is Unmeasured (its file is absent from the coverage report), which counts
// as 0% covered.
func (f Function) Coverage() float64 {
	if f.Unmeasured {
		return 0
	}
	if f.LinesTotal <= 0 {
		return 1
	}
	return float64(f.LinesCovered) / float64(f.LinesTotal)
}

// Score computes the CRAP score for a single function.
func Score(f Function) float64 {
	c := float64(f.Complexity)
	uncovered := 1 - f.Coverage()
	return c*c*uncovered*uncovered*uncovered + c
}

// Scored pairs a Function with its computed CRAP score.
type Scored struct {
	Function
	Crap float64 `json:"crap"`
}

// Report is a scored, sorted set of functions plus the gate that produced
// the pass/fail verdict.
type Report struct {
	Analysis  *reportio.Analysis `json:"analysis,omitempty"`
	Functions []Scored           `json:"functions"`
	FailAbove float64            `json:"fail_above"`

	// SizeThresholds and SizeFindings are set by WithSize; both stay zero
	// on a Report nobody ever applied a size gate to, so JSON output and
	// WriteTable can tell "no size gate configured" apart from "size gate
	// configured, nothing crossed it".
	SizeThresholds SizeThresholds `json:"size_thresholds,omitempty"`
	SizeFindings   []SizeFinding  `json:"size_findings,omitempty"`

	Passed bool `json:"passed"`
}

// crapFailed reports whether any function exceeds the CRAP gate on its
// own, independent of any size gate WithSize may have layered on top —
// used so WriteTable's CRAP-specific pass/fail line stays accurate (and
// its wording unchanged) regardless of what else made the report fail.
func (r Report) crapFailed() bool {
	for _, s := range r.Functions {
		if s.Crap > r.FailAbove {
			return true
		}
	}
	return false
}

// WithSize applies a SizeThresholds gate on top of an already-built
// Report, appending any crossed thresholds as SizeFindings and failing
// the report if there are any. Kept apart from NewReport — mirroring
// internal/mutation's WithMinMutants — so the CRAP score math stays a
// single pass and a ratcheted (--only-files) report can apply its own
// scoped size gate the same way.
func (r Report) WithSize(th SizeThresholds) Report {
	fns := make([]Function, len(r.Functions))
	for i, s := range r.Functions {
		fns[i] = s.Function
	}
	r.SizeThresholds = th
	r.SizeFindings = sizeFindings(fns, th)
	if len(r.SizeFindings) > 0 {
		r.Passed = false
	}
	return r
}

// NewReport scores every function, sorts worst-first by CRAP, and applies
// the fail-above gate.
func NewReport(fns []Function, failAbove float64) Report {
	scored := make([]Scored, len(fns))
	for i, f := range fns {
		scored[i] = Scored{Function: f, Crap: Score(f)}
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].Crap > scored[j].Crap })

	passed := true
	for _, s := range scored {
		if s.Crap > failAbove {
			passed = false
			break
		}
	}

	return Report{Functions: scored, FailAbove: failAbove, Passed: passed}
}
