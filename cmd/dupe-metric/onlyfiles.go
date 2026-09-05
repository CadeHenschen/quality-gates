package main

import (
	"git.roost-r.com/cadeh/quality-gates/internal/dupe"
	"git.roost-r.com/cadeh/quality-gates/internal/ratchet"
)

// filterForRatchet narrows files and clones to the ratchet scope: files
// keeps only those matching onlyFiles (used as the report's line-count
// denominator — "of what you touched"); clones keeps any clone where
// *either* side matches (touching one file that already duplicates
// untouched code elsewhere is still something this change should answer
// for). Note: because a kept clone's other side may not be in the
// filtered files list, DuplicationPercent on the scoped report can in
// rare cases read over 100% — both sides' line counts are still summed
// into the numerator (same as the unscoped report), just against a
// smaller denominator. Documented, not treated as a bug — see README.
func filterForRatchet(files []dupe.FileTokens, clones []dupe.Clone, onlyFiles map[string]bool) ([]dupe.FileTokens, []dupe.Clone) {
	scopedFiles := make([]dupe.FileTokens, 0, len(files))
	for _, f := range files {
		if ratchet.Matches(f.File, onlyFiles) {
			scopedFiles = append(scopedFiles, f)
		}
	}

	scopedClones := make([]dupe.Clone, 0, len(clones))
	for _, c := range clones {
		if ratchet.Matches(c.FileA, onlyFiles) || ratchet.Matches(c.FileB, onlyFiles) {
			scopedClones = append(scopedClones, c)
		}
	}

	return scopedFiles, scopedClones
}
