package crap

import (
	"bytes"
	"sort"
	"strings"
	"unicode/utf8"
)

// FileSize contains source-wide size measurements for one scanned file,
// independent of whether that file contains any functions.
type FileSize struct {
	File              string `json:"file"`
	Lines             int    `json:"lines"`
	MaxLineLength     int    `json:"max_line_length"`
	MaxLineLengthLine int    `json:"max_line_length_line"`
}

// MaxLineLength returns the longest physical line's length in Unicode code
// points and its 1-based line number. Tabs count as one code point; CRLF's
// carriage return is treated as part of the line ending, not its content.
func MaxLineLength(source []byte) (length, line int) {
	if len(source) == 0 {
		return 0, 0
	}
	for i, text := range strings.Split(string(source), "\n") {
		text = strings.TrimSuffix(text, "\r")
		if n := utf8.RuneCountInString(text); n > length {
			length, line = n, i+1
		}
	}
	return length, line
}

// PhysicalLineCount returns the number of source lines, counting a final
// unterminated line and not counting the empty position after a trailing
// newline. Empty input has zero lines.
func PhysicalLineCount(source []byte) int {
	if len(source) == 0 {
		return 0
	}
	lines := bytes.Count(source, []byte{'\n'})
	if source[len(source)-1] != '\n' {
		lines++
	}
	return lines
}

// SizeThresholds are generous size/shape gates, orthogonal to CRAP's
// complexity x (1-coverage) risk score: a flat 40-case switch scores high
// on cyclomatic complexity but reads fine, while a 6-deep nested if scores
// low but is unreadable — CRAP alone misses that second case entirely.
// Each threshold is a physical count, not a risk score; <= 0 disables it,
// matching internal/mutation's --min-mutants convention.
type SizeThresholds struct {
	MaxLines      int `json:"max_lines"`
	MaxParams     int `json:"max_params"`
	MaxNesting    int `json:"max_nesting"`
	MaxFileLines  int `json:"max_file_lines"`
	MaxLineLength int `json:"max_line_length"`
}

// enabled reports whether any threshold is actually gating, so callers
// (WriteTable) can tell "size gate configured, nothing crossed it" apart
// from "no size gate configured at all".
func (th SizeThresholds) enabled() bool {
	return th.MaxLines > 0 || th.MaxParams > 0 || th.MaxNesting > 0 || th.MaxFileLines > 0 || th.MaxLineLength > 0
}

// SizeFinding is one function or file that crossed a SizeThresholds gate.
type SizeFinding struct {
	File string `json:"file"`
	// Name and StartLine are empty/zero for a "file_lines" finding, which
	// is about the file as a whole rather than any one function in it.
	Name      string `json:"name,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	// Kind is "lines", "params", "nesting", "file_lines", or "file_line_length".
	Kind      string `json:"kind"`
	Value     int    `json:"value"`
	Threshold int    `json:"threshold"`
}

// sizeFindings checks every function in fns and every scanned file against
// th, returning one finding per crossed per-function threshold plus one
// per over-limit file. The explicit file facts include source files that
// have no functions.
func sizeFindings(fns []Function, files []FileSize, th SizeThresholds) []SizeFinding {
	var out []SizeFinding
	fileByPath := make(map[string]FileSize, len(files)+len(fns))
	explicit := make(map[string]bool, len(files))
	for _, file := range files {
		fileByPath[file.File] = file
		explicit[file.File] = true
	}
	for _, f := range fns {
		out = append(out, functionSizeFindings(f, th)...)
		if !explicit[f.File] {
			// Retain compatibility with callers that supply only functions.
			file := fileByPath[f.File]
			file.File = f.File
			if f.FileLines > file.Lines {
				file.Lines = f.FileLines
			}
			if f.FileMaxLineLength > file.MaxLineLength {
				file.MaxLineLength = f.FileMaxLineLength
				file.MaxLineLengthLine = f.FileMaxLineLengthLine
			}
			fileByPath[f.File] = file
		}
	}
	for _, file := range fileByPath {
		if th.MaxFileLines > 0 && file.Lines > th.MaxFileLines {
			out = append(out, SizeFinding{File: file.File, Kind: "file_lines", Value: file.Lines, Threshold: th.MaxFileLines})
		}
		if th.MaxLineLength > 0 && file.MaxLineLength > th.MaxLineLength {
			out = append(out, SizeFinding{
				File: file.File, StartLine: file.MaxLineLengthLine,
				Kind: "file_line_length", Value: file.MaxLineLength,
				Threshold: th.MaxLineLength,
			})
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
		out = append(out, SizeFinding{
			File: f.File, Name: f.Name, StartLine: f.StartLine,
			Kind: "nesting", Value: f.MaxNestingDepth, Threshold: th.MaxNesting,
		})
	}
	return out
}
