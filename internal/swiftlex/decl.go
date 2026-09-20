package swiftlex

// declStopKeywords signal, when hit at bracket-depth 0 while scanning for a
// function's opening '{', that no body will be found — e.g. a protocol
// method requirement ("func bar() -> Int" with no body, followed directly
// by the next requirement). This is a heuristic, not a real parser: a
// pathological signature could in principle confuse it into attributing a
// later declaration's body to this one. Not expected to matter in
// practice — protocol requirement signatures are simple — and documented
// here rather than solved with real parsing, consistent with this
// package's other v1 scope choices.
var declStopKeywords = map[string]bool{
	"func": true, "var": true, "let": true, "case": true, "init": true,
	"deinit": true, "subscript": true, "typealias": true,
	"associatedtype": true, "class": true, "struct": true, "enum": true,
	"protocol": true, "extension": true, "static": true, "override": true,
	"mutating": true, "private": true, "public": true, "internal": true,
	"fileprivate": true, "open": true, "final": true,
}

// FindFuncBodyOpen scans forward from a func/init/deinit/subscript
// keyword at funcIdx to find its body's opening '{', tracking a local
// depth over "([" / ")]" so a '{' inside the parameter list (a default
// closure argument) isn't mistaken for the body. Returns -1 if the
// declaration has no body (a protocol requirement).
func FindFuncBodyOpen(toks []Token, funcIdx int) int {
	depth := 0
	for i := funcIdx + 1; i < len(toks); i++ {
		t := toks[i]
		switch t.Text {
		case "(", "[":
			depth++
		case ")", "]":
			if depth > 0 {
				depth--
			}
		case "{":
			if depth == 0 {
				return i
			}
			depth++
		case "}":
			if depth == 0 {
				return -1
			}
			depth--
		case ";":
			if depth == 0 {
				return -1
			}
		default:
			if depth == 0 && t.Kind == Keyword && declStopKeywords[t.Text] {
				return -1
			}
		}
	}
	return -1
}

// MatchBrace returns the index of the "}" matching the "{" at open, or
// len(toks)-1 if the file ends first.
func MatchBrace(toks []Token, open int) int {
	depth := 0
	for i := open; i < len(toks); i++ {
		switch toks[i].Text {
		case "{":
			depth++
		case "}":
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(toks) - 1
}
