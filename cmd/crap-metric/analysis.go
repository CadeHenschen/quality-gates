package main

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"git.roost-r.com/cadeh/quality-gates/internal/analyzers"
	"git.roost-r.com/cadeh/quality-gates/internal/crap"
	"git.roost-r.com/cadeh/quality-gates/internal/evidence"
	"git.roost-r.com/cadeh/quality-gates/internal/exclude"
	"git.roost-r.com/cadeh/quality-gates/internal/safefile"
)

func writeCrapRatchet(w io.Writer, scoped crap.Report, failAbove float64) {
	fmt.Fprintf(w, "\nratchet scope: %d function(s) in changed files\n", len(scoped.Functions))
	if scoped.Passed {
		fmt.Fprintf(w, "PASS (ratcheted): no changed function exceeds CRAP %.1f or a size threshold\n", failAbove)
	} else {
		fmt.Fprintf(w, "FAIL (ratcheted): at least one changed function exceeds CRAP %.1f or a size threshold\n", failAbove)
	}
}

func collectFunctions(lang, dir, coverage, excludeFile string, patterns stringList, out io.Writer) ([]crap.Function, []string, exclude.Set, error) {
	analyzer, err := analyzerFor(lang)
	if err != nil {
		return nil, nil, exclude.Set{}, err
	}
	var visited []string
	fns, err := analyzer.Analyze(analyzers.Options{Dir: dir, CoveragePath: coverage, Visited: &visited})
	if err != nil {
		return nil, nil, exclude.Set{}, err
	}
	fns, excluded, err := applyExcludes(dir, excludeFile, patterns, fns, out)
	if err != nil {
		return nil, nil, exclude.Set{}, err
	}
	if err := stampFileMaxLineLengths(dir, fns); err != nil {
		return nil, nil, exclude.Set{}, err
	}
	return fns, visited, excluded, nil
}

// stampFileMaxLineLengths measures each analyzed source file once, then
// attaches the file-level fact to its functions like FileLines.
func stampFileMaxLineLengths(dir string, fns []crap.Function) (retErr error) {
	root, err := safefile.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()

	measured := make(map[string]struct{ length, line int })
	for i := range fns {
		fact, ok := measured[fns[i].File]
		if !ok {
			source, err := safefile.ReadFileAt(root, filepath.FromSlash(fns[i].File))
			if err != nil {
				return fmt.Errorf("read %s for line-length measurement: %w", fns[i].File, err)
			}
			fact.length, fact.line = crap.MaxLineLength(source)
			measured[fns[i].File] = fact
		}
		fns[i].FileMaxLineLength = fact.length
		fns[i].FileMaxLineLengthLine = fact.line
	}
	return nil
}

func withCrapEvidence(report crap.Report, dir, lang string, analyzed []string, excluded exclude.Set, changed map[string]bool) (crap.Report, error) {
	analysis, err := evidence.CheckFiltered(dir, lang, false, analyzed, changed, excluded.Matches)
	if err != nil {
		return report, err
	}
	report.Analysis = &analysis
	report.Passed = report.Passed && analysis.Passed
	return report, nil
}
