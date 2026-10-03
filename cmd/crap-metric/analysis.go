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
	fmt.Fprintf(w, "\nratchet scope: %d function(s) and file-size checks in changed files\n", len(scoped.Functions))
	if scoped.Passed {
		fmt.Fprintf(w,
			"PASS (ratcheted): no changed function exceeds CRAP %.1f or a function-size threshold, "+
				"and no changed file exceeds a file-size threshold\n",
			failAbove,
		)
	} else {
		fmt.Fprintf(w,
			"FAIL (ratcheted): a changed function exceeds CRAP %.1f or a function-size threshold, "+
				"or a changed file exceeds a file-size threshold\n",
			failAbove,
		)
	}
}

func collectFunctions(
	lang, dir, coverage, excludeFile string,
	patterns stringList,
	out io.Writer,
) ([]crap.Function, []crap.FileSize, []string, exclude.Set, error) {
	analyzer, err := analyzerFor(lang)
	if err != nil {
		return nil, nil, nil, exclude.Set{}, err
	}
	var visited []string
	fns, err := analyzer.Analyze(analyzers.Options{Dir: dir, CoveragePath: coverage, Visited: &visited})
	if err != nil {
		return nil, nil, nil, exclude.Set{}, err
	}
	fns, excluded, err := applyExcludes(dir, excludeFile, patterns, fns, out)
	if err != nil {
		return nil, nil, nil, exclude.Set{}, err
	}
	files, err := collectFileSizes(dir, visited, fns, excluded)
	if err != nil {
		return nil, nil, nil, exclude.Set{}, err
	}
	stampFunctionFileSizes(fns, files)
	return fns, files, visited, excluded, nil
}

// collectFileSizes measures each visited source file once, including files
// with no functions. Function file lengths retain the analyzer's existing
// line-count value; the shared measurement supplies fileless files and the
// maximum-line-length facts.
func collectFileSizes(dir string, visited []string, fns []crap.Function, excluded exclude.Set) (files []crap.FileSize, retErr error) {
	root, err := safefile.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()

	paths := append([]string(nil), visited...)
	functionLines := make(map[string]int, len(fns))
	for _, f := range fns {
		paths = append(paths, f.File)
		functionLines[f.File] = f.FileLines
	}
	seen := make(map[string]bool, len(paths))
	for _, file := range paths {
		if seen[file] || excluded.Matches(file) {
			continue
		}
		seen[file] = true
		source, err := safefile.ReadFileAt(root, filepath.FromSlash(file))
		if err != nil {
			return nil, fmt.Errorf("read %s for file-size measurement: %w", file, err)
		}
		length, line := crap.MaxLineLength(source)
		lines := crap.PhysicalLineCount(source)
		if n, ok := functionLines[file]; ok {
			lines = n
		}
		files = append(files, crap.FileSize{
			File: file, Lines: lines,
			MaxLineLength: length, MaxLineLengthLine: line,
		})
	}
	return files, nil
}

// stampFunctionFileSizes keeps existing function-level report fields aligned
// with the shared file-size facts used by WithSize.
func stampFunctionFileSizes(fns []crap.Function, files []crap.FileSize) {
	byFile := make(map[string]crap.FileSize, len(files))
	for _, file := range files {
		byFile[file.File] = file
	}
	for i := range fns {
		if file, ok := byFile[fns[i].File]; ok {
			fns[i].FileMaxLineLength = file.MaxLineLength
			fns[i].FileMaxLineLengthLine = file.MaxLineLengthLine
		}
	}
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
