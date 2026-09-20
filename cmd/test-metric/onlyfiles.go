package main

import (
	"git.roost-r.com/cadeh/quality-gates/internal/mutation"
	"git.roost-r.com/cadeh/quality-gates/internal/ratchet"
	"git.roost-r.com/cadeh/quality-gates/internal/testmetric"
)

// filterTests narrows scanned tests to those in files matching onlyFiles.
// Findings are then recomputed from just these, rather than filtering the
// full finding list, so the scoped assertion/test counts stay consistent
// with the scoped findings.
func filterTests(tests []testmetric.Test, onlyFiles map[string]bool) []testmetric.Test {
	var out []testmetric.Test
	for _, t := range tests {
		if ratchet.Matches(t.File, onlyFiles) {
			out = append(out, t)
		}
	}
	return out
}

// filterMutants narrows mutants to those in files matching onlyFiles. The
// score's numerator and denominator both derive from the mutant list, so
// unlike escape-metric's per-line rate there's no separate denominator to
// scope down.
func filterMutants(mutants []mutation.Mutant, onlyFiles map[string]bool) []mutation.Mutant {
	var out []mutation.Mutant
	for _, m := range mutants {
		if ratchet.Matches(m.File, onlyFiles) {
			out = append(out, m)
		}
	}
	return out
}
