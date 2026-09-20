package mutation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func parseFixture(t *testing.T, name string, opts Options) []Mutant {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "test", "mutation", name))
	if err != nil {
		t.Fatal(err)
	}
	mutants, err := Parse(data, opts)
	if err != nil {
		t.Fatalf("Parse(%s): %v", name, err)
	}
	return mutants
}

func tally(mutants []Mutant) map[Status]int {
	m := map[Status]int{}
	for _, x := range mutants {
		m[x.Status]++
	}
	return m
}

func TestParseStryker(t *testing.T) {
	got := tally(parseFixture(t, "stryker.json", Options{}))
	want := map[Status]int{Killed: 1, Timeout: 1, Survived: 1, NoCoverage: 1, Ignored: 1}
	for s, n := range want {
		if got[s] != n {
			t.Errorf("status %s: got %d, want %d (all: %v)", s, got[s], n, got)
		}
	}
}

func TestParseStrykerRecordsFileLineAndMutator(t *testing.T) {
	for _, m := range parseFixture(t, "stryker.json", Options{}) {
		if m.Status == Survived {
			if m.File != "src/calc.ts" || m.Line != 7 || m.Mutator != "EqualityOperator" {
				t.Errorf("survivor = %+v, want src/calc.ts:7 EqualityOperator", m)
			}
			return
		}
	}
	t.Error("no survivor found")
}

func TestParseGremlins(t *testing.T) {
	got := tally(parseFixture(t, "gremlins.json", Options{}))
	want := map[Status]int{Killed: 1, Timeout: 1, Survived: 1, NoCoverage: 1, Ignored: 1}
	for s, n := range want {
		if got[s] != n {
			t.Errorf("status %s: got %d, want %d (all: %v)", s, got[s], n, got)
		}
	}
}

func TestParseGeneric(t *testing.T) {
	got := tally(parseFixture(t, "generic.json", Options{}))
	if got[Killed] != 1 || got[Survived] != 1 || got[Ignored] != 1 {
		t.Errorf("tally = %v", got)
	}
}

func TestParseRelativizesAbsolutePathsToDir(t *testing.T) {
	dir := t.TempDir()
	data := `{"mutants":[{"file":"` + filepath.ToSlash(filepath.Join(dir, "pkg", "a.py")) + `","line":1,"mutator":"m","status":"killed"}]}`
	mutants, err := Parse([]byte(data), Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if mutants[0].File != "pkg/a.py" {
		t.Errorf("File = %q, want pkg/a.py", mutants[0].File)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"not json":       `nope`,
		"unrecognized":   `{"something":"else"}`,
		"unknown status": `{"mutants":[{"file":"a","line":1,"mutator":"m","status":"maybe"}]}`,
	}
	for name, in := range cases {
		if _, err := Parse([]byte(in), Options{}); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestScoreCountsTimeoutAsDetectedAndIgnoresNoVerdict(t *testing.T) {
	// stryker fixture: killed 1 + timeout 1 detected; survived 1; no-coverage 1; compile-error ignored.
	r := NewReport(parseFixture(t, "stryker.json", Options{}), 60, false)
	if r.Detected != 2 || r.Survived != 1 || r.NoCoverage != 1 {
		t.Fatalf("counts = %d/%d/%d", r.Detected, r.Survived, r.NoCoverage)
	}
	if r.Score != 50 {
		t.Errorf("Score = %v, want 50 (2 of 4 with a verdict)", r.Score)
	}
	if r.Passed || r.ExitCode() != 1 {
		t.Error("50% should fail a 60% gate")
	}
}

func TestCoveredOnlyExcludesNoCoverageFromDenominator(t *testing.T) {
	r := NewReport(parseFixture(t, "stryker.json", Options{}), 60, true)
	if got := r.Score; got < 66.6 || got > 66.7 {
		t.Errorf("Score = %v, want 66.7 (2 of 3 covered)", got)
	}
	if !r.Passed || r.ExitCode() != 0 {
		t.Error("66.7% should pass a 60% gate")
	}
}

func TestNoVerdictMutantsScoreFull(t *testing.T) {
	if r := NewReport(nil, 100, false); r.Score != 100 || !r.Passed {
		t.Errorf("empty report = %+v, want score 100 and passing", r)
	}
}

func TestWriteTableListsMissesAndVerdict(t *testing.T) {
	var sb strings.Builder
	NewReport(parseFixture(t, "gremlins.json", Options{}), 90, false).WriteTable(&sb, 1)
	out := sb.String()
	for _, want := range []string{"mutation score 50.0%", "internal/calc/calc.go:14", "survived", "1 more not shown", "FAIL: 50.0% is below 90.0%"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}
}

func TestWriteTablePass(t *testing.T) {
	var sb strings.Builder
	NewReport(nil, 60, false).WriteTable(&sb, 0)
	if !strings.Contains(sb.String(), "PASS: 100.0% is at or above 60.0%") {
		t.Errorf("unexpected table:\n%s", sb.String())
	}
}
