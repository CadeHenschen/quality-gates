package main

import (
	"git.roost-r.com/cadeh/quality-gates/internal/cycle"
	"git.roost-r.com/cadeh/quality-gates/internal/ratchet"
)

// filterForRatchet keeps a cycle if *any* file in it matches onlyFiles —
// touching one file in an existing cycle is still this change's problem
// to answer for, even if most of the cycle predates it.
func filterForRatchet(cycles []cycle.Cycle, onlyFiles map[string]bool) []cycle.Cycle {
	var out []cycle.Cycle
	for _, c := range cycles {
		for _, f := range c.Files {
			if ratchet.Matches(f, onlyFiles) {
				out = append(out, c)
				break
			}
		}
	}
	return out
}
