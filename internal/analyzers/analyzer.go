// Package analyzers defines the common interface each language analyzer
// implements to produce normalized crap.Function data.
package analyzers

import "git.roost-r.com/cadeh/quality-gates/internal/crap"

// Options carries the inputs a language analyzer needs. Not every field
// applies to every language (e.g. Go computes coverage from a profile file
// it locates itself under Dir if CoveragePath is empty).
type Options struct {
	// Dir is the source directory to analyze.
	Dir string
	// CoveragePath is the path to that language's coverage artifact
	// (coverage.json for Python, a go cover profile for Go,
	// coverage-final.json for TS/JS).
	CoveragePath string
	// Visited, when provided, receives paths of source files inspected by
	// the analyzer, including files with no functions to score.
	Visited *[]string
}

// Analyzer produces normalized function-level complexity and coverage data
// for one language.
type Analyzer interface {
	Analyze(opts Options) ([]crap.Function, error)
}
