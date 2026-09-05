package cycle

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteTableNoCycles(t *testing.T) {
	report := NewReport(5, nil, 0)
	var buf bytes.Buffer
	report.WriteTable(&buf, 20)
	if out := buf.String(); !strings.Contains(out, "PASS: no import cycles found") {
		t.Errorf("got %q", out)
	}
}

func TestWriteTableWithCycles(t *testing.T) {
	cycles := []Cycle{
		{Files: []string{"a.py", "b.py"}, Chain: []string{"a.py", "b.py", "a.py"}},
	}
	report := NewReport(10, cycles, 0)

	var buf bytes.Buffer
	report.WriteTable(&buf, 20)
	out := buf.String()
	if !strings.Contains(out, "a.py -> b.py -> a.py") {
		t.Errorf("expected the chain in output, got:\n%s", out)
	}
	if !strings.Contains(out, "FAIL: 1 cycle(s) exceeds 0") {
		t.Errorf("expected a FAIL line, got:\n%s", out)
	}
}

func TestWriteTableTruncation(t *testing.T) {
	cycles := make([]Cycle, 5)
	for i := range cycles {
		cycles[i] = Cycle{Files: []string{"a.py", "b.py"}, Chain: []string{"a.py", "b.py", "a.py"}}
	}
	report := NewReport(10, cycles, 100) // huge fail-above so it passes despite cycles existing

	var buf bytes.Buffer
	report.WriteTable(&buf, 2)
	out := buf.String()
	if !strings.Contains(out, "3 more cycle(s) not shown") {
		t.Errorf("expected truncation summary, got:\n%s", out)
	}
	if !strings.Contains(out, "PASS") {
		t.Errorf("expected PASS given huge fail-above, got:\n%s", out)
	}
}

func TestNewReportGate(t *testing.T) {
	if !NewReport(5, nil, 0).Passed {
		t.Error("expected Passed=true with 0 cycles and fail-above 0")
	}
	cycles := []Cycle{{Files: []string{"a.py", "b.py"}}}
	if NewReport(5, cycles, 0).Passed {
		t.Error("expected Passed=false with 1 cycle and fail-above 0")
	}
	if !NewReport(5, cycles, 1).Passed {
		t.Error("expected Passed=true with 1 cycle and fail-above 1")
	}
}

func TestWriteJSON(t *testing.T) {
	report := NewReport(5, []Cycle{{Files: []string{"a.py", "b.py"}, Chain: []string{"a.py", "b.py", "a.py"}}}, 0)
	var buf bytes.Buffer
	if err := report.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if !strings.Contains(buf.String(), `"a.py"`) {
		t.Errorf("JSON missing expected content, got:\n%s", buf.String())
	}
}
