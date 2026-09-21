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

func TestMinMutantsWaivesTheGateOnTooFewVerdicts(t *testing.T) {
	// gremlins.json: 2 detected, 1 survived, 1 not covered = 4 graded, 50%.
	mutants := parseFixture(t, "gremlins.json", Options{})

	if r := NewReport(mutants, 60, false); r.Graded != 4 || r.Passed {
		t.Fatalf("baseline: graded=%d passed=%v, want 4 graded and a failing 50%%", r.Graded, r.Passed)
	}
	// At or below the graded count the score is still enforced.
	if r := NewReport(mutants, 60, false).WithMinMutants(4); r.Passed || r.Insufficient {
		t.Errorf("min 4 of 4 graded: passed=%v insufficient=%v, want enforced and failing", r.Passed, r.Insufficient)
	}
	// Above it, the score is noise: pass with a note.
	r := NewReport(mutants, 60, false).WithMinMutants(5)
	if !r.Passed || !r.Insufficient || r.MinMutants != 5 {
		t.Errorf("min 5 of 4 graded: %+v, want passed + insufficient", r)
	}
	var sb strings.Builder
	r.WriteTable(&sb, 0)
	if !strings.Contains(sb.String(), "PASS: only 4 graded mutants (fewer than --min-mutants 5)") {
		t.Errorf("table = %q", sb.String())
	}
	// 0 (the default) never waives anything.
	if r := NewReport(mutants, 60, false).WithMinMutants(0); r.Passed || r.Insufficient {
		t.Errorf("min 0: %+v, want enforced", r)
	}
}

func TestMinMutantsCountsOnlyWhatTheScoreCounts(t *testing.T) {
	// --covered-only drops the never-executed mutant from the score, so it
	// must drop out of the graded count too: 3 graded, not 4.
	mutants := parseFixture(t, "gremlins.json", Options{})
	r := NewReport(mutants, 60, true)
	if r.Graded != 3 {
		t.Fatalf("covered-only graded = %d, want 3", r.Graded)
	}
	if r := r.WithMinMutants(4); !r.Insufficient {
		t.Errorf("min 4 of 3 covered-only graded: %+v, want insufficient", r)
	}
}

func TestMinMutantsNothingGradedStaysPassing(t *testing.T) {
	r := NewReport(nil, 60, false).WithMinMutants(10)
	if !r.Passed || r.Graded != 0 {
		t.Errorf("empty report: %+v", r)
	}
}
