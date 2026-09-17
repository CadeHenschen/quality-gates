// Package crap implements the CRAP (Change Risk Anti-Patterns) score:
// CRAP(m) = complexity(m)^2 * (1 - coverage(m))^3 + complexity(m)
package crap

import "sort"

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
// since an empty function carries no risk from being untested.
func (f Function) Coverage() float64 {
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
	Functions []Scored `json:"functions"`
	FailAbove float64  `json:"fail_above"`
	Passed    bool     `json:"passed"`
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
