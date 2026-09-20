package testmetric

import (
	"bytes"
	"strings"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/reportio"
)

func kinds(fs []Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Kind)
	}
	return out
}

func TestAnalyzeEachCheck(t *testing.T) {
	cases := []struct {
		name string
		test Test
		opts Options
		want []string
	}{
		{"healthy", Test{Name: "a", Assertions: 2, Interactions: 1}, Options{}, nil},
		{"no assertions", Test{Name: "a"}, Options{}, []string{KindNoAssertions}},
		{"low assertions", Test{Name: "a", Assertions: 1}, Options{MinAssertions: 2}, []string{KindLowAssertions}},
		{"interaction only", Test{Name: "a", Assertions: 2, Interactions: 2}, Options{}, []string{KindInteractionOnly}},
		{"skipped suppresses body checks", Test{Name: "a", Skipped: true}, Options{}, []string{KindSkipped}},
		{"focused", Test{Name: "a", Focused: true, Assertions: 1}, Options{}, []string{KindFocused}},
		{"expected failure", Test{Name: "a", ExpectedFail: true, Assertions: 1}, Options{}, []string{KindExpectedFailure}},
		{"temp leak", Test{Name: "a", Assertions: 1, UncleanedTemp: true}, Options{}, []string{KindTempNoCleanup}},
		{"suite only reports skip/focus", Test{Name: "s", Suite: true, Skipped: true, Focused: true}, Options{}, []string{KindSkipped, KindFocused}},
		{"suite with no assertions is fine", Test{Name: "s", Suite: true}, Options{}, nil},
		{"ignore disables a check", Test{Name: "a"}, Options{Ignore: map[string]bool{KindNoAssertions: true}}, nil},
	}
	for _, c := range cases {
		got := kinds(Analyze([]Test{c.test}, c.opts))
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s: kinds = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestAnalyzeSortsByFileThenLine(t *testing.T) {
	got := Analyze([]Test{
		{File: "b.go", Line: 1, Name: "b1"},
		{File: "a.go", Line: 9, Name: "a9"},
		{File: "a.go", Line: 2, Name: "a2"},
	}, Options{})
	var names []string
	for _, f := range got {
		names = append(names, f.Test)
	}
	if strings.Join(names, ",") != "a2,a9,b1" {
		t.Errorf("order = %v", names)
	}
}

func TestCountsExcludeSuites(t *testing.T) {
	n, a := Counts([]Test{{Assertions: 2}, {Assertions: 3}, {Suite: true, Assertions: 99}})
	if n != 2 || a != 5 {
		t.Errorf("Counts = %d, %d; want 2, 5", n, a)
	}
}

func TestReportGateAndJSONRoundTrip(t *testing.T) {
	findings := []Finding{{File: "a.go", Line: 1, Test: "T", Kind: KindNoAssertions, Detail: "d"}}

	if r := NewReport(findings, 4, 6, 0); r.Passed || r.ExitCode() != 1 {
		t.Error("1 finding should fail --fail-above 0")
	}
	r := NewReport(findings, 4, 6, 1)
	if !r.Passed || r.ExitCode() != 0 {
		t.Error("1 finding should pass --fail-above 1")
	}

	var buf bytes.Buffer
	if err := r.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	back, err := reportio.ReadReport[Report](&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Findings) != 1 || back.Tests != 4 || back.Assertions != 6 || !back.Passed {
		t.Errorf("round-trip = %+v", back)
	}
}

func TestWriteTable(t *testing.T) {
	findings := []Finding{
		{File: "a.go", Line: 1, Test: "T1", Kind: KindSkipped, Detail: "d1"},
		{File: "a.go", Line: 2, Test: "T2", Kind: KindNoAssertions, Detail: "d2"},
	}
	var sb strings.Builder
	NewReport(findings, 2, 3, 0).WriteTable(&sb, 1)
	out := sb.String()
	for _, want := range []string{"2 test finding(s) across 2 test(s), 1.5 assertion(s) per test", "a.go:1  skipped  T1", "1 more not shown", "FAIL: 2 finding(s) exceeds 0"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}

	sb.Reset()
	NewReport(nil, 0, 0, 0).WriteTable(&sb, 0)
	if !strings.Contains(sb.String(), "PASS: no test-quality findings") {
		t.Errorf("clean table = %q", sb.String())
	}

	sb.Reset()
	NewReport(findings, 2, 3, 5).WriteTable(&sb, 0)
	if !strings.Contains(sb.String(), "PASS: 2 finding(s) is at or under 5") {
		t.Errorf("pass table = %q", sb.String())
	}
}
