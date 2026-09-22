package arch

import (
	"fmt"
	"io"
	"sort"

	"git.roost-r.com/cadeh/quality-gates/internal/reportio"
)

// DiffEntry is one violation that appeared or disappeared between two
// reports. Unlike crap-metric's Delta, there's no continuous score to
// track a "changed" case for — a layer violation either exists or it
// doesn't, so Change is always "new" or "fixed".
type DiffEntry struct {
	Violation Violation `json:"violation"`
	Change    string    `json:"change"`
}

type violationKey struct{ rule, file, imp string }

func keyOf(v Violation) violationKey { return violationKey{v.Rule, v.File, v.Import} }

// Diff compares old and new reports (matching violations by rule+file+
// import) and returns every violation that's new or fixed, new ones
// first. Unchanged violations are omitted — they're not interesting for
// a trend view.
func Diff(old, new Report) []DiffEntry {
	oldByKey := make(map[violationKey]Violation, len(old.Violations))
	for _, v := range old.Violations {
		oldByKey[keyOf(v)] = v
	}
	newByKey := make(map[violationKey]Violation, len(new.Violations))
	for _, v := range new.Violations {
		newByKey[keyOf(v)] = v
	}

	var entries []DiffEntry
	for k, v := range newByKey {
		if _, ok := oldByKey[k]; !ok {
			entries = append(entries, DiffEntry{Violation: v, Change: "new"})
		}
	}
	for k, v := range oldByKey {
		if _, ok := newByKey[k]; !ok {
			entries = append(entries, DiffEntry{Violation: v, Change: "fixed"})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Change != entries[j].Change {
			return entries[i].Change == "new" // new sorts before fixed
		}
		a, b := entries[i].Violation, entries[j].Violation
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Import < b.Import
	})
	return entries
}

// WriteDiffTable writes a human-readable table of entries to w. Rows
// beyond top are omitted with a summary line.
func WriteDiffTable(w io.Writer, entries []DiffEntry, top int) {
	if len(entries) == 0 {
		fmt.Fprintln(w, "no violation changes")
		return
	}

	rows, truncated := truncateRows(entries, top)
	for _, e := range rows {
		v := e.Violation
		fmt.Fprintf(w, "%-6s [%s] %s -> %s (%s imports %s)\n", e.Change, v.Rule, v.FromPkg, v.ToPkg, v.File, v.Import)
	}
	if truncated > 0 {
		fmt.Fprintf(w, "... %d more change(s) not shown\n", truncated)
	}
}

// WriteDiffJSON writes entries as JSON.
func WriteDiffJSON(w io.Writer, entries []DiffEntry) error {
	return reportio.WriteJSON(w, entries)
}
