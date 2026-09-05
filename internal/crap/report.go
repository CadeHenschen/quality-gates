package crap

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// WriteJSON writes the full report as JSON (the machine-readable artifact,
// e.g. crap-report.json).
func (r Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// ReadReport reads back a report written by WriteJSON — used by `diff` to
// load a baseline report from an earlier run.
func ReadReport(r io.Reader) (Report, error) {
	var report Report
	err := json.NewDecoder(r).Decode(&report)
	return report, err
}

// WriteTable writes a human-readable, worst-first hotspot table to w. Rows
// beyond top are omitted with a summary line, since a full function listing
// isn't useful CI output for a repo with hundreds of functions. When
// verbose is true, each row that has uncovered lines gets an indented
// "uncovered: ..." line beneath it, pointing straight at what to test.
func (r Report) WriteTable(w io.Writer, top int, verbose bool) {
	if len(r.Functions) == 0 {
		fmt.Fprintln(w, "no functions analyzed")
		return
	}

	rows := r.Functions
	truncated := 0
	if top > 0 && len(rows) > top {
		truncated = len(rows) - top
		rows = rows[:top]
	}

	widths := [5]int{len("FILE"), len("FUNCTION"), len("COMPLEXITY"), len("COVERAGE"), len("CRAP")}
	type row [5]string
	rendered := make([]row, len(rows))
	for i, s := range rows {
		r := row{
			fmt.Sprintf("%s:%d", s.File, s.StartLine),
			s.Name,
			fmt.Sprintf("%d", s.Complexity),
			fmt.Sprintf("%.0f%%", s.Coverage()*100),
			fmt.Sprintf("%.1f", s.Crap),
		}
		for i, cell := range r {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
		rendered[i] = r
	}

	header := row{"FILE", "FUNCTION", "COMPLEXITY", "COVERAGE", "CRAP"}
	printRow(w, header, widths)
	printRow(w, row{
		strings.Repeat("-", widths[0]),
		strings.Repeat("-", widths[1]),
		strings.Repeat("-", widths[2]),
		strings.Repeat("-", widths[3]),
		strings.Repeat("-", widths[4]),
	}, widths)
	for i, r := range rendered {
		printRow(w, r, widths)
		if verbose {
			if ranges := rows[i].UncoveredLines; len(ranges) > 0 {
				fmt.Fprintf(w, "    uncovered: %s\n", formatRanges(ranges))
			}
		}
	}

	if truncated > 0 {
		fmt.Fprintf(w, "... %d more function(s) not shown\n", truncated)
	}

	fmt.Fprintln(w)
	if r.Passed {
		fmt.Fprintf(w, "PASS: no function exceeds CRAP %.1f\n", r.FailAbove)
	} else {
		fmt.Fprintf(w, "FAIL: at least one function exceeds CRAP %.1f\n", r.FailAbove)
	}
}

func formatRanges(ranges []LineRange) string {
	parts := make([]string, len(ranges))
	for i, rg := range ranges {
		if rg.Start == rg.End {
			parts[i] = fmt.Sprintf("%d", rg.Start)
		} else {
			parts[i] = fmt.Sprintf("%d-%d", rg.Start, rg.End)
		}
	}
	return strings.Join(parts, ", ")
}

func printRow(w io.Writer, r [5]string, widths [5]int) {
	fmt.Fprintf(w, "%-*s  %-*s  %*s  %*s  %*s\n",
		widths[0], r[0],
		widths[1], r[1],
		widths[2], r[2],
		widths[3], r[3],
		widths[4], r[4],
	)
}
