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
	"path/filepath"

	"git.roost-r.com/cadeh/quality-gates/internal/analyzers"
	"git.roost-r.com/cadeh/quality-gates/internal/crap"
	"git.roost-r.com/cadeh/quality-gates/internal/embedscript"
	"git.roost-r.com/cadeh/quality-gates/internal/evidence"
)

//go:embed complexity.js
var complexityScript []byte

type Analyzer struct{}

type jsFunction struct {
	File            string `json:"file"`
	Name            string `json:"name"`
	StartLine       int    `json:"start_line"`
	EndLine         int    `json:"end_line"`
	Complexity      int    `json:"complexity"`
	ParamCount      int    `json:"param_count"`
	MaxNestingDepth int    `json:"max_nesting_depth"`
	FileLines       int    `json:"file_lines"`
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
	absDir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return nil, err
	}

	var fns []jsFunction
	if err := embedscript.Run("node", complexityScript, ".js", absDir, &fns); err != nil {
		return nil, err
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
		fc, inReport := coverage[f.File]
		if inReport {
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
			File:            rel,
			Name:            f.Name,
			StartLine:       f.StartLine,
			EndLine:         f.EndLine,
			Complexity:      f.Complexity,
			LinesTotal:      total,
			LinesCovered:    covered,
			UncoveredLines:  crap.MergeLineRanges(uncoveredLines),
			Unmeasured:      opts.CoveragePath != "" && !inReport,
			ParamCount:      f.ParamCount,
			MaxNestingDepth: f.MaxNestingDepth,
			FileLines:       f.FileLines,
		})
	}
	if opts.Visited != nil {
		files, err := evidence.Eligible(opts.Dir, "ts", false)
		if err != nil {
			return nil, err
		}
		*opts.Visited = append(*opts.Visited, files...)
	}
	return out2, nil
}
