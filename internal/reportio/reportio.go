// Package reportio holds the byte-for-byte identical parts of each tool's
// Report type: JSON encode/decode, and the Passed → exit code mapping. All
// four tools (crap, dupe, escape, cycle) had their own copy of each of
// these before the migration into one module — see the repo's CLAUDE.md.
package reportio

import (
	"encoding/json"
	"io"
)

// WriteJSON writes report as indented JSON — the machine-readable artifact
// each tool's CLI writes alongside its human-readable table.
func WriteJSON[T any](w io.Writer, report T) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// ReadReport reads back a report written by WriteJSON.
func ReadReport[T any](r io.Reader) (T, error) {
	var report T
	err := json.NewDecoder(r).Decode(&report)
	return report, err
}

// ExitCode maps a report's Passed field to a process exit code.
func ExitCode(passed bool) int {
	if passed {
		return 0
	}
	return 1
}
