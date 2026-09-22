package crap

import "sort"

// SizeThresholds are generous size/shape gates, orthogonal to CRAP's
// complexity x (1-coverage) risk score: a flat 40-case switch scores high
// on cyclomatic complexity but reads fine, while a 6-deep nested if scores
// low but is unreadable — CRAP alone misses that second case entirely.
// Each threshold is a physical count, not a risk score; <= 0 disables it,
// matching internal/mutation's --min-mutants convention.
type SizeThresholds struct {
	MaxLines     int `json:"max_lines"`
	MaxParams    int `json:"max_params"`
	MaxNesting   int `json:"max_nesting"`
	MaxFileLines int `json:"max_file_lines"`
}

// enabled reports whether any threshold is actually gating, so callers
// (WriteTable) can tell "size gate configured, nothing crossed it" apart
// from "no size gate configured at all".
func (th SizeThresholds) enabled() bool {
	return th.MaxLines > 0 || th.MaxParams > 0 || th.MaxNesting > 0 || th.MaxFileLines > 0
}

// SizeFinding is one function or file that crossed a SizeThresholds gate.
type SizeFinding struct {
	File string `json:"file"`
	// Name and StartLine are empty/zero for a "file_lines" finding, which
	// is about the file as a whole rather than any one function in it.
	Name      string `json:"name,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	// Kind is "lines", "params", "nesting", or "file_lines".
	Kind      string `json:"kind"`
	Value     int    `json:"value"`
	Threshold int    `json:"threshold"`
}

// sizeFindings checks every function in fns against th, returning one
// finding per crossed per-function threshold plus one per over-long file.
// A file's length is deduplicated to a single finding even though every
// function in it carries the same stamped FileLines value.
func sizeFindings(fns []Function, th SizeThresholds) []SizeFinding {
	var out []SizeFinding
	seenFile := map[string]bool{}
	for _, f := range fns {
		out = append(out, functionSizeFindings(f, th)...)
		if th.MaxFileLines > 0 && f.FileLines > th.MaxFileLines && !seenFile[f.File] {
			seenFile[f.File] = true
			out = append(out, SizeFinding{File: f.File, Kind: "file_lines", Value: f.FileLines, Threshold: th.MaxFileLines})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].StartLine != out[j].StartLine {
			return out[i].StartLine < out[j].StartLine
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

// functionSizeFindings checks the three per-function thresholds (length,
// params, nesting) for a single function.
func functionSizeFindings(f Function, th SizeThresholds) []SizeFinding {
	var out []SizeFinding
	if th.MaxLines > 0 {
		if n := f.LineCount(); n > th.MaxLines {
			out = append(out, SizeFinding{File: f.File, Name: f.Name, StartLine: f.StartLine, Kind: "lines", Value: n, Threshold: th.MaxLines})
		}
	}
	if th.MaxParams > 0 && f.ParamCount > th.MaxParams {
		out = append(out, SizeFinding{File: f.File, Name: f.Name, StartLine: f.StartLine, Kind: "params", Value: f.ParamCount, Threshold: th.MaxParams})
	}
	if th.MaxNesting > 0 && f.MaxNestingDepth > th.MaxNesting {
		out = append(out, SizeFinding{File: f.File, Name: f.Name, StartLine: f.StartLine, Kind: "nesting", Value: f.MaxNestingDepth, Threshold: th.MaxNesting})
	}
	return out
}
