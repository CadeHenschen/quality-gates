package main

import (
	"git.roost-r.com/cadeh/quality-gates/internal/dupe"
	"git.roost-r.com/cadeh/quality-gates/internal/evidence"
)

func withDupeEvidence(report dupe.Report, files []dupe.FileTokens, dir, lang string, changed map[string]bool) (dupe.Report, error) {
	var analyzed []string
	for _, file := range files {
		analyzed = append(analyzed, file.File)
	}
	analysis, err := evidence.Check(dir, lang, false, analyzed, changed)
	if err != nil {
		return report, err
	}
	report.Analysis = &analysis
	report.Passed = report.Passed && analysis.Passed
	return report, nil
}
