// Package dupe finds duplicate code blocks via token-shingling: exact-match
// windows of tokens, greedily extended to their full length, reported as a
// duplication percentage — the numerical gate. Complements crap-metric
// (complexity/coverage) rather than folding into it: a different question
// ("is this code repeated") with a different detection method (tokenizing
// + matching, not parsing + scoring), so a standalone tool with its own
// gate is the more honest shape than a bolted-on subcommand.
package dupe

import (
	"git.roost-r.com/cadeh/quality-gates/internal/reportio"
)

// Token is one lexical token from a source file, as produced by a language
// tokenizer. Text is used for exact matching, so a tokenizer that wants
// "near-duplicate despite renamed identifiers" detection would normalize
// identifier text before handing tokens here — v1's tokenizers don't do
// that (documented limitation, see README).
type Token struct {
	Text string
	Line int
}

// FileTokens is one file's token stream.
type FileTokens struct {
	File   string
	Tokens []Token
}

// Clone is one duplicate block: a token run in File A matching a token run
// of the same length in File B.
type Clone struct {
	FileA      string `json:"file_a"`
	StartLineA int    `json:"start_line_a"`
	EndLineA   int    `json:"end_line_a"`
	FileB      string `json:"file_b"`
	StartLineB int    `json:"start_line_b"`
	EndLineB   int    `json:"end_line_b"`
	Tokens     int    `json:"tokens"`
}

// Report is a full duplication analysis.
type Report struct {
	Analysis           *reportio.Analysis `json:"analysis,omitempty"`
	Clones             []Clone            `json:"clones"`
	TotalLines         int                `json:"total_lines"`
	DuplicatedLines    int                `json:"duplicated_lines"`
	DuplicationPercent float64            `json:"duplication_percent"`
	FailAbove          float64            `json:"fail_above"`
	Passed             bool               `json:"passed"`
}

// NewReport builds a Report from a set of files and the clones found in
// them, and applies the fail-above gate (a duplication percentage).
func NewReport(files []FileTokens, clones []Clone, failAbove float64) Report {
	totalLines := 0
	for _, f := range files {
		totalLines += countLines(f)
	}

	dupLines := 0
	for _, c := range clones {
		dupLines += (c.EndLineA - c.StartLineA + 1) + (c.EndLineB - c.StartLineB + 1)
	}

	pct := 0.0
	if totalLines > 0 {
		pct = float64(dupLines) / float64(totalLines) * 100
	}

	return Report{
		Clones:             clones,
		TotalLines:         totalLines,
		DuplicatedLines:    dupLines,
		DuplicationPercent: pct,
		FailAbove:          failAbove,
		Passed:             pct <= failAbove,
	}
}

// countLines returns the number of distinct source lines a file's tokens
// span — an approximation of the file's line count using only what the
// tokenizer gave us (its last token's line), not a full line count of the
// file (blank/comment-only lines with no tokens aren't counted, which is
// fine: they can't be part of a duplicated block either).
func countLines(f FileTokens) int {
	if len(f.Tokens) == 0 {
		return 0
	}
	return f.Tokens[len(f.Tokens)-1].Line - f.Tokens[0].Line + 1
}

// ExitCode maps a Report's verdict to a process exit code.
func (r Report) ExitCode() int {
	return reportio.ExitCode(r.Passed)
}
