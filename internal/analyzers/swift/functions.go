package swift

import "git.roost-r.com/cadeh/quality-gates/internal/swiftlex"

type rawFunc struct {
	name               string
	startLine, endLine int
	complexity         int
	paramCount         int
	maxNestingDepth    int
}

var typeKeywords = map[string]bool{
	"class": true, "struct": true, "enum": true,
	"protocol": true, "extension": true, "actor": true,
}

var funcKeywords = map[string]bool{
	"func": true, "init": true, "deinit": true, "subscript": true,
}

// propertyAccessorKeywords mark a computed-property or property-observer
// sub-block: "get"/"set" for a property with no stored backing, "willSet"/
// "didSet" for a stored property's observers. See extractFunctions' "var"
// case and scanPropertyAccessors.
var propertyAccessorKeywords = map[string]bool{
	"get": true, "set": true, "willSet": true, "didSet": true,
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
			bodyOpen := swiftlex.FindFuncBodyOpen(toks, i)
			if bodyOpen == -1 {
				i++
				continue
			}
			name := funcQualifiedName(toks, i, typeStack)
			endIdx, complexity := scanFunctionBody(toks, bodyOpen)
			out = append(out, rawFunc{
				name:            name,
				startLine:       t.Line,
				endLine:         toks[endIdx].Line,
				complexity:      complexity,
				paramCount:      paramCount(toks, i),
				maxNestingDepth: maxNestingDepth(toks, bodyOpen, endIdx),
			})
			i = endIdx + 1

		// A computed property (no stored backing) or a stored property
		// with willSet/didSet observers. See extractPropertyFunctions.
		case t.Kind == swiftlex.Keyword && t.Text == "var":
			fns, nextIdx := extractPropertyFunctions(toks, i, typeStack)
			out = append(out, fns...)
			i = nextIdx

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
	return qualifiedName(base, typeStack)
}

// qualifiedName prefixes base with the innermost enclosing type's name
// (as tracked by typeStack), or returns it bare at file scope.
func qualifiedName(base string, typeStack []typeFrame) string {
	if len(typeStack) == 0 {
		return base
	}
	return typeStack[len(typeStack)-1].name + "." + base
}

// extractPropertyFunctions handles a "var" token found at varIdx by the
// main extractFunctions walk. Three shapes, disambiguated the same way
// Swift's own grammar does:
//
//   - A computed property with no stored backing ("var x: Int { ... }"):
//     its body — an implicit getter, or explicit get/set — is analyzed as
//     one or more functions ("Type.x", or "Type.x.get"/"Type.x.set").
//   - A stored property with observers ("var x: Int = 0 { didSet {...} }"):
//     only the observer(s) are analyzed ("Type.x.didSet"); the stored
//     declaration itself has nothing to measure.
//   - A plain stored property ("var x: Int" or "var x: Int = 0"), or one
//     whose default value merely contains a "{" that isn't an accessor
//     block (a closure literal, e.g. "var f: () -> Void = { ... }"):
//     nothing is emitted, mirroring the nested-local-func scope decision
//     — brace-track over what isn't ours rather than guess.
//
// Returns any functions found and the index to resume the outer walk at.
func extractPropertyFunctions(toks []swiftlex.Token, varIdx int, typeStack []typeFrame) (fns []rawFunc, nextIdx int) {
	t := toks[varIdx]
	bodyOpen := swiftlex.FindFuncBodyOpen(toks, varIdx)
	if bodyOpen == -1 {
		return nil, varIdx + 1
	}
	baseName := qualifiedName(propertyName(toks, varIdx), typeStack)
	if bodyOpen+1 < len(toks) && toks[bodyOpen+1].Kind == swiftlex.Keyword &&
		propertyAccessorKeywords[toks[bodyOpen+1].Text] {
		accessors, endIdx := scanPropertyAccessors(toks, bodyOpen, baseName)
		return accessors, endIdx + 1
	}
	if crossesTopLevelAssign(toks, varIdx+1, bodyOpen) {
		return nil, swiftlex.MatchBrace(toks, bodyOpen) + 1
	}
	endIdx, complexity := scanFunctionBody(toks, bodyOpen)
	return []rawFunc{{
		name:            baseName,
		startLine:       t.Line,
		endLine:         toks[endIdx].Line,
		complexity:      complexity,
		maxNestingDepth: maxNestingDepth(toks, bodyOpen, endIdx),
	}}, endIdx + 1
}

// propertyName returns a "var" declaration's own name — the identifier
// immediately following the "var" keyword. Falls back to "_" for a
// pattern this analyzer never actually walks into ("var (a, b) = ...", a
// tuple destructuring only legal as a local statement, never at the
// type-body/file scope the outer loop scans) rather than panicking.
func propertyName(toks []swiftlex.Token, varIdx int) string {
	if varIdx+1 < len(toks) &&
		(toks[varIdx+1].Kind == swiftlex.Ident || toks[varIdx+1].Kind == swiftlex.Keyword) {
		return toks[varIdx+1].Text
	}
	return "_"
}

// crossesTopLevelAssign reports whether toks[from:to] contains an "="
// outside any "(...)"/"[...]" nesting — a property declaration's default
// value ("var x: Int = 0"), as opposed to e.g. a default parameter's own
// "=" inside a function-type annotation's parameter list.
func crossesTopLevelAssign(toks []swiftlex.Token, from, to int) bool {
	depth := 0
	for i := from; i < to && i < len(toks); i++ {
		switch toks[i].Text {
		case "(", "[":
			depth++
		case ")", "]":
			depth--
		case "=":
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

// scanPropertyAccessors scans a property body opened at bodyOpen (a "{"
// immediately followed by one of propertyAccessorKeywords) for each
// accessor sub-block at the body's own top level, emitting one rawFunc
// per accessor found — "Type.prop.get", "Type.prop.set", and so on, each
// scored by its own scanFunctionBody the same way a real function is.
// Returns those functions and the index of the property body's own
// matching "}".
func scanPropertyAccessors(toks []swiftlex.Token, bodyOpen int, baseName string) (fns []rawFunc, endIdx int) {
	depth := 1
	i := bodyOpen + 1
	for i < len(toks) && depth > 0 {
		t := toks[i]
		if depth == 1 && t.Kind == swiftlex.Keyword && propertyAccessorKeywords[t.Text] {
			accessorTok := i
			j := i + 1
			if j < len(toks) && toks[j].Text == "(" {
				j = matchParen(toks, j) + 1
			}
			if j < len(toks) && toks[j].Text == "{" {
				accEnd, complexity := scanFunctionBody(toks, j)
				fns = append(fns, rawFunc{
					name:            baseName + "." + t.Text,
					startLine:       toks[accessorTok].Line,
					endLine:         toks[accEnd].Line,
					complexity:      complexity,
					paramCount:      paramCount(toks, accessorTok),
					maxNestingDepth: maxNestingDepth(toks, j, accEnd),
				})
				i = accEnd + 1
				continue
			}
		}
		switch t.Text {
		case "{":
			depth++
		case "}":
			depth--
		}
		i++
	}
	return fns, i - 1
}
