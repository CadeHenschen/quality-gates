package main

import (
	"fmt"
	"git.roost-r.com/cadeh/quality-gates/internal/evidence"
	"git.roost-r.com/cadeh/quality-gates/internal/mutation"
	"git.roost-r.com/cadeh/quality-gates/internal/testmetric"
	"io"
)

func writeTestRatchet(w io.Writer, scoped testmetric.Report, failAbove int) {
	fmt.Fprintf(w, "\nratchet scope: %d finding(s) across %d test(s) in changed files\n", len(scoped.Findings), scoped.Tests)
	verdict := "PASS"
	if !scoped.Passed {
		verdict = "FAIL"
	}
	fmt.Fprintf(w, "%s (ratcheted): %d finding(s), gate is %d\n", verdict, len(scoped.Findings), failAbove)
}

func publishCheckReport(report testmetric.Report, out io.Writer, top int, jsonOut string) error {
	report.WriteTable(out, top)
	if report.Analysis != nil {
		report.Analysis.WriteText(out)
	}
	if jsonOut != "" {
		return writeJSON(report.WriteJSON, jsonOut)
	}
	return nil
}

func testAnalysis(dir, lang string, tests []testmetric.Test, changed map[string]bool) (evidence.Report, error) {
	var files []string
	for _, test := range tests {
		if !test.Suite {
			files = append(files, test.File)
		}
	}
	return evidence.Check(dir, lang, true, files, changed)
}

func withTestAnalysis(report testmetric.Report, dir, lang string, tests []testmetric.Test, changed map[string]bool) (testmetric.Report, error) {
	analysis, err := testAnalysis(dir, lang, tests, changed)
	if err != nil {
		return report, err
	}
	report.Analysis = &analysis
	report.Passed = report.Passed && analysis.Passed
	return report, nil
}

func mutationAnalysis(dir, lang string, mutants []mutation.Mutant, changed map[string]bool) (evidence.Report, error) {
	files := make([]string, 0, len(mutants))
	for _, mutant := range mutants {
		files = append(files, mutant.File)
	}
	return evidence.Check(dir, lang, false, files, changed)
}

type mutationScopeOptions struct {
	Dir, Lang                 string
	Changed                   map[string]bool
	FailBelow                 float64
	CoveredOnly               bool
	MinMutants, MinimumGraded int
}

func scopedMutationReport(mutants []mutation.Mutant, opts mutationScopeOptions) (mutation.Report, *evidence.Report, error) {
	minimum := opts.MinimumGraded
	var analysis *evidence.Report
	if opts.MinimumGraded > 0 {
		checked, err := mutationAnalysis(opts.Dir, opts.Lang, mutants, opts.Changed)
		if err != nil {
			return mutation.Report{}, nil, err
		}
		analysis = &checked
		if checked.Required == 0 {
			minimum = 0
		}
	}
	report := mutation.NewReport(mutants, opts.FailBelow, opts.CoveredOnly).WithMinMutants(opts.MinMutants).WithMinimumGraded(minimum)
	if analysis != nil {
		report = report.WithEvidence(*analysis)
	}
	return report, analysis, nil
}
