package swift

import "git.roost-r.com/cadeh/quality-gates/internal/swiftlex"

type rawFunc struct {
	name               string
	startLine, endLine int
	complexity         int
}

var typeKeywords = map[string]bool{
	"class": true, "struct": true, "enum": true,
	"protocol": true, "extension": true, "actor": true,
}

var funcKeywords = map[string]bool{
	"func": true, "init": true, "deinit": true, "subscript": true,
}

// complexityKeywords are the lexically-unambiguous decision points counted
// toward cyclomatic complexity. Deliberately excludes the ternary '?:' —
// see the swiftlex package doc comment for why that can't be told apart
// from optional chaining/optional-type '?' without real parsing. "else"
// isn't counted either, matching the Go analyzer's convention: "else if"
// is already counted via its own "if".
var complexityKeywords = map[string]bool{
	"if": true, "guard": true, "while": true, "for": true,
	"case": true, "catch": true, "&&": true, "||": true, "??": true,
}

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

type typeFrame struct {
	depth int
	name  string
}

// extractFunctions walks a token stream once, tracking overall
// curly-brace depth and a stack of enclosing type scopes, to find every
// func/init/deinit/subscript body at depth-appropriate positions and score
// its cyclomatic complexity. See the package doc comment for the nested-
// local-func and computed-property scope decisions.
func extractFunctions(toks []swiftlex.Token) []rawFunc {
	var out []rawFunc
	var typeStack []typeFrame
	depth := 0

	for i := 0; i < len(toks); {
		t := toks[i]
		switch {
		case t.Kind == swiftlex.Keyword && typeKeywords[t.Text]:
			name := ""
			if i+1 < len(toks) && (toks[i+1].Kind == swiftlex.Ident || toks[i+1].Kind == swiftlex.Keyword) {
				name = toks[i+1].Text
			}
			openIdx := findFirstBrace(toks, i+1)
			if openIdx == -1 {
				i++
				continue
			}
			typeStack = append(typeStack, typeFrame{depth: depth, name: name})
			depth++
			i = openIdx + 1

		case t.Kind == swiftlex.Keyword && funcKeywords[t.Text]:
			bodyOpen := findFuncBodyOpenBrace(toks, i)
			if bodyOpen == -1 {
				i++
				continue
			}
			name := funcQualifiedName(toks, i, typeStack)
			endIdx, complexity := scanFunctionBody(toks, bodyOpen)
			out = append(out, rawFunc{
				name:       name,
				startLine:  t.Line,
				endLine:    toks[endIdx].Line,
				complexity: complexity,
			})
			i = endIdx + 1

		case t.Text == "{":
			depth++
			i++

		case t.Text == "}":
			depth--
			for len(typeStack) > 0 && typeStack[len(typeStack)-1].depth == depth {
				typeStack = typeStack[:len(typeStack)-1]
			}
			i++

		default:
			i++
		}
	}
	return out
}

// findFirstBrace returns the index of the first "{" token at or after
// from. Type declaration headers (generic params, inheritance clause,
// where clauses) never contain a '{' of their own before the body, so no
// depth tracking is needed here (unlike function signatures, which can
// contain one via a default-parameter closure).
func findFirstBrace(toks []swiftlex.Token, from int) int {
	for i := from; i < len(toks); i++ {
		if toks[i].Text == "{" {
			return i
		}
	}
	return -1
}

// findFuncBodyOpenBrace scans forward from a func/init/deinit/subscript
// keyword at funcIdx to find its body's opening '{', tracking a local
// depth over "([" / ")]" so a '{' inside the parameter list (a default
// closure argument) isn't mistaken for the body. Returns -1 if the
// declaration has no body (a protocol requirement).
func findFuncBodyOpenBrace(toks []swiftlex.Token, funcIdx int) int {
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
			if depth == 0 && t.Kind == swiftlex.Keyword && declStopKeywords[t.Text] {
				return -1
			}
		}
	}
	return -1
}

// scanFunctionBody consumes tokens from just after a function's opening
// '{' until its matching '}' (tracking only curly-brace depth — nested
// func/init/etc. declarations are deliberately not treated specially here;
// see the package doc comment), counting complexity keywords/operators
// anywhere in the span. Returns the index of the matching '}' and the
// complexity score (starting at 1).
func scanFunctionBody(toks []swiftlex.Token, bodyOpen int) (endIdx, complexity int) {
	complexity = 1
	depth := 1
	i := bodyOpen + 1
	for ; i < len(toks) && depth > 0; i++ {
		switch t := toks[i]; t.Text {
		case "{":
			depth++
		case "}":
			depth--
		default:
			if complexityKeywords[t.Text] {
				complexity++
			}
		}
	}
	return i - 1, complexity
}

func funcQualifiedName(toks []swiftlex.Token, funcIdx int, typeStack []typeFrame) string {
	base := toks[funcIdx].Text // "init" / "deinit" / "subscript"
	if toks[funcIdx].Text == "func" && funcIdx+1 < len(toks) {
		base = toks[funcIdx+1].Text
	}
	if len(typeStack) == 0 {
		return base
	}
	return typeStack[len(typeStack)-1].name + "." + base
}
