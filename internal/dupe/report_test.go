package dupe

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/reportio"
)

func TestWriteTableNoClones(t *testing.T) {
	report := NewReport(nil, nil, 5)
	var buf bytes.Buffer
	report.WriteTable(&buf, 20)
	out := buf.String()
	if !strings.Contains(out, "PASS: no duplication found") {
		t.Errorf("expected a PASS line, got:\n%s", out)
	}
}

func TestWriteTableWithClonesAndTruncation(t *testing.T) {
	files := []FileTokens{{File: "a.go", Tokens: toks("1", "2", "3")}}
	clones := []Clone{
		{FileA: "a.go", StartLineA: 1, EndLineA: 2, FileB: "b.go", StartLineB: 1, EndLineB: 2, Tokens: 50},
		{FileA: "a.go", StartLineA: 5, EndLineA: 6, FileB: "c.go", StartLineB: 5, EndLineB: 6, Tokens: 40},
		{FileA: "a.go", StartLineA: 9, EndLineA: 10, FileB: "d.go", StartLineB: 9, EndLineB: 10, Tokens: 30},
	}
	report := NewReport(files, clones, 1000) // huge fail-above so it passes despite clones existing

	var buf bytes.Buffer
	report.WriteTable(&buf, 2)
	out := buf.String()

	if !strings.Contains(out, "b.go:1-2") || !strings.Contains(out, "c.go:5-6") {
		t.Errorf("expected the two largest clones shown, got:\n%s", out)
	}
	if strings.Contains(out, "d.go:9-10") {
		t.Errorf("expected the smallest clone truncated out of top-2, got:\n%s", out)
	}
	if !strings.Contains(out, "1 more block(s) not shown") {
		t.Errorf("expected a truncation summary, got:\n%s", out)
	}
	if !strings.Contains(out, "PASS") {
		t.Errorf("expected PASS given the huge fail-above, got:\n%s", out)
	}
}

func TestWriteTableFail(t *testing.T) {
	files := []FileTokens{{File: "a.go", Tokens: toks("1", "2", "3", "4", "5", "6", "7", "8", "9", "10")}}
	clones := []Clone{{FileA: "a.go", StartLineA: 1, EndLineA: 5, FileB: "b.go", StartLineB: 1, EndLineB: 5, Tokens: 20}}
	report := NewReport(files, clones, 1)

	var buf bytes.Buffer
	report.WriteTable(&buf, 20)
	if out := buf.String(); !strings.Contains(out, "FAIL:") {
		t.Errorf("expected a FAIL line, got:\n%s", out)
	}
}

func TestWriteJSONRoundTrip(t *testing.T) {
	files := []FileTokens{{File: "a.go", Tokens: toks("1", "2", "3")}}
	clones := []Clone{{FileA: "a.go", StartLineA: 1, EndLineA: 2, FileB: "b.go", StartLineB: 1, EndLineB: 2, Tokens: 10}}
	report := NewReport(files, clones, 5)

	var buf bytes.Buffer
	if err := report.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	decoded, err := reportio.ReadReport[Report](bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("ReadReport: %v", err)
	}
	if len(decoded.Clones) != 1 || decoded.Clones[0].FileA != "a.go" {
		t.Errorf("round-tripped report = %+v", decoded)
	}
	if decoded.FailAbove != 5 {
		t.Errorf("FailAbove = %v, want 5", decoded.FailAbove)
	}

	// Also sanity-check it's valid, expected JSON shape.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("raw decode: %v", err)
	}
	for _, key := range []string{"clones", "total_lines", "duplicated_lines", "duplication_percent", "fail_above", "passed"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("JSON missing key %q", key)
		}
	}
}
