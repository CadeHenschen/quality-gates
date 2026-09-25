package escape

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"git.roost-r.com/cadeh/quality-gates/internal/reportio"
)

// Report is a full escape-hatch scan.
type Report struct {
	Analysis   *reportio.Analysis `json:"analysis,omitempty"`
	Hatches    []Hatch            `json:"hatches"`
	TotalLines int                `json:"total_lines"`
	// Rate is hatches per 1000 lines analyzed — normalized so a bigger
	// repo isn't unfairly penalized just for having more raw lines.
	Rate      float64 `json:"rate_per_1000_lines"`
	FailAbove float64 `json:"fail_above"`
	Passed    bool    `json:"passed"`
}

// NewReport builds a Report and applies the fail-above gate (a rate per
// 1000 lines).
func NewReport(hatches []Hatch, totalLines int, failAbove float64) Report {
	sorted := append([]Hatch(nil), hatches...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].File != sorted[j].File {
			return sorted[i].File < sorted[j].File
		}
		return sorted[i].Line < sorted[j].Line
	})

	rate := 0.0
	if totalLines > 0 {
		rate = float64(len(sorted)) / float64(totalLines) * 1000
	}

	return Report{
		Hatches:    sorted,
		TotalLines: totalLines,
		Rate:       rate,
		FailAbove:  failAbove,
		Passed:     rate <= failAbove,
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

// WriteTable writes a human-readable table to w.
func (r Report) WriteTable(w io.Writer, top int) {
	fmt.Fprintf(w, "%d escape hatch(es), %.2f per 1000 lines (%d lines analyzed)\n\n", len(r.Hatches), r.Rate, r.TotalLines)

	if len(r.Hatches) == 0 {
		fmt.Fprintln(w, "PASS: no escape hatches found")
		return
	}

	byPattern := map[string]int{}
	for _, h := range r.Hatches {
		byPattern[h.Pattern]++
	}
	names := make([]string, 0, len(byPattern))
	for n := range byPattern {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(w, "  %-20s %d\n", n, byPattern[n])
	}
	fmt.Fprintln(w)

	rows := r.Hatches
	truncated := 0
	if top > 0 && len(rows) > top {
		truncated = len(rows) - top
		rows = rows[:top]
	}

	widths := [3]int{len("LOCATION"), len("PATTERN"), len("TEXT")}
	type row [3]string
	rendered := make([]row, len(rows))
	for i, h := range rows {
		r := row{fmt.Sprintf("%s:%d", h.File, h.Line), h.Pattern, h.Text}
		for j, cell := range r {
			if len(cell) > widths[j] {
				widths[j] = len(cell)
			}
		}
		rendered[i] = r
	}

	printRow(w, row{"LOCATION", "PATTERN", "TEXT"}, widths)
	printRow(w, row{strings.Repeat("-", widths[0]), strings.Repeat("-", widths[1]), strings.Repeat("-", widths[2])}, widths)
	for _, r := range rendered {
		printRow(w, r, widths)
	}
	if truncated > 0 {
		fmt.Fprintf(w, "... %d more not shown\n", truncated)
	}

	fmt.Fprintln(w)
	if r.Passed {
		fmt.Fprintf(w, "PASS: %.2f per 1000 lines is at or under %.2f\n", r.Rate, r.FailAbove)
	} else {
		fmt.Fprintf(w, "FAIL: %.2f per 1000 lines exceeds %.2f\n", r.Rate, r.FailAbove)
	}
}

func printRow(w io.Writer, r [3]string, widths [3]int) {
	fmt.Fprintf(w, "%-*s  %-*s  %-*s\n", widths[0], r[0], widths[1], r[1], widths[2], r[2])
}
