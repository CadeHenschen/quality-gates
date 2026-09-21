package dupe

import (
	"fmt"
	"io"
	"strings"

	"git.roost-r.com/cadeh/quality-gates/internal/reportio"
)

// WriteJSON writes the full report as JSON.
func (r Report) WriteJSON(w io.Writer) error {
	return reportio.WriteJSON(w, r)
}

// WriteTable writes a human-readable table of clones, largest first, to w.
func (r Report) WriteTable(w io.Writer, top int) {
	fmt.Fprintf(w, "%d duplicate block(s), %.2f%% of %d analyzed lines duplicated\n\n", len(r.Clones), r.DuplicationPercent, r.TotalLines)

	if len(r.Clones) == 0 {
		fmt.Fprintln(w, "PASS: no duplication found")
		return
	}

	rows := r.Clones
	truncated := 0
	if top > 0 && len(rows) > top {
		truncated = len(rows) - top
		rows = rows[:top]
	}

	widths := [3]int{len("LOCATION A"), len("LOCATION B"), len("TOKENS")}
	type row [3]string
	rendered := make([]row, len(rows))
	for i, c := range rows {
		r := row{
			fmt.Sprintf("%s:%d-%d", c.FileA, c.StartLineA, c.EndLineA),
			fmt.Sprintf("%s:%d-%d", c.FileB, c.StartLineB, c.EndLineB),
			fmt.Sprintf("%d", c.Tokens),
		}
		for j, cell := range r {
			if len(cell) > widths[j] {
				widths[j] = len(cell)
			}
		}
		rendered[i] = r
	}

	printRow(w, row{"LOCATION A", "LOCATION B", "TOKENS"}, widths)
	printRow(w, row{strings.Repeat("-", widths[0]), strings.Repeat("-", widths[1]), strings.Repeat("-", widths[2])}, widths)
	for _, r := range rendered {
		printRow(w, r, widths)
	}
	if truncated > 0 {
		fmt.Fprintf(w, "... %d more block(s) not shown\n", truncated)
	}

	fmt.Fprintln(w)
	if r.Passed {
		fmt.Fprintf(w, "PASS: %.2f%% duplication is at or under %.2f%%\n", r.DuplicationPercent, r.FailAbove)
	} else {
		fmt.Fprintf(w, "FAIL: %.2f%% duplication exceeds %.2f%%\n", r.DuplicationPercent, r.FailAbove)
	}
}

func printRow(w io.Writer, r [3]string, widths [3]int) {
	fmt.Fprintf(w, "%-*s  %-*s  %*s\n", widths[0], r[0], widths[1], r[1], widths[2], r[2])
}
