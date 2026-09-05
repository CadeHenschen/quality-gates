package python

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

func skipIfNoRadon(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("radon"); err != nil {
		t.Skip("radon not on PATH — install with `pip install radon` to run this test")
	}
}

// writeCoverageFixture builds a coverage.json (the shape `coverage json`
// produces) keyed by exactly the path radon will report for sample.py
// under dir, so the fixture stays correct regardless of how dir's
// relative-path string happens to be spelled.
func writeCoverageFixture(t *testing.T, dir string) string {
	t.Helper()

	type fileCov struct {
		ExecutedLines []int `json:"executed_lines"`
		MissingLines  []int `json:"missing_lines"`
	}
	doc := struct {
		Files map[string]fileCov `json:"files"`
	}{
		Files: map[string]fileCov{
			filepath.Join(dir, "sample.py"): {
				ExecutedLines: []int{5, 9, 10, 16, 17, 18},
				MissingLines:  []int{11, 19, 20},
			},
		},
	}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "coverage.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAnalyzeComplexityAndCoverage(t *testing.T) {
	skipIfNoRadon(t)

	dir := "../../../testdata/crap/python"
	covPath := writeCoverageFixture(t, dir)

	fns, err := Analyzer{}.Analyze(analyzers.Options{Dir: dir, CoveragePath: covPath})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	byName := map[string]crap.Function{}
	for _, f := range fns {
		byName[f.Name] = f
	}

	cases := []struct {
		name                 string
		complexity           int
		linesTotal, linesCov int
		uncovered            []crap.LineRange
	}{
		{"simple", 1, 1, 1, nil},                                     // line 5, executed
		{"branchy", 2, 3, 2, []crap.LineRange{{Start: 11, End: 11}}}, // lines 9,10,11 measured; 11 missing
		{"loopy", 3, 5, 3, []crap.LineRange{{Start: 19, End: 20}}},   // lines 16-20 measured; 19,20 missing
	}

	if len(fns) != len(cases) {
		t.Fatalf("got %d functions (%v), want %d", len(fns), byName, len(cases))
	}

	for _, c := range cases {
		got, ok := byName[c.name]
		if !ok {
			t.Errorf("function %q not found (got: %v)", c.name, byName)
			continue
		}
		if got.Complexity != c.complexity {
			t.Errorf("%s complexity = %d, want %d", c.name, got.Complexity, c.complexity)
		}
		if got.LinesTotal != c.linesTotal || got.LinesCovered != c.linesCov {
			t.Errorf("%s coverage = %d/%d, want %d/%d", c.name, got.LinesCovered, got.LinesTotal, c.linesCov, c.linesTotal)
		}
		if !reflect.DeepEqual(got.UncoveredLines, c.uncovered) {
			t.Errorf("%s uncovered lines = %v, want %v", c.name, got.UncoveredLines, c.uncovered)
		}
	}
}

func TestAnalyzeWithoutCoverageReport(t *testing.T) {
	skipIfNoRadon(t)

	fns, err := Analyzer{}.Analyze(analyzers.Options{Dir: "../../../testdata/crap/python"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(fns) != 3 {
		t.Fatalf("got %d functions, want 3", len(fns))
	}
	for _, f := range fns {
		if f.LinesTotal != 0 || f.LinesCovered != 0 {
			t.Errorf("%s: got LinesTotal=%d LinesCovered=%d without a coverage report, want 0/0", f.Name, f.LinesTotal, f.LinesCovered)
		}
	}
}

func TestFlattenDropsClassEntries(t *testing.T) {
	entries := []radonEntry{
		{Type: "class", Name: "Widget", Methods: []radonEntry{{Type: "method", Name: "loopy"}}},
		{Type: "method", Name: "loopy"},
		{Type: "function", Name: "branchy"},
	}

	got := flatten(entries)
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2 (class entries should be dropped): %v", len(got), got)
	}
	for _, e := range got {
		if e.Type == "class" {
			t.Errorf("class entry leaked through flatten: %+v", e)
		}
	}
}
