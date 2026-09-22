package crap

import (
	"bytes"
	"strings"
	"testing"
)

func TestLineCount(t *testing.T) {
	f := Function{StartLine: 10, EndLine: 19}
	if got, want := f.LineCount(), 10; got != want {
		t.Errorf("LineCount() = %d, want %d", got, want)
	}
}

func TestSizeFindingsFlagsEachThreshold(t *testing.T) {
	fns := []Function{
		{File: "a.go", Name: "long", StartLine: 1, EndLine: 100, FileLines: 50}, // 100 lines
		{File: "a.go", Name: "manyParams", StartLine: 200, EndLine: 201, ParamCount: 9, FileLines: 50},
		{File: "a.go", Name: "deep", StartLine: 300, EndLine: 301, MaxNestingDepth: 7, FileLines: 50},
		{File: "b.go", Name: "fine", StartLine: 1, EndLine: 2, ParamCount: 1, FileLines: 900},
		{File: "b.go", Name: "alsoInB", StartLine: 10, EndLine: 11, FileLines: 900},
	}
	th := SizeThresholds{MaxLines: 80, MaxParams: 6, MaxNesting: 4, MaxFileLines: 600}

	got := sizeFindings(fns, th)

	kinds := map[string]int{}
	for _, f := range got {
		kinds[f.Kind]++
	}
	if kinds["lines"] != 1 {
		t.Errorf("lines findings = %d, want 1", kinds["lines"])
	}
	if kinds["params"] != 1 {
		t.Errorf("params findings = %d, want 1", kinds["params"])
	}
	if kinds["nesting"] != 1 {
		t.Errorf("nesting findings = %d, want 1", kinds["nesting"])
	}
	// b.go is 900 lines but has two functions — the file-length finding
	// must be deduplicated to exactly one, not one per function.
	if kinds["file_lines"] != 1 {
		t.Errorf("file_lines findings = %d, want 1 (deduplicated across b.go's two functions)", kinds["file_lines"])
	}
}

func TestSizeFindingsThresholdDisabledByZeroOrNegative(t *testing.T) {
	fns := []Function{
		{File: "a.go", Name: "huge", StartLine: 1, EndLine: 1000, ParamCount: 20, MaxNestingDepth: 20, FileLines: 5000},
	}
	if got := sizeFindings(fns, SizeThresholds{}); len(got) != 0 {
		t.Errorf("zero-value SizeThresholds should disable every check, got %v", got)
	}
	if got := sizeFindings(fns, SizeThresholds{MaxLines: -1, MaxParams: -1, MaxNesting: -1, MaxFileLines: -1}); len(got) != 0 {
		t.Errorf("negative thresholds should disable every check, got %v", got)
	}
}

func TestWithSizeFailsReportAndPreservesCrapPass(t *testing.T) {
	report := NewReport([]Function{
		{File: "a.go", Name: "big", Complexity: 1, LinesTotal: 10, LinesCovered: 10, StartLine: 1, EndLine: 200},
	}, 30) // passes CRAP (score 1, well under 30)

	if !report.Passed {
		t.Fatal("expected the un-sized report to pass CRAP")
	}

	sized := report.WithSize(SizeThresholds{MaxLines: 80})
	if sized.Passed {
		t.Error("expected WithSize to fail the report once a function's length crosses MaxLines")
	}
	if len(sized.SizeFindings) != 1 || sized.SizeFindings[0].Kind != "lines" {
		t.Errorf("SizeFindings = %+v, want one 'lines' finding", sized.SizeFindings)
	}

	var buf bytes.Buffer
	sized.WriteTable(&buf, 20, false)
	out := buf.String()
	if !strings.Contains(out, "PASS: no function exceeds CRAP 30.0") {
		t.Errorf("CRAP verdict line should still say PASS, got:\n%s", out)
	}
	if !strings.Contains(out, "FAIL: 1 size threshold violation(s)") {
		t.Errorf("expected a size FAIL line, got:\n%s", out)
	}
	if !strings.Contains(out, "SIZE:") || !strings.Contains(out, "200 lines (> 80)") {
		t.Errorf("expected a SIZE section naming the violation, got:\n%s", out)
	}
}

func TestWithSizePassesCleanReport(t *testing.T) {
	report := NewReport([]Function{
		{File: "a.go", Name: "fine", Complexity: 1, LinesTotal: 10, LinesCovered: 10, StartLine: 1, EndLine: 5},
	}, 30).WithSize(SizeThresholds{MaxLines: 80, MaxParams: 6, MaxNesting: 4, MaxFileLines: 600})

	if !report.Passed {
		t.Errorf("expected a clean report to still pass, got SizeFindings = %+v", report.SizeFindings)
	}

	var buf bytes.Buffer
	report.WriteTable(&buf, 20, false)
	out := buf.String()
	if strings.Contains(out, "SIZE:") {
		t.Errorf("no SIZE section expected when nothing crossed a threshold, got:\n%s", out)
	}
	if !strings.Contains(out, "PASS: within size thresholds") {
		t.Errorf("expected an explicit size PASS line since thresholds were configured, got:\n%s", out)
	}
}

func TestWriteTableWithoutWithSizeOmitsSizeOutput(t *testing.T) {
	report := NewReport([]Function{
		{File: "a.go", Name: "f", Complexity: 1, LinesTotal: 10, LinesCovered: 10, StartLine: 1, EndLine: 1},
	}, 30)

	var buf bytes.Buffer
	report.WriteTable(&buf, 20, false)
	out := buf.String()
	if strings.Contains(out, "SIZE") {
		t.Errorf("a report nobody called WithSize on shouldn't mention size at all, got:\n%s", out)
	}
}
