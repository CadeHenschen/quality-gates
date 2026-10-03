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

func TestMaxLineLengthCountsCodePointsAndReportsLine(t *testing.T) {
	length, line := MaxLineLength([]byte("ab\r\n\t界🙂xyz\n"))
	if length != 6 || line != 2 {
		t.Errorf("MaxLineLength() = (%d, %d), want (6, 2)", length, line)
	}
	if length, line := MaxLineLength(nil); length != 0 || line != 0 {
		t.Errorf("MaxLineLength(nil) = (%d, %d), want (0, 0)", length, line)
	}
}

func TestPhysicalLineCount(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		want   int
	}{
		{name: "empty", source: "", want: 0},
		{name: "unterminated", source: "one", want: 1},
		{name: "terminated", source: "one\n", want: 1},
		{name: "two lines", source: "one\ntwo", want: 2},
		{name: "blank trailing line", source: "one\n\n", want: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := PhysicalLineCount([]byte(tc.source)); got != tc.want {
				t.Errorf("PhysicalLineCount(%q) = %d, want %d", tc.source, got, tc.want)
			}
		})
	}
}

func TestSizeFindingsFlagsEachThreshold(t *testing.T) {
	fns := []Function{
		{File: "a.go", Name: "long", StartLine: 1, EndLine: 100, FileLines: 50}, // 100 lines
		{File: "a.go", Name: "manyParams", StartLine: 200, EndLine: 201, ParamCount: 9, FileLines: 50},
		{File: "a.go", Name: "deep", StartLine: 300, EndLine: 301, MaxNestingDepth: 7, FileLines: 50},
		{File: "b.go", Name: "fine", StartLine: 1, EndLine: 2, ParamCount: 1, FileLines: 900},
		{File: "b.go", Name: "alsoInB", StartLine: 10, EndLine: 11, FileLines: 900, FileMaxLineLength: 170, FileMaxLineLengthLine: 12},
		{File: "b.go", Name: "thirdInB", StartLine: 20, EndLine: 21, FileLines: 900, FileMaxLineLength: 170, FileMaxLineLengthLine: 12},
	}
	th := SizeThresholds{MaxLines: 80, MaxParams: 6, MaxNesting: 4, MaxFileLines: 600, MaxLineLength: 150}

	got := sizeFindings(fns, nil, th)

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
	if kinds["file_line_length"] != 1 {
		t.Errorf("file_line_length findings = %d, want 1 (deduplicated across b.go's three functions)", kinds["file_line_length"])
	}
	for _, finding := range got {
		if finding.Kind == "file_line_length" && (finding.StartLine != 12 || finding.Value != 170 || finding.Threshold != 150) {
			t.Errorf("file_line_length finding = %+v, want line 12, width 170, threshold 150", finding)
		}
	}
}

func TestSizeFindingsThresholdDisabledByZeroOrNegative(t *testing.T) {
	fns := []Function{
		{File: "a.go", Name: "huge", StartLine: 1, EndLine: 1000, ParamCount: 20, MaxNestingDepth: 20, FileLines: 5000, FileMaxLineLength: 500, FileMaxLineLengthLine: 1},
	}
	if got := sizeFindings(fns, nil, SizeThresholds{}); len(got) != 0 {
		t.Errorf("zero-value SizeThresholds should disable every check, got %v", got)
	}
	if got := sizeFindings(fns, nil, SizeThresholds{MaxLines: -1, MaxParams: -1, MaxNesting: -1, MaxFileLines: -1, MaxLineLength: -1}); len(got) != 0 {
		t.Errorf("negative thresholds should disable every check, got %v", got)
	}
}

func TestMaxLineLengthThresholdAllowsExactLimit(t *testing.T) {
	fns := []Function{{File: "a.go", Name: "f", FileMaxLineLength: 150, FileMaxLineLengthLine: 4}}
	if got := sizeFindings(fns, nil, SizeThresholds{MaxLineLength: 150}); len(got) != 0 {
		t.Errorf("a line exactly at the limit should pass, got %+v", got)
	}
}

func TestSizeFindingsIncludesFunctionlessFiles(t *testing.T) {
	got := sizeFindings(nil, []FileSize{{
		File: "types.go", Lines: 601, MaxLineLength: 151, MaxLineLengthLine: 8,
	}}, SizeThresholds{MaxFileLines: 600, MaxLineLength: 150})
	if len(got) != 2 {
		t.Fatalf("sizeFindings = %+v, want one finding for each file-wide threshold", got)
	}
	if got[0] != (SizeFinding{File: "types.go", Kind: "file_lines", Value: 601, Threshold: 600}) {
		t.Errorf("line-count finding = %+v", got[0])
	}
	if got[1] != (SizeFinding{File: "types.go", StartLine: 8, Kind: "file_line_length", Value: 151, Threshold: 150}) {
		t.Errorf("line-length finding = %+v", got[1])
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
	if strings.Contains(out, "FUNCTION") || strings.Contains(out, "COMPLEXITY") {
		t.Errorf("size-only failure should not print a passing CRAP hotspot table, got:\n%s", out)
	}
	if !strings.Contains(out, "FAIL: 1 size threshold violation(s)") {
		t.Errorf("expected a size FAIL line, got:\n%s", out)
	}
	if !strings.Contains(out, "SIZE:") || !strings.Contains(out, "200 lines (> 80)") {
		t.Errorf("expected a SIZE section naming the violation, got:\n%s", out)
	}
}

func TestWithSizePrintsMaxLineLength(t *testing.T) {
	report := NewReport([]Function{{
		File: "b.go", Name: "f", Complexity: 1, LinesTotal: 1, LinesCovered: 1,
		StartLine: 1, EndLine: 1, FileMaxLineLength: 170, FileMaxLineLengthLine: 12,
	}}, 30).WithSize(SizeThresholds{MaxLineLength: 150})
	if report.Passed || len(report.SizeFindings) != 1 {
		t.Fatalf("report = passed %v, findings %+v; want one failing line-length finding", report.Passed, report.SizeFindings)
	}
	var buf bytes.Buffer
	report.WriteTable(&buf, 20, false)
	if !strings.Contains(buf.String(), "b.go:12: longest line is 170 characters (> 150)") {
		t.Errorf("expected a location-bearing line-length finding, got:\n%s", buf.String())
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
