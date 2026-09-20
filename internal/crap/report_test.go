package crap

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	NewReport(nil, 30).WriteTable(&buf, 20, false)

	if got := buf.String(); got != "no functions analyzed\n" {
		t.Errorf("got %q, want %q", got, "no functions analyzed\n")
	}
}

func TestWriteTablePass(t *testing.T) {
	report := NewReport([]Function{
		{File: "a.go", Name: "f", StartLine: 1, Complexity: 1, LinesTotal: 10, LinesCovered: 10},
	}, 30)

	var buf bytes.Buffer
	report.WriteTable(&buf, 20, false)
	out := buf.String()

	for _, want := range []string{"a.go:1", "f", "FILE", "FUNCTION", "PASS: no function exceeds CRAP 30.0"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "FAIL") {
		t.Errorf("passing report shouldn't mention FAIL, got:\n%s", out)
	}
}

func TestWriteTableFailAndTruncation(t *testing.T) {
	fns := []Function{
		{Name: "risky", Complexity: 10, LinesTotal: 10, LinesCovered: 0}, // crap 110
		{Name: "mid", Complexity: 4, LinesTotal: 10, LinesCovered: 5},    // crap 6
		{Name: "safe", Complexity: 1, LinesTotal: 10, LinesCovered: 10},  // crap 1
	}
	report := NewReport(fns, 30)

	var buf bytes.Buffer
	report.WriteTable(&buf, 2, false) // top=2, so "safe" should be truncated
	out := buf.String()

	if !strings.Contains(out, "risky") || !strings.Contains(out, "mid") {
		t.Errorf("expected the two worst functions in output, got:\n%s", out)
	}
	if strings.Contains(out, "safe") {
		t.Errorf("expected 'safe' to be truncated out of top-2 output, got:\n%s", out)
	}
	if !strings.Contains(out, "1 more function(s) not shown") {
		t.Errorf("expected a truncation summary line, got:\n%s", out)
	}
	if !strings.Contains(out, "FAIL: at least one function exceeds CRAP 30.0") {
		t.Errorf("expected a FAIL line, got:\n%s", out)
	}
}

func TestWriteTableTopZeroShowsAll(t *testing.T) {
	fns := make([]Function, 30)
	for i := range fns {
		fns[i] = Function{Name: "f", Complexity: 1, LinesTotal: 1, LinesCovered: 1}
	}
	report := NewReport(fns, 30)

	var buf bytes.Buffer
	report.WriteTable(&buf, 0, false)
	out := buf.String()

	if strings.Contains(out, "more function(s) not shown") {
		t.Errorf("top=0 should show every row untruncated, got:\n%s", out)
	}
	if got := strings.Count(out, "f "); got == 0 {
		t.Errorf("expected function rows in output, got:\n%s", out)
	}
}

func TestWriteTableVerboseShowsUncoveredLines(t *testing.T) {
	report := NewReport([]Function{
		{
			File: "a.go", Name: "risky", Complexity: 5, LinesTotal: 10, LinesCovered: 5,
			UncoveredLines: []LineRange{{Start: 12, End: 15}, {Start: 20, End: 20}},
		},
		{File: "a.go", Name: "clean", Complexity: 1, LinesTotal: 5, LinesCovered: 5},
	}, 30)

	var buf bytes.Buffer
	report.WriteTable(&buf, 20, true)
	out := buf.String()

	if !strings.Contains(out, "uncovered: 12-15, 20") {
		t.Errorf("expected an uncovered-lines line for 'risky', got:\n%s", out)
	}
	// "clean" has no uncovered lines, so it shouldn't get an uncovered: line
	// of its own — count should be exactly 1 across the whole table.
	if got := strings.Count(out, "uncovered:"); got != 1 {
		t.Errorf("expected exactly 1 'uncovered:' line, got %d in:\n%s", got, out)
	}
}

func TestWriteTableNotVerboseHidesUncoveredLines(t *testing.T) {
	report := NewReport([]Function{
		{File: "a.go", Name: "risky", Complexity: 5, LinesTotal: 10, LinesCovered: 5,
			UncoveredLines: []LineRange{{Start: 12, End: 15}}},
	}, 30)

	var buf bytes.Buffer
	report.WriteTable(&buf, 20, false)
	if out := buf.String(); strings.Contains(out, "uncovered:") {
		t.Errorf("non-verbose output shouldn't show uncovered lines, got:\n%s", out)
	}
}

func TestWriteJSON(t *testing.T) {
	report := NewReport([]Function{
		{File: "a.go", Name: "f", Complexity: 2, LinesTotal: 10, LinesCovered: 5},
	}, 30)

	var buf bytes.Buffer
	if err := report.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var decoded Report
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("decode: %v (raw: %s)", err, buf.String())
	}
	if len(decoded.Functions) != 1 || decoded.Functions[0].Name != "f" {
		t.Errorf("round-tripped report = %+v, want one function named %q", decoded, "f")
	}
	if decoded.FailAbove != 30 {
		t.Errorf("FailAbove = %v, want 30", decoded.FailAbove)
	}
}

func TestWriteTableFlagsUnmeasured(t *testing.T) {
	report := NewReport([]Function{
		{File: "b.go", Name: "orphan", StartLine: 3, Complexity: 40, Unmeasured: true},
		{File: "a.go", Name: "ok", StartLine: 1, Complexity: 1, LinesTotal: 4, LinesCovered: 4},
	}, 30)

	var buf bytes.Buffer
	report.WriteTable(&buf, 20, false)
	out := buf.String()

	for _, want := range []string{"none", "WARNING: 1 function(s) in 1 file(s) are absent from the coverage report", "  b.go\n", "FAIL"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
}
