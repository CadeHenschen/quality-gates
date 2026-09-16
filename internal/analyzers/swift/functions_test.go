package swift

import (
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/swiftlex"
)

// funcTokenIndex returns the index of the first Keyword token matching name.
func funcTokenIndex(t *testing.T, toks []swiftlex.Token, name string) int {
	t.Helper()
	for i, tok := range toks {
		if tok.Kind == swiftlex.Keyword && tok.Text == name {
			return i
		}
	}
	t.Fatalf("no %q keyword token found", name)
	return -1
}

// A default-parameter closure argument containing its own braces must not
// be mistaken for the function's body: the '{'/'}' pair inside "(...)" has
// to nest under the paren-tracked depth rather than immediately returning.
func TestFindFuncBodyOpenBraceSkipsNestedBracesInDefaultClosureArg(t *testing.T) {
	src := `func withClosure(cb: () -> Void = { print(1) }) {
    cb()
}`
	toks := swiftlex.Tokenize([]byte(src))
	funcIdx := funcTokenIndex(t, toks, "func")

	got := findFuncBodyOpenBrace(toks, funcIdx)
	if got == -1 || toks[got].Text != "{" {
		t.Fatalf("findFuncBodyOpenBrace = %d, want index of the body's opening brace", got)
	}
	// The body brace is the *second* "{" in the token stream — the first
	// belongs to the default closure argument.
	braceCount := 0
	for i := funcIdx; i <= got; i++ {
		if toks[i].Text == "{" {
			braceCount++
		}
	}
	if braceCount != 2 {
		t.Errorf("expected the returned brace to be the 2nd '{' after func, got the %d(st/nd)", braceCount)
	}
}

// A protocol requirement with no body, immediately followed by the
// protocol's own closing brace, must report "no body" rather than mistaking
// that brace for one.
func TestFindFuncBodyOpenBraceNoBodyBeforeClosingBrace(t *testing.T) {
	src := `protocol P {
    func bar() -> Int
}`
	toks := swiftlex.Tokenize([]byte(src))
	funcIdx := funcTokenIndex(t, toks, "func")

	if got := findFuncBodyOpenBrace(toks, funcIdx); got != -1 {
		t.Errorf("findFuncBodyOpenBrace = %d, want -1 (no body, terminated by enclosing '}')", got)
	}
}

// A semicolon-terminated requirement (no body) must also report "no body".
func TestFindFuncBodyOpenBraceNoBodyBeforeSemicolon(t *testing.T) {
	src := `protocol P {
    func bar() -> Int;
    func baz() -> Int
}`
	toks := swiftlex.Tokenize([]byte(src))
	funcIdx := funcTokenIndex(t, toks, "func")

	if got := findFuncBodyOpenBrace(toks, funcIdx); got != -1 {
		t.Errorf("findFuncBodyOpenBrace = %d, want -1 (no body, terminated by ';')", got)
	}
}

// Two consecutive requirements with no separator at all: hitting the next
// declaration's keyword at depth 0 must also signal "no body".
func TestFindFuncBodyOpenBraceNoBodyBeforeNextDecl(t *testing.T) {
	src := `protocol P {
    func bar() -> Int
    func baz() -> Int
}`
	toks := swiftlex.Tokenize([]byte(src))
	funcIdx := funcTokenIndex(t, toks, "func")

	if got := findFuncBodyOpenBrace(toks, funcIdx); got != -1 {
		t.Errorf("findFuncBodyOpenBrace = %d, want -1 (no body, terminated by next decl keyword)", got)
	}
}

// A declaration with no body and nothing at all following it (end of the
// token stream) must also report "no body" rather than panicking or looping.
func TestFindFuncBodyOpenBraceNoBodyAtEndOfTokens(t *testing.T) {
	src := `func bar() -> Int`
	toks := swiftlex.Tokenize([]byte(src))
	funcIdx := funcTokenIndex(t, toks, "func")

	if got := findFuncBodyOpenBrace(toks, funcIdx); got != -1 {
		t.Errorf("findFuncBodyOpenBrace = %d, want -1 (no body, ran out of tokens)", got)
	}
}
