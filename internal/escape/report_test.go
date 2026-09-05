package escape

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

func TestNewReportRateAndGate(t *testing.T) {
	hatches := []Hatch{
		{File: "b.go", Line: 5, Pattern: "nolint"},
		{File: "a.go", Line: 10, Pattern: "nolint"},
		{File: "a.go", Line: 2, Pattern: "noqa"},
	}
	report := NewReport(hatches, 1000, 5) // 3 hatches / 1000 lines * 1000 = 3.0 rate
	if report.Rate != 3.0 {
		t.Errorf("Rate = %v, want 3.0", report.Rate)
	}
	if !report.Passed || report.ExitCode() != 0 {
		t.Errorf("expected pass (3.0 <= 5), got Passed=%v ExitCode=%d", report.Passed, report.ExitCode())
	}

	// Sorted by file then line.
	want := []string{"a.go:2", "a.go:10", "b.go:5"}
	for i, h := range report.Hatches {
		got := h.File + ":" + strconv.Itoa(h.Line)
		if got != want[i] {
			t.Errorf("Hatches[%d] = %s, want %s", i, got, want[i])
		}
	}

	failing := NewReport(hatches, 1000, 1)
	if failing.Passed || failing.ExitCode() != 1 {
		t.Errorf("expected fail (3.0 > 1), got Passed=%v ExitCode=%d", failing.Passed, failing.ExitCode())
	}
}

func TestNewReportZeroLinesNoDivideByZero(t *testing.T) {
	report := NewReport(nil, 0, 1)
	if report.Rate != 0 || !report.Passed {
		t.Errorf("report = %+v, want zero rate and Passed=true", report)
	}
}

func TestWriteTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	NewReport(nil, 100, 5).WriteTable(&buf, 20)
	if out := buf.String(); !strings.Contains(out, "PASS: no escape hatches found") {
		t.Errorf("got %q", out)
	}
}

func TestWriteTableGroupsByPatternAndTruncates(t *testing.T) {
	hatches := make([]Hatch, 5)
	for i := range hatches {
		hatches[i] = Hatch{File: "a.go", Line: i + 1, Pattern: "nolint", Text: "x"}
	}
	report := NewReport(hatches, 1000, 1000) // huge fail-above so it passes

	var buf bytes.Buffer
	report.WriteTable(&buf, 2)
	out := buf.String()

	if !strings.Contains(out, "nolint") {
		t.Errorf("expected pattern breakdown, got:\n%s", out)
	}
	if !strings.Contains(out, "3 more not shown") {
		t.Errorf("expected truncation summary, got:\n%s", out)
	}
	if !strings.Contains(out, "PASS") {
		t.Errorf("expected PASS given huge fail-above, got:\n%s", out)
	}
}

func TestWriteJSONRoundTrip(t *testing.T) {
	hatches := []Hatch{{File: "a.go", Line: 1, Pattern: "nolint", Text: "x //nolint"}}
	report := NewReport(hatches, 100, 5)

	var buf bytes.Buffer
	if err := report.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if !strings.Contains(buf.String(), `"pattern": "nolint"`) {
		t.Errorf("JSON missing expected field, got:\n%s", buf.String())
	}
}
