package deadcode

import (
	"bytes"
	"strings"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/reportio"
)

func sample() []Finding {
	return []Finding{
		{File: "b.go", Line: 5, Name: "two", Kind: KindFunction},
		{File: "a.go", Line: 9, Name: "one", Kind: KindFunction},
		{File: "a.go", Line: 2, Name: "zero", Kind: KindFunction},
	}
}

func TestNewReportGate(t *testing.T) {
	if r := NewReport(sample(), 3); !r.Passed || r.Count != 3 {
		t.Errorf("at the limit passes: %+v", r)
	}
	if r := NewReport(sample(), 2); r.Passed {
		t.Errorf("above the limit fails: %+v", r)
	}
	if r := NewReport(nil, 0); !r.Passed || r.ExitCode() != 0 {
		t.Errorf("nothing dead passes: %+v", r)
	}
	if r := NewReport(sample(), 0); r.ExitCode() != 1 {
		t.Errorf("failing report exits 1")
	}
}

func TestReportWriteTableSortsAndTruncates(t *testing.T) {
	var buf bytes.Buffer
	NewReport(sample(), 0).WriteTable(&buf, 2)
	out := buf.String()
	if strings.Index(out, "a.go:2") > strings.Index(out, "a.go:9") || strings.Contains(out, "b.go:5") {
		t.Errorf("want file/line order cut to top 2:\n%s", out)
	}
	for _, want := range []string{"3 dead", "... 1 more not shown", "FAIL: 3 dead symbol(s), gate is 0"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}

func TestReportWriteTablePass(t *testing.T) {
	var buf bytes.Buffer
	NewReport(nil, 0).WriteTable(&buf, 20)
	if !strings.Contains(buf.String(), "PASS: 0 dead symbol(s), gate is 0") {
		t.Errorf("got:\n%s", buf.String())
	}
}

func TestReportJSONRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	orig := NewReport(sample(), 1)
	if err := orig.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	back, err := reportio.ReadReport[Report](&buf)
	if err != nil || back.Count != 3 || back.FailAbove != 1 || back.Passed {
		t.Fatalf("round trip: %+v, %v", back, err)
	}
}
