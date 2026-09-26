// Package swift scans Swift test files (XCTest and Swift Testing) for the
// per-test facts internal/testmetric grades. Built on internal/swiftlex, the
// same native lexer crap-metric and dupe-metric use — no Swift toolchain
// needed — so it inherits that package's documented limits (regex literals
// lex as division; string-interpolation contents are opaque, so an
// assertion written *inside* an interpolation is invisible, which doesn't
// happen in practice).
//
// A test is a `func test…()` with no parameters in a file that imports
// XCTest, or any `func` carrying a `@Test` attribute (Swift Testing).
// Nested local funcs fold into their enclosing test, the same way
// internal/analyzers/swift treats them.
package swift

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"git.roost-r.com/cadeh/quality-gates/internal/swiftlex"
	"git.roost-r.com/cadeh/quality-gates/internal/testmetric"
)

type Scanner struct{}

// helperName is the fallback assertion-helper convention for helpers not
// defined under the scanned dir, as in the Go scanner; in-tree helpers are
// resolved by body instead.
var helperName = regexp.MustCompile(`(?i)^(assert|check|verify|expect|require|ensure|validate|compare|equal|want|snapshot)`)

// fileFuncs is one parsed file's functions plus its file-level facts.
type fileFuncs struct {
	path     string
	toks     []swiftlex.Token // comments removed
	funcs    []funcInfo
	xctest   bool // imports XCTest
	teardown bool // defines tearDown/deinit: cleans up on its tests' behalf
}

type funcInfo struct {
	name       string
	line       int
	attrs      []swiftlex.Token // tokens between the previous declaration/brace and `func`
	bodyOpen   int
	bodyEnd    int
	paramsNone bool
}

var skipDirs = map[string]bool{
	".build": true, "Pods": true, "Carthage": true, "DerivedData": true,
	"vendor": true, "node_modules": true, "testdata": true, "checkouts": true,
}

func (Scanner) Scan(dir string) (out []testmetric.Test, scanErr error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() {
		scanErr = errors.Join(scanErr, root.Close())
	}()

	var files []*fileFuncs
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] || (strings.HasPrefix(d.Name(), ".") && path != dir) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".swift") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("relative path for %s: %w", path, err)
		}
		f, err := root.Open(rel)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		src, readErr := io.ReadAll(f)
		closeErr := f.Close()
		if readErr != nil {
			return fmt.Errorf("%s: %w", path, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("%s: %w", path, closeErr)
		}
		files = append(files, parseFile(path, src))
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Helpers may live in any file under dir (a shared TestHelpers.swift),
	// so resolve them across all of them before grading any test.
	helpers := assertingHelpers(files)

	for _, f := range files {
		rel, err := filepath.Rel(dir, f.path)
		if err != nil {
			rel = f.path
		}
		for _, fn := range f.funcs {
			if !isTest(f, fn) {
				continue
			}
			t := analyze(f, fn, helpers)
			t.File = rel
			t.Line = fn.line
			t.Name = fn.name
			out = append(out, t)
		}
	}
	return out, nil
}

func parseFile(path string, src []byte) *fileFuncs {
	var toks []swiftlex.Token
	for _, t := range swiftlex.Tokenize(src) {
		if t.Kind != swiftlex.Comment {
			toks = append(toks, t)
		}
	}
	f := &fileFuncs{path: path, toks: toks}

	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch {
		case t.Text == "XCTest" && i > 0 && toks[i-1].Text == "import":
			f.xctest = true
		case t.Text == "deinit":
			f.teardown = true
		}
		if t.Kind != swiftlex.Keyword || t.Text != "func" || i+1 >= len(toks) {
			continue
		}
		// The func's name token is skipped along with its body below, so
		// note a tearDown override here rather than in the switch above.
		if toks[i+1].Text == "tearDown" || toks[i+1].Text == "tearDownWithError" {
			f.teardown = true
		}
		open := swiftlex.FindFuncBodyOpen(toks, i)
		if open == -1 {
			continue
		}
		end := swiftlex.MatchBrace(toks, open)
		f.funcs = append(f.funcs, funcInfo{
			name:       toks[i+1].Text,
			line:       t.Line,
			attrs:      attrRegion(toks, i),
			bodyOpen:   open,
			bodyEnd:    end,
			paramsNone: i+3 < len(toks) && toks[i+2].Text == "(" && toks[i+3].Text == ")",
		})
		// Skip the body: nested local funcs belong to this one.
		i = end
	}
	return f
}

// attrRegion returns the tokens between the previous declaration boundary
// and the func keyword at idx — where attributes like @Test(...) live.
func attrRegion(toks []swiftlex.Token, idx int) []swiftlex.Token {
	j := idx - 1
	for j >= 0 && toks[j].Text != "{" && toks[j].Text != "}" && toks[j].Text != ";" {
		j--
	}
	return toks[j+1 : idx]
}

// testAttr returns the @Test attribute's tokens (from `@` to the end of
// the region), or nil if fn has none.
func testAttr(fn funcInfo) []swiftlex.Token {
	for k := 0; k+1 < len(fn.attrs); k++ {
		if fn.attrs[k].Text == "@" && fn.attrs[k+1].Text == "Test" {
			return fn.attrs[k:]
		}
	}
	return nil
}

