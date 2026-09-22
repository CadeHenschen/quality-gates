package arch

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func v(rule, file, imp string) Violation {
	return Violation{Rule: rule, FromPkg: "a", ToPkg: "b", File: file, Import: imp}
}

func TestDiffNewAndFixed(t *testing.T) {
	old := NewReport(3, []Violation{
		v("r", "a/x.go", "b/y.go"),
		v("r", "a/z.go", "b/y.go"), // fixed in new
	}, 0)
	newR := NewReport(3, []Violation{
		v("r", "a/x.go", "b/y.go"), // unchanged, not interesting
		v("r", "a/w.go", "b/y.go"), // new
	}, 0)

	entries := Diff(old, newR)
	if len(entries) != 2 {
		t.Fatalf("got %d entr(ies), want 2: %+v", len(entries), entries)
	}

	var gotNew, gotFixed bool
	for _, e := range entries {
		switch e.Change {
		case "new":
			gotNew = true
			if e.Violation.File != "a/w.go" {
				t.Errorf("new entry = %+v, want a/w.go", e)
			}
		case "fixed":
			gotFixed = true
			if e.Violation.File != "a/z.go" {
				t.Errorf("fixed entry = %+v, want a/z.go", e)
			}
		default:
			t.Errorf("unexpected Change %q", e.Change)
		}
	}
	if !gotNew || !gotFixed {
		t.Errorf("expected both a \"new\" and a \"fixed\" entry, got %+v", entries)
	}
}

func TestDiffNoChanges(t *testing.T) {
	r := NewReport(3, []Violation{v("r", "a/x.go", "b/y.go")}, 0)
	if entries := Diff(r, r); len(entries) != 0 {
		t.Errorf("diffing a report against itself should yield no entries, got %+v", entries)
	}
}

func TestDiffOrdersNewBeforeFixed(t *testing.T) {
	old := NewReport(1, []Violation{v("r", "a/gone.go", "b/y.go")}, 0)
	newR := NewReport(1, []Violation{v("r", "a/added.go", "b/y.go")}, 0)

	entries := Diff(old, newR)
	if len(entries) != 2 {
		t.Fatalf("got %d entr(ies), want 2", len(entries))
	}
	if entries[0].Change != "new" || entries[1].Change != "fixed" {
		t.Errorf("entries = %+v, want [new, fixed]", entries)
	}
}

func TestWriteDiffTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	WriteDiffTable(&buf, nil, 20)
	if !strings.Contains(buf.String(), "no violation changes") {
		t.Errorf("expected a no-changes message, got:\n%s", buf.String())
	}
}

func TestWriteDiffTableAndTruncation(t *testing.T) {
	var entries []DiffEntry
	for i := 0; i < 5; i++ {
		entries = append(entries, DiffEntry{Violation: v("r", "a/x.go", "b/y.go"), Change: "new"})
	}

	var buf bytes.Buffer
	WriteDiffTable(&buf, entries, 2)
	out := buf.String()
	if !strings.Contains(out, "new") || !strings.Contains(out, "a/x.go") {
		t.Errorf("expected a row for the change, got:\n%s", out)
	}
	if !strings.Contains(out, "3 more change(s) not shown") {
		t.Errorf("expected truncation note, got:\n%s", out)
	}
}

func TestWriteDiffJSON(t *testing.T) {
	entries := []DiffEntry{{Violation: v("r", "a/x.go", "b/y.go"), Change: "new"}}

	var buf bytes.Buffer
	if err := WriteDiffJSON(&buf, entries); err != nil {
		t.Fatalf("WriteDiffJSON: %v", err)
	}

	var decoded []DiffEntry
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(decoded) != 1 || decoded[0].Change != "new" || decoded[0].Violation.File != "a/x.go" {
		t.Errorf("decoded = %+v", decoded)
	}
}
