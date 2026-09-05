package crap

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestDiff(t *testing.T) {
	old := NewReport([]Function{
		{File: "a.go", Name: "stable", Complexity: 1, LinesTotal: 10, LinesCovered: 10},    // crap 1
		{File: "a.go", Name: "getsWorse", Complexity: 2, LinesTotal: 10, LinesCovered: 10}, // crap 2
		{File: "a.go", Name: "getsRemoved", Complexity: 3, LinesTotal: 10, LinesCovered: 10},
	}, 30)
	new := NewReport([]Function{
		{File: "a.go", Name: "stable", Complexity: 1, LinesTotal: 10, LinesCovered: 10},   // crap 1, unchanged
		{File: "a.go", Name: "getsWorse", Complexity: 2, LinesTotal: 10, LinesCovered: 0}, // crap 6, regressed
		{File: "a.go", Name: "brandNew", Complexity: 4, LinesTotal: 10, LinesCovered: 0},  // crap 20, new
	}, 30)

	deltas := Diff(old, new)

	byName := map[string]Delta{}
	for _, d := range deltas {
		byName[d.Name] = d
	}

	if _, ok := byName["stable"]; ok {
		t.Errorf("unchanged function 'stable' shouldn't appear in the diff, got: %+v", deltas)
	}

	worse, ok := byName["getsWorse"]
	if !ok {
		t.Fatalf("expected 'getsWorse' in diff, got: %+v", deltas)
	}
	if worse.Change != "changed" || worse.OldCrap != 2 || worse.NewCrap != 6 {
		t.Errorf("getsWorse = %+v, want Change=changed OldCrap=2 NewCrap=6", worse)
	}

	added, ok := byName["brandNew"]
	if !ok || added.Change != "new" {
		t.Errorf("expected 'brandNew' with Change=new, got: %+v (found=%v)", added, ok)
	}

	removed, ok := byName["getsRemoved"]
	if !ok || removed.Change != "removed" || removed.OldCrap != 3 {
		t.Errorf("expected 'getsRemoved' with Change=removed OldCrap=3, got: %+v (found=%v)", removed, ok)
	}

	if len(deltas) != 3 {
		t.Fatalf("got %d deltas, want 3 (getsWorse, brandNew, getsRemoved): %+v", len(deltas), deltas)
	}

	// Worst regression/newest-highest-score first.
	if deltas[0].Name != "brandNew" {
		t.Errorf("deltas[0] = %q, want %q (highest new score sorts first)", deltas[0].Name, "brandNew")
	}
}

func TestWriteDiffTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	WriteDiffTable(&buf, nil, 20)
	if got := buf.String(); got != "no CRAP score changes\n" {
		t.Errorf("got %q, want %q", got, "no CRAP score changes\n")
	}
}

func TestWriteDiffTableFormatsEachChangeKind(t *testing.T) {
	deltas := []Delta{
		{File: "a.go", Name: "worse", OldCrap: 2, NewCrap: 10, Change: "changed"},
		{File: "a.go", Name: "better", OldCrap: 10, NewCrap: 2, Change: "changed"},
		{File: "a.go", Name: "brandNew", NewCrap: 5, Change: "new"},
		{File: "a.go", Name: "gone", OldCrap: 7, Change: "removed"},
	}
	var buf bytes.Buffer
	WriteDiffTable(&buf, deltas, 20)
	out := buf.String()

	for _, want := range []string{
		"2.0 -> 10.0 (+8.0)",
		"10.0 -> 2.0 (-8.0)",
		"new: 5.0",
		"removed (was 7.0)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
}

func TestWriteDiffTableTruncation(t *testing.T) {
	deltas := make([]Delta, 5)
	for i := range deltas {
		deltas[i] = Delta{File: "a.go", Name: "f", Change: "new", NewCrap: float64(i)}
	}
	var buf bytes.Buffer
	WriteDiffTable(&buf, deltas, 2)
	if out := buf.String(); !strings.Contains(out, "3 more change(s) not shown") {
		t.Errorf("expected truncation summary, got:\n%s", out)
	}
}

func TestWriteDiffJSON(t *testing.T) {
	deltas := []Delta{{File: "a.go", Name: "f", OldCrap: 1, NewCrap: 2, Change: "changed"}}
	var buf bytes.Buffer
	if err := WriteDiffJSON(&buf, deltas); err != nil {
		t.Fatalf("WriteDiffJSON: %v", err)
	}
	var decoded []Delta
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("decode: %v (raw: %s)", err, buf.String())
	}
	if len(decoded) != 1 || decoded[0].Name != "f" {
		t.Errorf("round-tripped = %+v, want one delta named f", decoded)
	}
}
