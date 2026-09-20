// Package python scans Python test files by running an embedded script
// (scan_tests.py) that walks each with the stdlib `ast` module — no pip
// package needed, the same as the dupe-metric tokenizer script.
package python

import (
	_ "embed"
	"path/filepath"

	"git.roost-r.com/cadeh/quality-gates/internal/embedscript"
	"git.roost-r.com/cadeh/quality-gates/internal/testmetric"
)

//go:embed scan_tests.py
var script []byte

type Scanner struct{}

func (Scanner) Scan(dir string) ([]testmetric.Test, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	var tests []testmetric.Test
	if err := embedscript.Run("python3", script, ".py", absDir, &tests); err != nil {
		return nil, err
	}
	for i := range tests {
		if rel, err := filepath.Rel(absDir, tests[i].File); err == nil {
			tests[i].File = rel
		}
	}
	return tests, nil
}
