package main

import (
	"git.roost-r.com/cadeh/quality-gates/internal/escape"
	"git.roost-r.com/cadeh/quality-gates/internal/evidence"
)

func withEscapeEvidence(report escape.Report, result escape.Result, dir, lang string, changed map[string]bool) (escape.Report, error) {
	var analyzed []string
	for file := range result.LinesByFile {
		analyzed = append(analyzed, file)
	}
	analysis, err := evidence.Check(dir, lang, false, analyzed, changed)
	if err != nil {
		return report, err
	}
	report.Analysis = &analysis
	report.Passed = report.Passed && analysis.Passed
	return report, nil
}
