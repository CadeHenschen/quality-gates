package swift

import (
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/analyzers"
)

func TestAnalyzeStampsSizeFields(t *testing.T) {
	fns, err := Analyzer{}.Analyze(analyzers.Options{Dir: "../../../testdata/crap/swift-size"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	type want struct{ params, nesting int }
	cases := map[string]want{
		"Sample.simple":           {0, 0},
		"Sample.manyParams":       {4, 0},
		"Sample.elifChain":        {3, 1}, // if/else-if/else-if/else flattens to one level
		"Sample.nested":           {1, 2}, // for -> if
		"Sample.guarded":          {1, 1}, // guard's else block is a real nested block
		"Sample.withNestedHelper": {1, 0}, // nested local func's own ifs are opaque to the outer function
	}

	got := map[string]want{}
	var fileLines int
	for _, f := range fns {
		got[f.Name] = want{f.ParamCount, f.MaxNestingDepth}
		fileLines = f.FileLines
	}

	for name, w := range cases {
		g, ok := got[name]
		if !ok {
			t.Errorf("function %q not found (got: %v)", name, got)
			continue
		}
		if g != w {
			t.Errorf("%s = %+v, want %+v", name, g, w)
		}
	}
	if len(got) != len(cases) {
		t.Errorf("got %d functions, want %d (got: %v) — a nested local func must not appear on its own", len(got), len(cases), got)
	}

	if fileLines != 52 {
		t.Errorf("FileLines = %d, want 52", fileLines)
	}
}
