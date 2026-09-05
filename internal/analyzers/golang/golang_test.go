package golang

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/analyzers"
	"git.roost-r.com/cadeh/quality-gates/internal/crap"
)

func TestAnalyzeComplexity(t *testing.T) {
	fns, err := Analyzer{}.Analyze(analyzers.Options{Dir: "../../../testdata/crap/golang"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	want := map[string]int{
		"Simple":  1,
		"Branchy": 2, // +1 for the if
		"Loopy":   4, // +1 range, +1 if, +1 &&
	}

	got := map[string]int{}
	for _, f := range fns {
		got[f.Name] = f.Complexity
	}

	for name, wantComplexity := range want {
		gotComplexity, ok := got[name]
		if !ok {
			t.Errorf("function %q not found in analyzer output (got: %v)", name, got)
			continue
		}
		if gotComplexity != wantComplexity {
			t.Errorf("%s complexity = %d, want %d", name, gotComplexity, wantComplexity)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d functions, want %d (got: %v)", len(got), len(want), got)
	}
}

// TestAnalyzeSkipsTestdataDir walks the whole crap-metric repo (its own
// go.mod root) and checks no testdata/ fixture leaked in as if it were
// real source — this also doubles as a smoke test of the analyzer against
// crap-metric's actual codebase.
func TestAnalyzeSkipsTestdataDir(t *testing.T) {
	fns, err := Analyzer{}.Analyze(analyzers.Options{Dir: "../../.."})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(fns) == 0 {
		t.Fatal("got 0 functions analyzing crap-metric's own repo — something's wrong")
	}
	for _, f := range fns {
		if strings.Contains(f.File, "testdata") {
			t.Errorf("testdata file leaked into analysis: %+v", f)
		}
	}
}

func TestParseCoverProfileAndRangeOverlap(t *testing.T) {
	profile := "mode: set\n" +
		"example.com/pkg/file.go:10.1,12.2 3 1\n" + // covered, inside [8,15]
		"example.com/pkg/file.go:20.1,22.2 2 0\n" + // uncovered, inside [18,25]
		"example.com/pkg/file.go:30.1,31.2 1 1\n" + // covered, outside any range we query
		"example.com/other/file.go:10.1,12.2 5 1\n" // different file entirely

	dir := t.TempDir()
	path := filepath.Join(dir, "cover.out")
	if err := os.WriteFile(path, []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}

	blocks, err := parseCoverProfile(path)
	if err != nil {
		t.Fatalf("parseCoverProfile: %v", err)
	}
	if len(blocks) != 4 {
		t.Fatalf("got %d blocks, want 4", len(blocks))
	}

	total, covered, uncovered := coverageForRange(blocks, "example.com/pkg/file.go", 8, 15)
	if total != 3 || covered != 3 {
		t.Errorf("range [8,15]: total=%d covered=%d, want total=3 covered=3", total, covered)
	}
	if len(uncovered) != 0 {
		t.Errorf("range [8,15]: uncovered=%v, want none (fully covered)", uncovered)
	}

	total, covered, uncovered = coverageForRange(blocks, "example.com/pkg/file.go", 18, 25)
	if total != 2 || covered != 0 {
		t.Errorf("range [18,25]: total=%d covered=%d, want total=2 covered=0", total, covered)
	}
	wantUncovered := []crap.LineRange{{Start: 20, End: 22}}
	if !reflect.DeepEqual(uncovered, wantUncovered) {
		t.Errorf("range [18,25]: uncovered=%v, want %v", uncovered, wantUncovered)
	}

	total, covered, uncovered = coverageForRange(blocks, "example.com/pkg/file.go", 1, 5)
	if total != 0 || covered != 0 {
		t.Errorf("range [1,5]: total=%d covered=%d, want total=0 covered=0 (no overlap)", total, covered)
	}
	if len(uncovered) != 0 {
		t.Errorf("range [1,5]: uncovered=%v, want none (no overlap)", uncovered)
	}
}
