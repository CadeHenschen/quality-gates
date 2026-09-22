package golang

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/analyzers"
)

func parseFunc(t *testing.T, src string) *ast.FuncDecl {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", "package p\n"+src, 0)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			return fn
		}
	}
	t.Fatalf("no func decl found in %q", src)
	return nil
}

func TestParamCount(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"none", "func f() {}", 0},
		{"one", "func f(a int) {}", 1},
		{"multi-name field", "func f(a, b int, c string) {}", 3},
		{"receiver not counted", "func (r *T) m(a int) {}", 1},
		{"variadic counts as one", "func f(a ...int) {}", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := paramCount(parseFunc(t, c.src)); got != c.want {
				t.Errorf("paramCount(%q) = %d, want %d", c.src, got, c.want)
			}
		})
	}
}

func TestMaxNestingDepth(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"flat", `func f() { x := 1; _ = x }`, 0},
		{"single if", `func f() { if true { x := 1; _ = x } }`, 1},
		{"nested if", `func f() { if true { if true { x := 1; _ = x } } }`, 2},
		{
			"else-if chain doesn't add depth",
			`func f(a, b, c bool) {
				if a {
					x := 1
					_ = x
				} else if b {
					y := 1
					_ = y
				} else if c {
					z := 1
					_ = z
				} else {
					w := 1
					_ = w
				}
			}`,
			1,
		},
		{
			"nesting inside one else-if branch still counts",
			`func f(a, b bool) {
				if a {
				} else if b {
					if a {
						x := 1
						_ = x
					}
				}
			}`,
			2,
		},
		{"for containing if", `func f(xs []int) { for range xs { if true { x := 1; _ = x } } }`, 2},
		{
			"switch case body nests",
			`func f(x int) {
				switch x {
				case 1:
					if true {
						y := 1
						_ = y
					}
				}
			}`,
			2,
		},
		{
			"select comm clause nests",
			`func f(ch chan int) {
				select {
				case <-ch:
					if true {
						y := 1
						_ = y
					}
				}
			}`,
			2,
		},
		{"bare block", `func f() { { x := 1; _ = x } }`, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := maxNestingDepth(parseFunc(t, c.src)); got != c.want {
				t.Errorf("maxNestingDepth(%s) = %d, want %d", c.name, got, c.want)
			}
		})
	}
}

// TestAnalyzeStampsSizeFields checks ParamCount/MaxNestingDepth/FileLines
// end-to-end through Analyze against the real fixture, rather than just
// the unit-level helpers above.
func TestAnalyzeStampsSizeFields(t *testing.T) {
	fns, err := Analyzer{}.Analyze(analyzers.Options{Dir: "../../../testdata/crap/golang"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	byName := map[string]struct {
		params, nesting int
	}{}
	var fileLines int
	for _, f := range fns {
		byName[f.Name] = struct{ params, nesting int }{f.ParamCount, f.MaxNestingDepth}
		fileLines = f.FileLines
	}

	want := map[string]struct{ params, nesting int }{
		"Simple":  {0, 0},
		"Branchy": {1, 1},
		"Loopy":   {1, 2}, // range -> if
	}
	for name, w := range want {
		got, ok := byName[name]
		if !ok {
			t.Errorf("function %q not found (got: %v)", name, byName)
			continue
		}
		if got != w {
			t.Errorf("%s = %+v, want %+v", name, got, w)
		}
	}

	if fileLines <= 0 {
		t.Errorf("FileLines = %d, want a positive physical line count", fileLines)
	}
}
