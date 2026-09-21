package deadcode

import (
	"fmt"
	"io"
	"sort"

	"git.roost-r.com/cadeh/quality-gates/internal/reportio"
)

// Report is a dead-code verdict over a set of findings.
type Report struct {
	Findings  []Finding `json:"findings"`
	Count     int       `json:"count"`
	FailAbove int       `json:"fail_above"`
	Passed    bool      `json:"passed"`
}

// NewReport tallies findings and applies the fail-above gate (a raw count).
func NewReport(findings []Finding, failAbove int) Report {
	return Report{Findings: findings, Count: len(findings), FailAbove: failAbove, Passed: len(findings) <= failAbove}
}

// ExitCode maps a Report's verdict to a process exit code.
func (r Report) ExitCode() int { return reportio.ExitCode(r.Passed) }

// WriteJSON writes the full report as JSON.
func (r Report) WriteJSON(w io.Writer) error { return reportio.WriteJSON(w, r) }

// WriteTable writes the findings in file/line order, truncated to top
// (0 = all), then the verdict.
func (r Report) WriteTable(w io.Writer, top int) {
	fmt.Fprintf(w, "%d dead symbol(s) found\n\n", r.Count)

	sorted := append([]Finding(nil), r.Findings...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].File != sorted[j].File {
			return sorted[i].File < sorted[j].File
		}
		return sorted[i].Line < sorted[j].Line
	})
	truncated := 0
	if top > 0 && len(sorted) > top {
		truncated = len(sorted) - top
		sorted = sorted[:top]
	}
	for _, f := range sorted {
		fmt.Fprintf(w, "%s:%d  %-11s %s\n", f.File, f.Line, f.Kind, f.Name)
	}
	if truncated > 0 {
		fmt.Fprintf(w, "... %d more not shown\n", truncated)
	}
	if len(sorted) > 0 {
		fmt.Fprintln(w)
	}

	verdict := "PASS"
	if !r.Passed {
		verdict = "FAIL"
	}
	fmt.Fprintf(w, "%s: %d dead symbol(s), gate is %d\n", verdict, r.Count, r.FailAbove)
}
