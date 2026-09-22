package typescript

import (
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/analyzers"
)

func TestAnalyzeStampsSizeFields(t *testing.T) {
	fns, err := Analyzer{}.Analyze(analyzers.Options{Dir: "../../../testdata/crap/typescript-size"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	type want struct{ params, nesting int }
	cases := map[string]want{
		"simple":     {0, 0},
		"manyParams": {4, 0},
		"elifChain":  {3, 1}, // if/else-if/else-if/else flattens to one level
		"nested":     {1, 2}, // for -> if
		"method":     {2, 0},
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

	if fileLines != 37 {
		t.Errorf("FileLines = %d, want 37", fileLines)
	}
}
