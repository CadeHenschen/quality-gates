package typescript

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/analyzers"
	"git.roost-r.com/cadeh/quality-gates/internal/crap"
)

func skipIfNoNode(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not on PATH")
	}
}

// writeCoverageFixture builds an Istanbul-shaped coverage-final.json for
// sample.ts, keyed by its absolute path (matching what complexity.js
// reports as "file").
func writeCoverageFixture(t *testing.T, absFile string) string {
	t.Helper()

	type stmt struct {
		Start struct {
			Line int `json:"line"`
		} `json:"start"`
	}
	doc := map[string]struct {
		StatementMap map[string]stmt `json:"statementMap"`
		S            map[string]int  `json:"s"`
	}{
		absFile: {
			StatementMap: map[string]stmt{
				"0": {Start: struct {
					Line int `json:"line"`
				}{4}}, // simple body
				"1": {Start: struct {
					Line int `json:"line"`
				}{8}}, // branchy: if
				"2": {Start: struct {
					Line int `json:"line"`
				}{9}}, // branchy: return x
				"3": {Start: struct {
					Line int `json:"line"`
				}{11}}, // branchy: return -x (uncovered)
				"4": {Start: struct {
					Line int `json:"line"`
				}{15}}, // loopy: total = 0
				"5": {Start: struct {
					Line int `json:"line"`
				}{17}}, // loopy: if
				"6": {Start: struct {
					Line int `json:"line"`
				}{18}}, // loopy: total += v (uncovered)
			},
			S: map[string]int{
				"0": 1,
				"1": 1,
				"2": 1,
				"3": 0,
				"4": 1,
				"5": 1,
				"6": 0,
			},
		},
	}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "coverage-final.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAnalyzeComplexityAndCoverage(t *testing.T) {
	skipIfNoNode(t)

	dir := "../../../testdata/crap/typescript"
	absDir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	absFile := filepath.Join(absDir, "sample.ts")

	if _, err := os.Stat(filepath.Join(absDir, "node_modules", "typescript")); err != nil {
		t.Skip("testdata/crap/typescript has no installed typescript package — run `npm install typescript@5` there")
	}

	covPath := writeCoverageFixture(t, absFile)

	fns, err := Analyzer{}.Analyze(analyzers.Options{Dir: dir, CoveragePath: covPath})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	cases := map[string]struct {
		complexity           int
		linesTotal, linesCov int
		uncovered            []crap.LineRange
	}{
		"simple":  {1, 1, 1, nil},
		"branchy": {2, 3, 2, []crap.LineRange{{Start: 11, End: 11}}},
		"loopy":   {4, 3, 2, []crap.LineRange{{Start: 18, End: 18}}},
	}

	if len(fns) != len(cases) {
		t.Fatalf("got %d functions, want %d: %+v", len(fns), len(cases), fns)
	}
	for _, f := range fns {
		want, ok := cases[f.Name]
		if !ok {
			t.Errorf("unexpected function %q", f.Name)
			continue
		}
		if f.Complexity != want.complexity {
			t.Errorf("%s complexity = %d, want %d", f.Name, f.Complexity, want.complexity)
		}
		if f.LinesTotal != want.linesTotal || f.LinesCovered != want.linesCov {
			t.Errorf("%s coverage = %d/%d, want %d/%d", f.Name, f.LinesCovered, f.LinesTotal, want.linesCov, want.linesTotal)
		}
		if !reflect.DeepEqual(f.UncoveredLines, want.uncovered) {
			t.Errorf("%s uncovered lines = %v, want %v", f.Name, f.UncoveredLines, want.uncovered)
		}
	}
}

// TestAnalyzeFallsBackToTypescript6ForTS7 verifies complexity.js's fallback
// to the official @typescript/typescript6 compat package when the target
// repo's installed `typescript` is v7+ and has no classic compiler API —
// see CLAUDE.md for why that fallback exists.
func TestAnalyzeFallsBackToTypescript6ForTS7(t *testing.T) {
	skipIfNoNode(t)

	dir := "../../../testdata/crap/typescript-ts7"
	absDir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(absDir, "node_modules", "@typescript", "typescript6")); err != nil {
		t.Skip("testdata/crap/typescript-ts7 has no installed fixtures — run `npm install` there")
	}

	fns, err := Analyzer{}.Analyze(analyzers.Options{Dir: dir})
	if err != nil {
		t.Fatalf("Analyze: %v (fallback to @typescript/typescript6 should have made this succeed even though typescript there is v7)", err)
	}
	if len(fns) != 1 || fns[0].Name != "branchy" {
		t.Fatalf("got %+v, want a single 'branchy' function", fns)
	}
	if fns[0].Complexity != 2 {
		t.Errorf("branchy complexity = %d, want 2", fns[0].Complexity)
	}
}
