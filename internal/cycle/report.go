package cycle

import (
	"fmt"
	"io"
	"strings"

	"git.roost-r.com/cadeh/quality-gates/internal/reportio"
)

// Report is a full cycle analysis.
type Report struct {
	Analysis          *reportio.Analysis `json:"analysis,omitempty"`
	UnresolvedImports []ImportIssue      `json:"unresolved_imports,omitempty"`
	Cycles            []Cycle            `json:"cycles"`
	FilesAnalyzed     int                `json:"files_analyzed"`
	FailAbove         int                `json:"fail_above"`
	Passed            bool               `json:"passed"`
}

// NewReport builds a Report and applies the fail-above gate (a raw cycle
// count — cycles are a yes/no structural fact, not a rate to normalize).
func NewReport(filesAnalyzed int, cycles []Cycle, failAbove int) Report {
	return Report{
		Cycles:        cycles,
		FilesAnalyzed: filesAnalyzed,
		FailAbove:     failAbove,
		Passed:        len(cycles) <= failAbove,
	}
}

// ExitCode maps a Report's verdict to a process exit code.
func (r Report) ExitCode() int {
	return reportio.ExitCode(r.Passed)
}

// WriteJSON writes the full report as JSON.
func (r Report) WriteJSON(w io.Writer) error {
	return reportio.WriteJSON(w, r)
}

// WriteTable writes a human-readable table to w, one cycle per row group.
func (r Report) WriteTable(w io.Writer, top int) {
	fmt.Fprintf(w, "%d cycle(s) found (%d files analyzed)\n\n", len(r.Cycles), r.FilesAnalyzed)

	if len(r.Cycles) == 0 {
		fmt.Fprintln(w, "PASS: no import cycles found")
		return
	}

	rows := r.Cycles
	truncated := 0
	if top > 0 && len(rows) > top {
		truncated = len(rows) - top
		rows = rows[:top]
	}

	for i, c := range rows {
		fmt.Fprintf(w, "%d. %s\n", i+1, strings.Join(c.Chain, " -> "))
		// Chain is one concrete path back to its own start, which may not
		// visit every file in a larger tangle — show the full set too
		// when it says more than the chain already did.
		if len(c.Files) > len(c.Chain)-1 {
			fmt.Fprintf(w, "   (%d file(s) total in this cycle: %s)\n", len(c.Files), strings.Join(c.Files, ", "))
		}
	}
	if truncated > 0 {
		fmt.Fprintf(w, "... %d more cycle(s) not shown\n", truncated)
	}

	fmt.Fprintln(w)
	if r.Passed {
		fmt.Fprintf(w, "PASS: %d cycle(s) is at or under %d\n", len(r.Cycles), r.FailAbove)
	} else {
		fmt.Fprintf(w, "FAIL: %d cycle(s) exceeds %d\n", len(r.Cycles), r.FailAbove)
	}
}
