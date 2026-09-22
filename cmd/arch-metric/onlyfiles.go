package main

import (
	"git.roost-r.com/cadeh/quality-gates/internal/ratchet"
)

// filterForRatchet keeps an item if either the importing file or the
// imported file matches onlyFiles — touching either end of a pre-existing
// boundary break is still this change's problem to answer for, even if
// the other end predates it. Generic because arch.Violation and
// arch.StabilityViolation have identical File/Import semantics but
// aren't otherwise related types — same reasoning as reportio's own
// WriteJSON[T any]/ReadReport[T any].
func filterForRatchet[T any](items []T, onlyFiles map[string]bool, file, imp func(T) string) []T {
	var out []T
	for _, v := range items {
		if ratchet.Matches(file(v), onlyFiles) || ratchet.Matches(imp(v), onlyFiles) {
			out = append(out, v)
		}
	}
	return out
}
