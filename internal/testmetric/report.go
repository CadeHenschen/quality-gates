package testmetric

import (
	"fmt"
	"io"
	"sort"

	"git.roost-r.com/cadeh/quality-gates/internal/reportio"
)

// Report is a full test-quality scan.
type Report struct {
	Analysis   *reportio.Analysis `json:"analysis,omitempty"`
	Findings   []Finding          `json:"findings"`
	Tests      int                `json:"tests"`
	Assertions int                `json:"assertions"`
	// FailAbove is the finding count above which the gate fails (0 = any
	// finding fails).
	FailAbove int  `json:"fail_above"`
	Passed    bool `json:"passed"`
}

// NewReport builds a Report over findings (already sorted by Analyze) and
// applies the fail-above gate.
func NewReport(findings []Finding, tests, assertions, failAbove int) Report {
	return Report{
		Findings:   findings,
		Tests:      tests,
		Assertions: assertions,
		FailAbove:  failAbove,
		Passed:     len(findings) <= failAbove,
	}
}

// ExitCode maps a Report's verdict to a process exit code.
func (r Report) ExitCode() int { return reportio.ExitCode(r.Passed) }

// WriteJSON writes the full report as JSON.
func (r Report) WriteJSON(w io.Writer) error { return reportio.WriteJSON(w, r) }

// WriteTable writes a human-readable summary and findings table to w.
func (r Report) WriteTable(w io.Writer, top int) {
	density := 0.0
	if r.Tests > 0 {
		density = float64(r.Assertions) / float64(r.Tests)
	}
	fmt.Fprintf(w, "%d test finding(s) across %d test(s), %.1f assertion(s) per test\n\n", len(r.Findings), r.Tests, density)

	if len(r.Findings) == 0 {
		fmt.Fprintln(w, "PASS: no test-quality findings")
		return
	}

	byKind := map[string]int{}
	for _, f := range r.Findings {
		byKind[f.Kind]++
	}
	kinds := make([]string, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		fmt.Fprintf(w, "  %-18s %d\n", k, byKind[k])
	}
	fmt.Fprintln(w)

	rows := r.Findings
	truncated := 0
	if top > 0 && len(rows) > top {
		truncated = len(rows) - top
		rows = rows[:top]
	}
	for _, f := range rows {
		fmt.Fprintf(w, "%s:%d  %s  %s\n    %s\n", f.File, f.Line, f.Kind, f.Test, f.Detail)
	}
	if truncated > 0 {
		fmt.Fprintf(w, "... %d more not shown\n", truncated)
	}

	fmt.Fprintln(w)
	if r.Passed {
		fmt.Fprintf(w, "PASS: %d finding(s) is at or under %d\n", len(r.Findings), r.FailAbove)
	} else {
		fmt.Fprintf(w, "FAIL: %d finding(s) exceeds %d\n", len(r.Findings), r.FailAbove)
	}
}
