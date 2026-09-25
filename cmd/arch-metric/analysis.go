package main

import (
	"git.roost-r.com/cadeh/quality-gates/internal/cycle"
	"git.roost-r.com/cadeh/quality-gates/internal/evidence"
)

func unresolvedFor(dir string, issues []cycle.ImportIssue, changed map[string]bool) []string {
	var out []string
	for _, issue := range issues {
		if evidence.Changed(dir, issue.File, changed) {
			out = append(out, issue.File+": "+issue.Specifier)
		}
	}
	return out
}
