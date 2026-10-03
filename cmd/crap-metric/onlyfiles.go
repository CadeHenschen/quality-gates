package main

import (
	"git.roost-r.com/cadeh/quality-gates/internal/crap"
	"git.roost-r.com/cadeh/quality-gates/internal/ratchet"
)

// filterFunctions keeps only the functions whose file matches onlyFiles.
func filterFunctions(fns []crap.Function, onlyFiles map[string]bool, dir string) []crap.Function {
	out := make([]crap.Function, 0, len(fns))
	for _, f := range fns {
		if ratchet.MatchesInDir(f.File, onlyFiles, dir) {
			out = append(out, f)
		}
	}
	return out
}
