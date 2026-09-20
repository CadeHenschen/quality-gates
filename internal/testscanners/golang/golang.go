// Package golang scans Go test files with go/parser — real AST, no
// regexes — for the per-test facts internal/testmetric grades.
package golang

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"git.roost-r.com/cadeh/quality-gates/internal/testmetric"
)

type Scanner struct{}

// helperName matches the assertion-helper naming convention: a call like
// `assertEqual(t, ...)` or `verifyOutput(t, ...)` that receives the test's
// *testing.T is treated as an assertion, since the real check lives in the
// helper's body. This is the fallback for helpers defined outside the
// package under scan; in-package helpers are resolved by body instead (see
// assertingHelpers). Passing t to an arbitrary function isn't enough —
// that would count `newFixture(t)` and hide a genuinely assertion-less
// test.
var helperName = regexp.MustCompile(`(?i)^(assert|check|verify|expect|require|must|ensure|validate|compare|equal|want|golden|diff)`)

var failMethods = map[string]bool{"Error": true, "Errorf": true, "Fatal": true, "Fatalf": true, "Fail": true, "FailNow": true}

var skipMethods = map[string]bool{"Skip": true, "Skipf": true, "SkipNow": true}

// testifyMockAsserts verify a testify mock's recorded calls — assertions,
// but about interaction rather than behavior.
var testifyMockAsserts = map[string]bool{
	"AssertExpectations": true, "AssertCalled": true,
	"AssertNotCalled": true, "AssertNumberOfCalls": true,
}

// parsedFile is one parsed *_test.go file.
type parsedFile struct {
	path string
	fset *token.FileSet
	file *ast.File
}

func (Scanner) Scan(dir string) ([]testmetric.Test, error) {
	byDir := map[string][]parsedFile{}
	var order []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == "node_modules" || name == "testdata" ||
				(strings.HasPrefix(name, ".") && path != dir) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		d0 := filepath.Dir(path)
		if _, seen := byDir[d0]; !seen {
			order = append(order, d0)
		}
		byDir[d0] = append(byDir[d0], parsedFile{path: path, fset: fset, file: file})
		return nil
	})
	if err != nil {
		return nil, err
	}

	var out []testmetric.Test
	for _, d0 := range order {
		files := byDir[d0]
		helpers := assertingHelpers(files)
		for _, pf := range files {
			out = append(out, testsIn(pf, dir, helpers)...)
		}
	}
	return out, nil
}

// assertingHelpers finds the package's own helper functions — non-test
// functions taking a *testing.T — that fail it directly or by calling
// another such helper. A test calling one (`findFunc(t, ...)`) is asserting
// through it even when its name doesn't sound like an assertion, so this
// resolves helpers by what their bodies do rather than only by naming
// convention. Iterated to a fixpoint so a helper of a helper counts.
func assertingHelpers(files []parsedFile) map[string]bool {
	helpers := map[string]bool{}
	for changed := true; changed; {
		changed = false
		for _, pf := range files {
			for _, decl := range pf.file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil || helpers[fn.Name.Name] || isTestFunc(fn) {
					continue
				}
				if len(testingTParams(fn.Type)) == 0 {
					continue
				}
				if analyzeFunc(fn, helpers).Assertions > 0 {
					helpers[fn.Name.Name] = true
					changed = true
				}
			}
		}
	}
	return helpers
}

func testsIn(pf parsedFile, dir string, helpers map[string]bool) []testmetric.Test {
	rel, err := filepath.Rel(dir, pf.path)
	if err != nil {
		rel = pf.path
	}
	var out []testmetric.Test
	for _, decl := range pf.file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || !isTestFunc(fn) {
			continue
		}
		t := analyzeFunc(fn, helpers)
		t.File = rel
		t.Line = pf.fset.Position(fn.Pos()).Line
		t.Name = fn.Name.Name
		out = append(out, t)
	}
	return out
}

// isTestFunc reports whether fn is a `func TestXxx(t *testing.T)`.
func isTestFunc(fn *ast.FuncDecl) bool {
	if fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") {
		return false
	}
	rest := strings.TrimPrefix(fn.Name.Name, "Test")
	if r, _ := utf8.DecodeRuneInString(rest); rest != "" && unicode.IsLower(r) {
		return false
	}
	// go test only runs a func with exactly one parameter, a *testing.T.
	return fn.Type.Params.NumFields() == 1 && len(testingTParams(fn.Type)) == 1
}

