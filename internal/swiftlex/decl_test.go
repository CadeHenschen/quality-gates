package swiftlex

import "testing"

// funcTokenIndex returns the index of the first Keyword token matching name.
func funcTokenIndex(t *testing.T, toks []Token, name string) int {
	t.Helper()
	for i, tok := range toks {
		if tok.Kind == Keyword && tok.Text == name {
			return i
		}
	}
	t.Fatalf("no %q keyword token found", name)
	return -1
}

// A default-parameter closure argument containing its own braces must not
// be mistaken for the function's body: the '{'/'}' pair inside "(...)" has
// to nest under the paren-tracked depth rather than immediately returning.
func TestFindFuncBodyOpenSkipsNestedBracesInDefaultClosureArg(t *testing.T) {
	src := `func withClosure(cb: () -> Void = { print(1) }) {
    cb()
}`
	toks := Tokenize([]byte(src))
	funcIdx := funcTokenIndex(t, toks, "func")

	got := FindFuncBodyOpen(toks, funcIdx)
	if got == -1 || toks[got].Text != "{" {
		t.Fatalf("FindFuncBodyOpen = %d, want index of the body's opening brace", got)
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
func TestFindFuncBodyOpenNoBodyBeforeClosingBrace(t *testing.T) {
	src := `protocol P {
    func bar() -> Int
}`
	toks := Tokenize([]byte(src))
	funcIdx := funcTokenIndex(t, toks, "func")

	if got := FindFuncBodyOpen(toks, funcIdx); got != -1 {
		t.Errorf("FindFuncBodyOpen = %d, want -1 (no body, terminated by enclosing '}')", got)
	}
}

// A semicolon-terminated requirement (no body) must also report "no body".
func TestFindFuncBodyOpenNoBodyBeforeSemicolon(t *testing.T) {
	src := `protocol P {
    func bar() -> Int;
    func baz() -> Int
}`
	toks := Tokenize([]byte(src))
	funcIdx := funcTokenIndex(t, toks, "func")

	if got := FindFuncBodyOpen(toks, funcIdx); got != -1 {
		t.Errorf("FindFuncBodyOpen = %d, want -1 (no body, terminated by ';')", got)
	}
}

// Two consecutive requirements with no separator at all: hitting the next
// declaration's keyword at depth 0 must also signal "no body".
func TestFindFuncBodyOpenNoBodyBeforeNextDecl(t *testing.T) {
	src := `protocol P {
    func bar() -> Int
    func baz() -> Int
}`
	toks := Tokenize([]byte(src))
	funcIdx := funcTokenIndex(t, toks, "func")

	if got := FindFuncBodyOpen(toks, funcIdx); got != -1 {
		t.Errorf("FindFuncBodyOpen = %d, want -1 (no body, terminated by next decl keyword)", got)
	}
}

// A declaration with no body and nothing at all following it (end of the
// token stream) must also report "no body" rather than panicking or looping.
func TestFindFuncBodyOpenNoBodyAtEndOfTokens(t *testing.T) {
	src := `func bar() -> Int`
	toks := Tokenize([]byte(src))
	funcIdx := funcTokenIndex(t, toks, "func")

	if got := FindFuncBodyOpen(toks, funcIdx); got != -1 {
		t.Errorf("FindFuncBodyOpen = %d, want -1 (no body, ran out of tokens)", got)
	}
}

func TestMatchBrace(t *testing.T) {
	toks := Tokenize([]byte("func f() { if x { y() } }\nfunc g() {"))
	open := -1
	for i, tok := range toks {
		if tok.Text == "{" {
			open = i
			break
		}
	}
	end := MatchBrace(toks, open)
	if toks[end].Text != "}" || toks[end+1].Text != "func" {
		t.Errorf("MatchBrace returned index %d (%q), want the outer function's closing brace", end, toks[end].Text)
	}

	// An unterminated body runs to the end of the tokens rather than panicking.
	last := len(toks) - 1
	unterminated := -1
	for i := last; i >= 0; i-- {
		if toks[i].Text == "{" {
			unterminated = i
			break
		}
	}
	if got := MatchBrace(toks, unterminated); got != last {
		t.Errorf("MatchBrace on an unterminated body = %d, want %d", got, last)
	}
}
