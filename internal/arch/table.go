package arch

import (
	"fmt"
	"io"
)

// truncateRows caps rows at top (0 means unlimited) and reports how many
// were dropped — the "show the first N, note how many more" shape every
// WriteTable in this package (and, structurally, every other tool's) uses.
func truncateRows[T any](rows []T, top int) (kept []T, truncated int) {
	if top > 0 && len(rows) > top {
		return rows[:top], len(rows) - top
	}
	return rows, 0
}

// writeGateVerdict writes the trailing PASS/FAIL line shared by every
// report in this package: count and failAbove are always a bare number
// (never a rate — see the package doc on both gates being raw counts),
// label names what's being counted ("violation").
func writeGateVerdict(w io.Writer, count, failAbove int, label string, passed bool) {
	fmt.Fprintln(w)
	if passed {
		fmt.Fprintf(w, "PASS: %d %s(s) is at or under %d\n", count, label, failAbove)
	} else {
		fmt.Fprintf(w, "FAIL: %d %s(s) exceeds %d\n", count, label, failAbove)
	}
}
