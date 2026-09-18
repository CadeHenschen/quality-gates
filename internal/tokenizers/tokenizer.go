// Package tokenizers defines the common interface each language tokenizer
// implements to produce normalized dupe.FileTokens, plus the shared wire
// shape (RawFileTokens/ToFileTokens) the Python and TypeScript tokenizers
// both need for "run an embedded script, parse its JSON" — factored out
// after dupe-metric's own self-check flagged those two tokenizers as ~90%
// duplicated (see CLAUDE.md). The "run an embedded script" machinery
// itself lives in internal/embedscript, shared with the TypeScript
// complexity analyzer too.
package tokenizers

import (
	"git.roost-r.com/cadeh/quality-gates/internal/dupe"
)

// Options carries the inputs a language tokenizer needs.
type Options struct {
	// Dir is the source directory to tokenize.
	Dir string
}

// Tokenizer produces token streams for one language's source files.
type Tokenizer interface {
	Tokenize(opts Options) ([]dupe.FileTokens, error)
}

// RawFileTokens is the JSON wire shape both the embedded Python
// (tokenize_files.py) and Node (scanner.js) scripts emit: one object per
// file, each token as {text, line}. The two scripts speak the exact same
// format by design, so the shape — and the code that consumes it — lives
// here once rather than once per language package.
type RawFileTokens struct {
	File   string `json:"file"`
	Tokens []struct {
		Text string `json:"text"`
		Line int    `json:"line"`
	} `json:"tokens"`
}

// ToFileTokens converts the wire format into dupe.FileTokens.
func ToFileTokens(raw []RawFileTokens) []dupe.FileTokens {
	result := make([]dupe.FileTokens, 0, len(raw))
	for _, f := range raw {
		toks := make([]dupe.Token, len(f.Tokens))
		for i, t := range f.Tokens {
			toks[i] = dupe.Token{Text: t.Text, Line: t.Line}
		}
		result = append(result, dupe.FileTokens{File: f.File, Tokens: toks})
	}
	return result
}
