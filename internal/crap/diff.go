package crap

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Delta compares one function's CRAP score between two reports.
type Delta struct {
	File    string  `json:"file"`
	Name    string  `json:"name"`
	OldCrap float64 `json:"old_crap"`
	NewCrap float64 `json:"new_crap"`
	// Change is "new", "removed", or "changed" — a function present in
	// only one report, or scored differently in both.
	Change string `json:"change"`
}

func (d Delta) delta() float64 { return d.NewCrap - d.OldCrap }

type funcKey struct{ file, name string }

func keyOf(f Function) funcKey { return funcKey{f.File, f.Name} }

// Diff compares old and new reports (matching functions by file+name) and
// returns every function that's new, removed, or changed score, sorted
// worst-regression-first (so a growing hotspot is always at the top,
// mirroring Report's own worst-first ordering). Unchanged functions are
// omitted — they're not interesting for a trend view.
func Diff(old, new Report) []Delta {
	oldByKey := make(map[funcKey]Scored, len(old.Functions))
	for _, f := range old.Functions {
		oldByKey[keyOf(f.Function)] = f
	}
	newByKey := make(map[funcKey]Scored, len(new.Functions))
	for _, f := range new.Functions {
		newByKey[keyOf(f.Function)] = f
	}

	var deltas []Delta
	for k, nf := range newByKey {
		if of, ok := oldByKey[k]; ok {
			if of.Crap != nf.Crap {
				deltas = append(deltas, Delta{File: k.file, Name: k.name, OldCrap: of.Crap, NewCrap: nf.Crap, Change: "changed"})
			}
		} else {
			deltas = append(deltas, Delta{File: k.file, Name: k.name, NewCrap: nf.Crap, Change: "new"})
		}
	}
	for k, of := range oldByKey {
		if _, ok := newByKey[k]; !ok {
			deltas = append(deltas, Delta{File: k.file, Name: k.name, OldCrap: of.Crap, Change: "removed"})
		}
	}

	sort.Slice(deltas, func(i, j int) bool { return deltas[i].delta() > deltas[j].delta() })
	return deltas
}

// WriteDiffTable writes a human-readable, worst-regression-first table of
// deltas to w. Rows beyond top are omitted with a summary line.
func WriteDiffTable(w io.Writer, deltas []Delta, top int) {
	if len(deltas) == 0 {
		fmt.Fprintln(w, "no CRAP score changes")
		return
	}

	rows := deltas
	truncated := 0
	if top > 0 && len(rows) > top {
		truncated = len(rows) - top
		rows = rows[:top]
	}

	widths := [4]int{len("FILE"), len("FUNCTION"), len("CHANGE"), len("CRAP")}
	type row [4]string
	rendered := make([]row, len(rows))
	for i, d := range rows {
		r := row{d.File, d.Name, d.Change, formatDelta(d)}
		for j, cell := range r {
			if len(cell) > widths[j] {
				widths[j] = len(cell)
			}
		}
		rendered[i] = r
	}

	printDiffRow(w, row{"FILE", "FUNCTION", "CHANGE", "CRAP"}, widths)
	printDiffRow(w, row{
		strings.Repeat("-", widths[0]), strings.Repeat("-", widths[1]), strings.Repeat("-", widths[2]), strings.Repeat("-", widths[3]),
	}, widths)
	for _, r := range rendered {
		printDiffRow(w, r, widths)
	}

	if truncated > 0 {
		fmt.Fprintf(w, "... %d more change(s) not shown\n", truncated)
	}
}

func formatDelta(d Delta) string {
	switch d.Change {
	case "new":
		return fmt.Sprintf("new: %.1f", d.NewCrap)
	case "removed":
		return fmt.Sprintf("removed (was %.1f)", d.OldCrap)
	default:
		sign := "+"
		if d.delta() < 0 {
			sign = ""
		}
		return fmt.Sprintf("%.1f -> %.1f (%s%.1f)", d.OldCrap, d.NewCrap, sign, d.delta())
	}
}

func printDiffRow(w io.Writer, r [4]string, widths [4]int) {
	fmt.Fprintf(w, "%-*s  %-*s  %-*s  %-*s\n", widths[0], r[0], widths[1], r[1], widths[2], r[2], widths[3], r[3])
}

// WriteDiffJSON writes the deltas as JSON.
func WriteDiffJSON(w io.Writer, deltas []Delta) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(deltas)
}
