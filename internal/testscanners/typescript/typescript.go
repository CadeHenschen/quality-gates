// Package typescript scans TS/JS test files (*.test.* / *.spec.*) by
// running an embedded Node script (scan_tests.js) that walks each via the
// *target repo's own* installed `typescript` package — the same approach,
// resolution, and TS7 fallback as internal/analyzers/typescript.
package typescript

import (
	_ "embed"
	"path/filepath"

	"git.roost-r.com/cadeh/quality-gates/internal/embedscript"
	"git.roost-r.com/cadeh/quality-gates/internal/testmetric"
)

//go:embed scan_tests.js
var script []byte

type Scanner struct{}

func (Scanner) Scan(dir string) ([]testmetric.Test, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	var tests []testmetric.Test
	if err := embedscript.Run("node", script, absDir, &tests); err != nil {
		return nil, err
	}
	for i := range tests {
		if rel, err := filepath.Rel(absDir, tests[i].File); err == nil {
			tests[i].File = rel
		}
	}
	return tests, nil
}
