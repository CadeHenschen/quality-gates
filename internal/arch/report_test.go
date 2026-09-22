package arch

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNewReportGate(t *testing.T) {
	violations := []Violation{{Rule: "r", FromPkg: "a", ToPkg: "b", File: "a/x.go", Import: "b/y.go"}}

	r := NewReport(2, violations, 1)
	if !r.Passed {
		t.Error("1 violation with --fail-above 1 should pass")
	}
	if r.ExitCode() != 0 {
		t.Errorf("ExitCode() = %d, want 0", r.ExitCode())
	}

	r = NewReport(2, violations, 0)
	if r.Passed {
		t.Error("1 violation with --fail-above 0 should fail")
	}
	if r.ExitCode() != 1 {
		t.Errorf("ExitCode() = %d, want 1", r.ExitCode())
	}
}

func TestWriteTablePass(t *testing.T) {
	var buf bytes.Buffer
	NewReport(3, nil, 0).WriteTable(&buf, 20)
	out := buf.String()
	if !strings.Contains(out, "PASS: no layer rule violations found") {
		t.Errorf("expected a clean-pass message, got:\n%s", out)
	}
}

func TestWriteTableFailAndTruncation(t *testing.T) {
	var violations []Violation
	for i := 0; i < 5; i++ {
		violations = append(violations, Violation{Rule: "r", FromPkg: "a", ToPkg: "b", File: "a/x.go", Import: "b/y.go"})
	}

	var buf bytes.Buffer
	NewReport(3, violations, 0).WriteTable(&buf, 2)
	out := buf.String()
	if !strings.Contains(out, "FAIL: 5 violation(s) exceeds 0") {
		t.Errorf("expected a FAIL summary, got:\n%s", out)
	}
	if !strings.Contains(out, "3 more violation(s) not shown") {
		t.Errorf("expected truncation note, got:\n%s", out)
	}
}

func TestWriteJSONRoundTrip(t *testing.T) {
	r := NewReport(4, []Violation{{Rule: "r", FromPkg: "a", ToPkg: "b", File: "a/x.go", Import: "b/y.go"}}, 0)

	var buf bytes.Buffer
	if err := r.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var decoded Report
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.FilesAnalyzed != 4 || len(decoded.Violations) != 1 || decoded.Passed {
		t.Errorf("round-tripped report = %+v, want FilesAnalyzed=4, 1 violation, Passed=false", decoded)
	}
}

func TestReadReportRoundTrip(t *testing.T) {
	r := NewReport(4, []Violation{{Rule: "r", FromPkg: "a", ToPkg: "b", File: "a/x.go", Import: "b/y.go"}}, 0)

	var buf bytes.Buffer
	if err := r.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	got, err := ReadReport(&buf)
	if err != nil {
		t.Fatalf("ReadReport: %v", err)
	}
	if got.FilesAnalyzed != 4 || len(got.Violations) != 1 || got.Violations[0].File != "a/x.go" {
		t.Errorf("ReadReport = %+v, want a round trip of the written report", got)
	}
}