func isTest(f *fileFuncs, fn funcInfo) bool {
	if testAttr(fn) != nil {
		return true
	}
	return f.xctest && fn.paramsNone && strings.HasPrefix(fn.name, "test")
}

// facts are what a walk over one function body finds.
type facts struct {
	asserts                         int
	skipped, expectedFail           bool
	createdTemp, wroteDisk, cleaned bool
}

// guardStack tracks whether the walk is inside a conditional block: a `{`
// opened after if/guard/else/switch marks everything inside as an
// environment guard, so a `throw XCTSkip` there isn't a disabled test.
type guardStack struct {
	stack   []bool
	pending bool
}

func (g *guardStack) inGuard() bool { return len(g.stack) > 0 && g.stack[len(g.stack)-1] }

func (g *guardStack) observe(text string) {
	switch text {
	case "if", "guard", "else", "switch":
		g.pending = true
	case "{":
		g.stack = append(g.stack, g.inGuard() || g.pending)
		g.pending = false
	case "}":
		if len(g.stack) > 0 {
			g.stack = g.stack[:len(g.stack)-1]
		}
	}
}

// around returns the texts of the tokens either side of toks[i].
func around(toks []swiftlex.Token, i int) (prev, next string) {
	if i > 0 {
		prev = toks[i-1].Text
	}
	if i+1 < len(toks) {
		next = toks[i+1].Text
	}
	return prev, next
}

// isAssertion reports whether toks[i] is the name of an assertion call: an
// XCTest/Swift Testing primitive, an in-tree helper that asserts, or a
// call named like an assertion helper.
func isAssertion(toks []swiftlex.Token, i int, helpers map[string]bool) bool {
	t := toks[i]
	prev, next := around(toks, i)
	switch {
	case strings.HasPrefix(t.Text, "XCTAssert") && next == "(":
		return true
	case (t.Text == "XCTFail" || t.Text == "XCTUnwrap") && next == "(":
		return true
	case prev == "#" && (t.Text == "expect" || t.Text == "require"):
		return true
	case t.Text == "Issue" && next == "." && i+2 < len(toks) && toks[i+2].Text == "record":
		return true
	}
	if t.Kind != swiftlex.Ident || next != "(" || prev == "func" {
		return false
	}
	return helpers[t.Text] || (helperName.MatchString(t.Text) && t.Text != "expectation")
}

// noteLifecycle records the non-assertion facts toks[i] contributes: skips,
// expected failures, and temp-file creation/cleanup.
func (fc *facts) noteLifecycle(toks []swiftlex.Token, i int, inGuard bool) {
	text := toks[i].Text
	prev, next := around(toks, i)
	switch {
	case text == "XCTSkip" && next == "(" && prev == "throw" && !inGuard:
		fc.skipped = true
	case text == "XCTExpectFailure" && next == "(":
		fc.expectedFail = true
	case text == "withKnownIssue" && (next == "(" || next == "{"):
		fc.expectedFail = true
	case text == "temporaryDirectory" || text == "NSTemporaryDirectory" || text == "mkdtemp":
		fc.createdTemp = true
	case (text == "createDirectory" || text == "createFile") && next == "(":
		fc.wroteDisk = true
	case text == "removeItem" || text == "addTeardownBlock" || text == "addTeardown" || text == "defer":
		fc.cleaned = true
	}
}

// walkBody scans fn's body for the facts testmetric grades.
func walkBody(f *fileFuncs, fn funcInfo, helpers map[string]bool) facts {
	var fc facts
	var guards guardStack
	for i := fn.bodyOpen + 1; i < fn.bodyEnd; i++ {
		inGuard := guards.inGuard()
		guards.observe(f.toks[i].Text)
		if isAssertion(f.toks, i, helpers) {
			fc.asserts++
		}
		fc.noteLifecycle(f.toks, i, inGuard)
	}
	return fc
}

// assertingHelpers finds non-test functions, anywhere under the scanned dir,
// whose body asserts or calls another such helper — so a `loadFixture(...)`
// that itself XCTAsserts counts however it's named. Iterated to a fixpoint
// for helpers of helpers.
func assertingHelpers(files []*fileFuncs) map[string]bool {
	helpers := map[string]bool{}
	for changed := true; changed; {
		changed = false
		for _, f := range files {
			for _, fn := range f.funcs {
				if helpers[fn.name] || isTest(f, fn) {
					continue
				}
				if walkBody(f, fn, helpers).asserts > 0 {
					helpers[fn.name] = true
					changed = true
				}
			}
		}
	}
	return helpers
}

func analyze(f *fileFuncs, fn funcInfo, helpers map[string]bool) testmetric.Test {
	fc := walkBody(f, fn, helpers)
	t := testmetric.Test{
		Assertions:    fc.asserts,
		Skipped:       fc.skipped,
		ExpectedFail:  fc.expectedFail,
		UncleanedTemp: fc.createdTemp && fc.wroteDisk && !fc.cleaned && !f.teardown,
	}
	// @Test(.disabled(...)) is unconditional; .enabled(if:) is a guard.
	attr := testAttr(fn)
	for k := 1; k < len(attr); k++ {
		if attr[k].Text == "disabled" && attr[k-1].Text == "." {
			t.Skipped = true
		}
	}
	return t
}