// testingTParams returns the names of ft's *testing.T parameters.
func testingTParams(ft *ast.FuncType) []string {
	var names []string
	for _, field := range ft.Params.List {
		star, ok := field.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		sel, ok := star.X.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "T" {
			continue
		}
		if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "testing" {
			continue
		}
		for _, n := range field.Names {
			names = append(names, n.Name)
		}
	}
	return names
}

// state carries the walk's running facts.
type state struct {
	test        testmetric.Test
	helpers     map[string]bool
	tVars       map[string]bool
	createdTemp bool
	cleanedUp   bool
}

func analyzeFunc(fn *ast.FuncDecl, helpers map[string]bool) testmetric.Test {
	s := &state{tVars: map[string]bool{}, helpers: helpers}
	for _, n := range testingTParams(fn.Type) {
		s.tVars[n] = true
	}
	s.walk(fn.Body, false)
	s.test.UncleanedTemp = s.createdTemp && !s.cleanedUp
	return s.test
}

// walk visits n; conditional is true beneath any if/switch/select, where a
// t.Skip is an environment guard (tool missing, -short) rather than a
// disabled test.
func (s *state) walk(n ast.Node, conditional bool) {
	ast.Inspect(n, func(node ast.Node) bool {
		switch x := node.(type) {
		case *ast.IfStmt:
			s.walkIf(x, conditional)
			return false
		case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			s.walkChildren(node, true)
			return false
		case *ast.FuncLit:
			for _, name := range testingTParams(x.Type) {
				s.tVars[name] = true
			}
		case *ast.DeferStmt:
			s.cleanedUp = true
		case *ast.CallExpr:
			s.call(x, conditional)
		}
		return true
	})
}

func (s *state) walkIf(x *ast.IfStmt, conditional bool) {
	if x.Init != nil {
		s.walk(x.Init, conditional)
	}
	s.walk(x.Cond, conditional)
	s.walk(x.Body, true)
	if x.Else != nil {
		s.walk(x.Else, true)
	}
}

func (s *state) walkChildren(n ast.Node, conditional bool) {
	ast.Inspect(n, func(child ast.Node) bool {
		// Inspect also calls back with nil after a node's children, and a
		// switch's absent Init/Tag are nil interfaces — neither is a node
		// to walk.
		if child == nil || child == n {
			return child != nil
		}
		s.walk(child, conditional)
		return false
	})
}

func (s *state) call(c *ast.CallExpr, conditional bool) {
	recv, method := selectorParts(c.Fun)

	switch {
	case s.tVars[recv] && failMethods[method]:
		s.test.Assertions++
	case s.tVars[recv] && skipMethods[method]:
		if !conditional {
			s.test.Skipped = true
		}
	case s.tVars[recv] && method == "Cleanup":
		s.cleanedUp = true
	case recv == "assert" || recv == "require":
		s.test.Assertions++
	case testifyMockAsserts[method]:
		s.test.Assertions++
		s.test.Interactions++
	case isTempCreate(recv, method):
		s.createdTemp = true
	case recv == "os" && (method == "RemoveAll" || method == "Remove"):
		s.cleanedUp = true
	default:
		name := method
		if name == "" {
			name = identName(c.Fun)
		}
		if (helperName.MatchString(name) || s.helpers[name]) && s.passesT(c) {
			s.test.Assertions++
		}
	}
}

func (s *state) passesT(c *ast.CallExpr) bool {
	for _, arg := range c.Args {
		if id, ok := arg.(*ast.Ident); ok && s.tVars[id.Name] {
			return true
		}
	}
	return false
}

func isTempCreate(recv, method string) bool {
	return (recv == "os" && (method == "MkdirTemp" || method == "CreateTemp")) ||
		(recv == "ioutil" && (method == "TempDir" || method == "TempFile"))
}

// selectorParts splits `x.Sel(...)` into ("x", "Sel"); anything else
// yields empty parts.
func selectorParts(fun ast.Expr) (recv, method string) {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return "", ""
	}
	if id, ok := sel.X.(*ast.Ident); ok {
		recv = id.Name
	}
	return recv, sel.Sel.Name
}

func identName(fun ast.Expr) string {
	if id, ok := fun.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}
