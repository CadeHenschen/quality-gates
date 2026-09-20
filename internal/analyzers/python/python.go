// Package python analyzes Python source by shelling out to `radon cc -j`
// for per-function cyclomatic complexity, and reading a `coverage json`
// report for per-line coverage. Both tools are expected to already be
// present as dev dependencies in the target repo — crap-metric doesn't
// install them.
package python

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"git.roost-r.com/cadeh/quality-gates/internal/analyzers"
	"git.roost-r.com/cadeh/quality-gates/internal/crap"
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

	var cov coverageReport
	if opts.CoveragePath != "" {
		data, err := os.ReadFile(opts.CoveragePath)
		if err != nil {
			return nil, fmt.Errorf("read coverage report: %w", err)
		}
		if err := json.Unmarshal(data, &cov); err != nil {
			return nil, fmt.Errorf("parse coverage report: %w", err)
		}
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

			out = append(out, crap.Function{
				File:           file,
				Name:           e.Name,
				StartLine:      e.Lineno,
				EndLine:        e.Endline,
				Complexity:     e.Complexity,
				LinesTotal:     total,
				LinesCovered:   covered,
				UncoveredLines: crap.MergeLineRanges(uncoveredLines),
				Unmeasured:     unmeasured,
			})
		}
	}
	return out, nil
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
	cmd := exec.Command("radon", "cc", "-j", dir)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("radon failed: %w: %s", err, exitErr.Stderr)
		}
		return nil, fmt.Errorf("run radon (is it installed?): %w", err)
	}
	return out, nil
}
