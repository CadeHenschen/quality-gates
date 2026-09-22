package crap

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"git.roost-r.com/cadeh/quality-gates/internal/reportio"
)

// WriteJSON writes the full report as JSON (the machine-readable artifact,
// e.g. crap-report.json).
func (r Report) WriteJSON(w io.Writer) error {
	return reportio.WriteJSON(w, r)
}

// ReadReport reads back a report written by WriteJSON — used by `diff` to
// load a baseline report from an earlier run.
func ReadReport(r io.Reader) (Report, error) {
	return reportio.ReadReport[Report](r)
}

type row [5]string

// renderRows formats each Scored as a table row and grows widths to fit
// every cell, so WriteTable's own body stays focused on layout/sequencing.
func renderRows(rows []Scored) (widths [5]int, rendered []row) {
	widths = [5]int{len("FILE"), len("FUNCTION"), len("COMPLEXITY"), len("COVERAGE"), len("CRAP")}
	rendered = make([]row, len(rows))
	for i, s := range rows {
		coverage := fmt.Sprintf("%.0f%%", s.Coverage()*100)
		if s.Unmeasured {
			coverage = "none"
		}
		r := row{
			fmt.Sprintf("%s:%d", s.File, s.StartLine),
			s.Name,
			fmt.Sprintf("%d", s.Complexity),
			coverage,
			fmt.Sprintf("%.1f", s.Crap),
		}
		for i, cell := range r {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
		rendered[i] = r
	}
	return widths, rendered
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

	widths, rendered := renderRows(rows)
	printRow(w, row{"FILE", "FUNCTION", "COMPLEXITY", "COVERAGE", "CRAP"}, widths)
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
	if n, files := r.unmeasured(); n > 0 {
		fmt.Fprintf(w, "WARNING: %d function(s) in %d file(s) are absent from the coverage report (no test loads them) and are scored as 0%% covered:\n", n, len(files))
		for _, f := range files {
			fmt.Fprintf(w, "  %s\n", f)
		}
		fmt.Fprintln(w)
	}
	writeSizeFindings(w, r.SizeFindings)

	if r.crapFailed() {
		fmt.Fprintf(w, "FAIL: at least one function exceeds CRAP %.1f\n", r.FailAbove)
	} else {
		fmt.Fprintf(w, "PASS: no function exceeds CRAP %.1f\n", r.FailAbove)
	}
	if len(r.SizeFindings) > 0 {
		fmt.Fprintf(w, "FAIL: %d size threshold violation(s)\n", len(r.SizeFindings))
	} else if r.SizeThresholds.enabled() {
		fmt.Fprintln(w, "PASS: within size thresholds")
	}
}

// writeSizeFindings prints one line per SizeFinding, grouped under a "SIZE:"
// header — the same "warning block before the verdict" shape as the
// unmeasured-functions block above it. A no-op when there are none, so a
// report nobody applied WithSize to (or one with nothing to flag) doesn't
// grow a stray blank section.
func writeSizeFindings(w io.Writer, findings []SizeFinding) {
	if len(findings) == 0 {
		return
	}
	fmt.Fprintln(w, "SIZE:")
	for _, f := range findings {
		loc := f.File
		if f.StartLine > 0 {
			loc = fmt.Sprintf("%s:%d", f.File, f.StartLine)
		}
		switch f.Kind {
		case "lines":
			fmt.Fprintf(w, "  %s %s: %d lines (> %d)\n", loc, f.Name, f.Value, f.Threshold)
		case "params":
			fmt.Fprintf(w, "  %s %s: %d params (> %d)\n", loc, f.Name, f.Value, f.Threshold)
		case "nesting":
			fmt.Fprintf(w, "  %s %s: nesting depth %d (> %d)\n", loc, f.Name, f.Value, f.Threshold)
		case "file_lines":
			fmt.Fprintf(w, "  %s: file is %d lines (> %d)\n", loc, f.Value, f.Threshold)
		}
	}
	fmt.Fprintln(w)
}

// unmeasured counts functions whose file is missing from the coverage
// report, and lists those files (sorted) so the fix — add a test that
// loads them, or include them in the coverage run — is obvious.
func (r Report) unmeasured() (int, []string) {
	seen := map[string]bool{}
	n := 0
	for _, s := range r.Functions {
		if s.Unmeasured {
			n++
			seen[s.File] = true
		}
	}
	files := make([]string, 0, len(seen))
	for f := range seen {
		files = append(files, f)
	}
	sort.Strings(files)
	return n, files
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
