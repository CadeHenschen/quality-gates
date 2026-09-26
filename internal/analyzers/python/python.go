// Package python analyzes Python source by shelling out to `radon cc -j`
// for per-function cyclomatic complexity, and reading a `coverage json`
// report for per-line coverage. Both tools are expected to already be
// present as dev dependencies in the target repo — crap-metric doesn't
// install them.
package python

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"

	"git.roost-r.com/cadeh/quality-gates/internal/analyzers"
	"git.roost-r.com/cadeh/quality-gates/internal/crap"
	"git.roost-r.com/cadeh/quality-gates/internal/evidence"
	"git.roost-r.com/cadeh/quality-gates/internal/safefile"
)

type Analyzer struct{}

// radonEntry mirrors the subset of `radon cc -j` output crap-metric needs.
// radon nests method entries inside their class entry rather than listing
// them flat, so this also carries Methods to recurse into.
type radonEntry struct {
	Type       string       `json:"type"` // "function", "method", or "class"
	Name       string       `json:"name"`
	Lineno     int          `json:"lineno"`
	Endline    int          `json:"endline"`
	Complexity int          `json:"complexity"`
	Methods    []radonEntry `json:"methods"`
}

type coverageReport struct {
	Files map[string]struct {
		ExecutedLines []int `json:"executed_lines"`
		MissingLines  []int `json:"missing_lines"`
	} `json:"files"`
}

func (Analyzer) Analyze(opts analyzers.Options) ([]crap.Function, error) {
	radonOut, err := runRadon(opts.Dir)
	if err != nil {
		return nil, err
	}

	var byFile map[string][]radonEntry
	if err := json.Unmarshal(radonOut, &byFile); err != nil {
		return nil, fmt.Errorf("parse radon output: %w", err)
	}
	byFile, err = normalizeRadonFiles(opts.Dir, byFile)
	if err != nil {
		return nil, err
	}

	sizes, err := sizeFacts(opts.Dir)
	if err != nil {
		return nil, fmt.Errorf("size facts: %w", err)
	}

	cov, err := readCoverage(opts.CoveragePath)
	if err != nil {
		return nil, err
	}

	var out []crap.Function
	for file, entries := range byFile {
		fileCov, inReport := cov.Files[file]
		unmeasured := opts.CoveragePath != "" && !inReport
		executed := toSet(fileCov.ExecutedLines)
		measured := toSet(fileCov.ExecutedLines)
		for _, l := range fileCov.MissingLines {
			measured[l] = struct{}{}
		}

		for _, e := range flatten(entries) {
			total, covered := 0, 0
			var uncoveredLines []int
			for line := range measured {
				if line < e.Lineno || line > e.Endline {
					continue
				}
				total++
				if _, ok := executed[line]; ok {
					covered++
				} else {
					uncoveredLines = append(uncoveredLines, line)
				}
			}

			size := sizes[sizeKey{file, e.Lineno}]
			out = append(out, crap.Function{
				File:            file,
				Name:            e.Name,
				StartLine:       e.Lineno,
				EndLine:         e.Endline,
				Complexity:      e.Complexity,
				LinesTotal:      total,
				LinesCovered:    covered,
				UncoveredLines:  crap.MergeLineRanges(uncoveredLines),
				Unmeasured:      unmeasured,
				ParamCount:      size.ParamCount,
				MaxNestingDepth: size.MaxNestingDepth,
				FileLines:       size.FileLines,
			})
		}
	}
	if opts.Visited != nil {
		files, err := evidence.Eligible(opts.Dir, "python", false)
		if err != nil {
			return nil, err
		}
		*opts.Visited = append(*opts.Visited, files...)
	}
	return out, nil
}

func readCoverage(path string) (coverageReport, error) {
	var report coverageReport
	if path == "" {
		return report, nil
	}
	data, err := safefile.ReadFile(path)
	if err != nil {
		return report, fmt.Errorf("read coverage report: %w", err)
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return report, fmt.Errorf("parse coverage report: %w", err)
	}
	return report, nil
}

func normalizeRadonFiles(dir string, files map[string][]radonEntry) (map[string][]radonEntry, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	normalized := make(map[string][]radonEntry, len(files))
	for file, entries := range files {
		clean := filepath.Clean(file)
		if filepath.IsAbs(clean) {
			if rel, err := filepath.Rel(absDir, clean); err == nil {
				clean = rel
			}
		}
		normalized[filepath.Join(dir, clean)] = entries
	}
	return normalized, nil
}

// flatten drops radon's "class" entries. radon's top-level list already
// includes each class's methods as their own flat "method" entries
// alongside the class entry (which just repeats them under Methods for
// structure) — so recursing into Methods would double-count. A class has
// no complexity of its own beyond its methods, so it isn't scored.
func flatten(entries []radonEntry) []radonEntry {
	var out []radonEntry
	for _, e := range entries {
		if e.Type == "class" {
			continue
		}
		out = append(out, e)
	}
	return out
}

func toSet(lines []int) map[int]struct{} {
	set := make(map[int]struct{}, len(lines))
	for _, l := range lines {
		set[l] = struct{}{}
	}
	return set
}

func runRadon(dir string) ([]byte, error) {
	// Keep the executable and argv literal. The selected source directory is
	// the process working directory, not an argument interpreted by Radon.
	cmd := exec.Command("radon", "cc", "-j", ".")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("radon failed: %w: %s", err, exitErr.Stderr)
		}
		return nil, fmt.Errorf("run radon (is it installed?): %w", err)
	}
	return out, nil
}
