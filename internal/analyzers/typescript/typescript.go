// Package typescript analyzes TS/JS source by shelling out to an embedded
// Node script (complexity.js) that walks the AST via the *target repo's
// own* installed `typescript` package. Coverage comes from an Istanbul/V8
// coverage-final.json report (as produced by `vitest --coverage` or
// `jest --coverage`), parsed directly in Go.
package typescript

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"git.roost-r.com/cadeh/quality-gates/internal/analyzers"
	"git.roost-r.com/cadeh/quality-gates/internal/crap"
)

//go:embed complexity.js
var complexityScript []byte

type Analyzer struct{}

type jsFunction struct {
	File       string `json:"file"`
	Name       string `json:"name"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	Complexity int    `json:"complexity"`
}

// istanbulFile mirrors the subset of a coverage-final.json per-file entry
// needed to compute per-statement-block coverage within a line range.
type istanbulFile struct {
	StatementMap map[string]struct {
		Start struct {
			Line int `json:"line"`
		} `json:"start"`
	} `json:"statementMap"`
	S map[string]int `json:"s"`
}

func (Analyzer) Analyze(opts analyzers.Options) ([]crap.Function, error) {
	scriptPath, cleanup, err := writeScript()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	absDir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("node", scriptPath, absDir)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("complexity.js failed: %w: %s", err, exitErr.Stderr)
		}
		return nil, fmt.Errorf("run node (is it installed?): %w", err)
	}

	var fns []jsFunction
	if err := json.Unmarshal(out, &fns); err != nil {
		return nil, fmt.Errorf("parse complexity.js output: %w", err)
	}

	coverage := map[string]istanbulFile{}
	if opts.CoveragePath != "" {
		data, err := os.ReadFile(opts.CoveragePath)
		if err != nil {
			return nil, fmt.Errorf("read coverage report: %w", err)
		}
		if err := json.Unmarshal(data, &coverage); err != nil {
			return nil, fmt.Errorf("parse coverage report: %w", err)
		}
	}

	out2 := make([]crap.Function, 0, len(fns))
	for _, f := range fns {
		rel, err := filepath.Rel(absDir, f.File)
		if err != nil {
			rel = f.File
		}

		total, covered := 0, 0
		var uncoveredLines []int
		if fc, ok := coverage[f.File]; ok {
			for id, stmt := range fc.StatementMap {
				if stmt.Start.Line < f.StartLine || stmt.Start.Line > f.EndLine {
					continue
				}
				total++
				if fc.S[id] > 0 {
					covered++
				} else {
					uncoveredLines = append(uncoveredLines, stmt.Start.Line)
				}
			}
		}

		out2 = append(out2, crap.Function{
			File:           rel,
			Name:           f.Name,
			StartLine:      f.StartLine,
			EndLine:        f.EndLine,
			Complexity:     f.Complexity,
			LinesTotal:     total,
			LinesCovered:   covered,
			UncoveredLines: crap.MergeLineRanges(uncoveredLines),
		})
	}
	return out2, nil
}

func writeScript() (path string, cleanup func(), err error) {
	tmp, err := os.CreateTemp("", "crap-metric-complexity-*.js")
	if err != nil {
		return "", nil, err
	}
	if _, err := tmp.Write(complexityScript); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", nil, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", nil, err
	}
	return tmp.Name(), func() { os.Remove(tmp.Name()) }, nil
}
