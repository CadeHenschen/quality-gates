package swiftlex

import "testing"

func texts(toks []Token) []string {
	out := make([]string, len(toks))
	for i, t := range toks {
		out[i] = t.Text
	}
	return out
}

func TestSimpleFunc(t *testing.T) {
	toks := Tokenize([]byte(`func add(a: Int, b: Int) -> Int {
	return a + b
}`))
	want := []string{"func", "add", "(", "a", ":", "Int", ",", "b", ":", "Int", ")", "->", "Int", "{", "return", "a", "+", "b", "}"}
	got := texts(toks)
	if len(got) != len(want) {
		t.Fatalf("got %d tokens %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token %d = %q, want %q", i, got[i], want[i])
		}
	}
	if toks[0].Kind != Keyword {
		t.Errorf("\"func\" kind = %v, want Keyword", toks[0].Kind)
	}
	if toks[1].Kind != Ident {
		t.Errorf("\"add\" kind = %v, want Ident", toks[1].Kind)
	}
}

func TestStringLiteralWholeToken(t *testing.T) {
	toks := Tokenize([]byte(`let x = "hello"`))
	if len(toks) != 4 {
		t.Fatalf("got %d tokens %v, want 4", len(toks), texts(toks))
	}
	if toks[3].Text != `"hello"` || toks[3].Kind != String {
		t.Errorf("string token = %+v, want Text=%q Kind=String", toks[3], `"hello"`)
	}
}

// This is the design review's specific stress case: a nested string
// literal inside interpolation, containing characters ('?' ':') that could
// be mistaken for something else, must not confuse the outer string's own
// terminator search.
func TestNestedInterpolationWithNestedString(t *testing.T) {
	src := `let s = "prefix \(x == "a" ? 1 : 2) suffix"
let next = 1`
	toks := Tokenize([]byte(src))
	want := []string{"let", "s", "=", `"prefix \(x == "a" ? 1 : 2) suffix"`, "let", "next", "=", "1"}
	got := texts(toks)
	if len(got) != len(want) {
		t.Fatalf("got %d tokens %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token %d = %q, want %q", i, got[i], want[i])
		}
	}
	if toks[7].Line != 2 {
		t.Errorf("line after multi-interpolation string = %d, want 2", toks[7].Line)
	}
}

func TestDeeplyNestedInterpolation(t *testing.T) {
	toks := Tokenize([]byte(`"\( "\(a)" )"`))
	if len(toks) != 1 || toks[0].Kind != String {
		t.Fatalf("got %v, want a single String token", toks)
	}
}

func TestNestedBlockComments(t *testing.T) {
	toks := Tokenize([]byte(`/* outer /* inner */ still comment */ let x = 1`))
	want := []string{`/* outer /* inner */ still comment */`, "let", "x", "=", "1"}
	got := texts(toks)
	if len(got) != len(want) {
		t.Fatalf("got %d tokens %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRawStringNoInterpolation(t *testing.T) {
	// Inside a #"..."# raw string, a bare \( is literal text, not
	// interpolation — the terminator search must not be confused by it.
	toks := Tokenize([]byte(`let s = #"literal \(not interpolated) text"# ; let y = 2`))
	got := texts(toks)
	if got[3] != `#"literal \(not interpolated) text"#` {
		t.Errorf("raw string token = %q", got[3])
	}
	if got[len(got)-1] != "2" {
		t.Errorf("tokens after raw string = %v, lexer likely desynced", got)
	}
}

func TestRawStringWithHashInterpolation(t *testing.T) {
	toks := Tokenize([]byte(`let s = #"value: \#(x + 1)"# ; let y = 2`))
	got := texts(toks)
	if got[3] != `#"value: \#(x + 1)"#` {
		t.Errorf("raw string token = %q", got[3])
	}
	if got[len(got)-1] != "2" {
		t.Errorf("tokens after raw string = %v, lexer likely desynced", got)
	}
}

func TestMultilineString(t *testing.T) {
	src := "let s = \"\"\"\nline one\nline two\n\"\"\"\nlet y = 2"
	toks := Tokenize([]byte(src))
	got := texts(toks)
	if got[3] != "\"\"\"\nline one\nline two\n\"\"\"" {
		t.Errorf("multiline string token = %q", got[3])
	}
	if got[len(got)-1] != "2" {
		t.Errorf("tokens after multiline string = %v, lexer likely desynced", got)
	}
	// "let y = 2" starts on line 5.
	if toks[len(toks)-4].Line != 5 {
		t.Errorf("line after multiline string = %d, want 5", toks[len(toks)-4].Line)
	}
}

func TestRegexSlashTreatedAsOperator(t *testing.T) {
	toks := Tokenize([]byte(`let x = a / b`))
	want := []string{"let", "x", "=", "a", "/", "b"}
	got := texts(toks)
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token %d = %q, want %q", i, got[i], want[i])
		}
	}
	if toks[4].Kind != Operator {
		t.Errorf("'/' kind = %v, want Operator", toks[4].Kind)
	}
}

func TestBacktickIdentifierNotKeyword(t *testing.T) {
	toks := Tokenize([]byte("let `class` = 1"))
	got := texts(toks)
	want := []string{"let", "`class`", "=", "1"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token %d = %q, want %q", i, got[i], want[i])
		}
	}
	if toks[1].Kind != Ident {
		t.Errorf("`class` kind = %v, want Ident (not Keyword)", toks[1].Kind)
	}
}

func TestRangeOperatorNotSwallowedByNumber(t *testing.T) {
	toks := Tokenize([]byte(`for i in 1...10 {}`))
	got := texts(toks)
	want := []string{"for", "i", "in", "1", "...", "10", "{", "}"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDecimalNumberAndExponent(t *testing.T) {
	toks := Tokenize([]byte(`let a = 1.5 ; let b = 1e+5 ; let c = 0xFF`))
	got := texts(toks)
	want := []string{"let", "a", "=", "1.5", ";", "let", "b", "=", "1e+5", ";", "let", "c", "=", "0xFF"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLineCommentAndKindClassification(t *testing.T) {
	toks := Tokenize([]byte("// a comment\nlet x = 1"))
	if toks[0].Kind != Comment || toks[0].Text != "// a comment" {
		t.Errorf("comment token = %+v", toks[0])
	}
	if toks[1].Line != 2 {
		t.Errorf("line after line comment = %d, want 2", toks[1].Line)
	}
}

func TestUnterminatedStringDoesNotHang(t *testing.T) {
	// No assertion beyond "returns": an unterminated string/comment/
	// interpolation must not infinite-loop. `go test`'s own timeout is the
	// real backstop if this regresses.
	toks := Tokenize([]byte(`let s = "unterminated`))
	if len(toks) < 3 {
		t.Errorf("got %v, want at least let/s/= before the dangling string", texts(toks))
	}
}

func FuzzTokenize(f *testing.F) {
	f.Add([]byte(`func f() { return "ok" }`))
	f.Add([]byte(`/* nested /* comment */`))
	f.Add([]byte("\xff\x00\n"))
	f.Fuzz(func(t *testing.T, source []byte) {
		tokens := Tokenize(source)
		lastLine := 1
		for _, token := range tokens {
			if token.Line < lastLine || token.Line < 1 {
				t.Errorf("invalid token line sequence %d after %d: %+v", token.Line, lastLine, token)
			}
			lastLine = token.Line
		}
	})
}
