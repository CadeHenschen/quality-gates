package main

import (
	"git.roost-r.com/cadeh/quality-gates/internal/escape"
	"git.roost-r.com/cadeh/quality-gates/internal/ratchet"
)

// filterForRatchet narrows a scan result to the ratchet scope: hatches in
// files matching onlyFiles, and a line-count denominator of just those
// files' own lines (from LinesByFile) — not the whole --dir's line count,
// which would dilute a small change's hatches into an ineffectively tiny
// rate.
func filterForRatchet(result escape.Result, onlyFiles map[string]bool) ([]escape.Hatch, int) {
	var hatches []escape.Hatch
	for _, h := range result.Hatches {
		if ratchet.Matches(h.File, onlyFiles) {
			hatches = append(hatches, h)
		}
	}

	lines := 0
	for file, count := range result.LinesByFile {
		if ratchet.Matches(file, onlyFiles) {
			lines += count
		}
	}

	return hatches, lines
}
