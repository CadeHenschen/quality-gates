package swift

import "git.roost-r.com/cadeh/quality-gates/internal/swiftlex"

// paramCount counts a func/init/subscript's declared parameters by
// scanning forward from its keyword to the first top-level "(" (skipping
// past a generic clause like "<T>", which never contains one) and
// counting top-level commas inside it (tracking nested "(", "[", "{" so a
// default closure argument or a tuple/array type's own commas don't
// count). "deinit" has no parameter list at all — the scan hits the
// body's "{" before any "(" and returns 0.
func paramCount(toks []swiftlex.Token, funcIdx int) int {
	open := -1
	for i := funcIdx + 1; i < len(toks); i++ {
		if toks[i].Text == "(" {
			open = i
		}
		if open != -1 || toks[i].Text == "{" {
			break
		}
	}
	if open == -1 {
		return 0
	}
	close := matchParen(toks, open)
	if close <= open+1 {
		return 0
	}
	n := 1
	depth := 0
	for i := open + 1; i < close; i++ {
		switch toks[i].Text {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
		case ",":
			if depth == 0 {
				n++
			}
		}
	}
	return n
}

func matchParen(toks []swiftlex.Token, open int) int {
	depth := 0
	for i := open; i < len(toks); i++ {
		switch toks[i].Text {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(toks) - 1
}

// controlKeywords are the keywords whose next top-level "{" opens a
// control-flow block that should count toward nesting depth. "else" is
// handled separately (see classifyControlBraces) since an "else if"
// chain deliberately does NOT nest — it reads like a switch's cases, the
// same reasoning size-metric exists for in the first place.
var controlKeywords = map[string]bool{
	"if": true, "guard": true, "while": true, "for": true,
	"switch": true, "catch": true,
}

// maxNestingDepth returns the deepest level of control-flow block nesting
// within [bodyOpen, bodyEnd] (a function's own "{"..."}"), 0 for a flat
// function. Mirrors the Go/TypeScript analyzers' algorithm — see
// internal/analyzers/golang/size.go — with two Swift-specific notes:
//
//   - Swift's switch has no per-case braces (a case's statements run to
//     the next case/default/the switch's own closing "}"), so the whole
//     switch body is treated as a single +1 level rather than +1 per case
//     the way Go's CaseClause bodies are — a coarser approximation, but
//     the same "many cases shouldn't explode the score" property holds.
//   - Nested local functions/closures/types are opaque here (their own
//     "{" is never classified as a control brace), the safe
//     under-counting direction for a generous gate — same choice the Go
//     analyzer makes for closures.
func maxNestingDepth(toks []swiftlex.Token, bodyOpen, bodyEnd int) int {
	real := classifyControlBraces(toks, bodyOpen, bodyEnd)
	depth, max := 0, 0
	var stack []bool
	for i := bodyOpen; i <= bodyEnd; i++ {
		switch toks[i].Text {
		case "{":
			isReal := real[i]
			if isReal {
				depth++
				if depth > max {
					max = depth
				}
			}
			stack = append(stack, isReal)
		case "}":
			if len(stack) == 0 {
				continue
			}
			isReal := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if isReal {
				depth--
			}
		}
	}
	return max
}

// classifyControlBraces finds, for every control-flow keyword in
// [bodyOpen, bodyEnd], the "{" that opens its block, and marks it real
// (should add a nesting level) — except an "else if" continuation's "if",
// which is marked NOT real so the chain stays flat like a switch's cases.
func classifyControlBraces(toks []swiftlex.Token, bodyOpen, bodyEnd int) map[int]bool {
	real := map[int]bool{}
	for i := bodyOpen; i <= bodyEnd; i++ {
		t := toks[i]
		if t.Kind != swiftlex.Keyword {
			continue
		}
		if funcKeywords[t.Text] {
			// A nested local func/init/deinit/subscript: its own control
			// flow is opaque to the enclosing function's nesting depth
			// (mirrors the Go analyzer's closure handling) — jump past
			// its whole body rather than let its keywords get classified
			// as if they belonged to the outer function directly.
			if open := swiftlex.FindFuncBodyOpen(toks, i); open != -1 {
				i = swiftlex.MatchBrace(toks, open)
			}
			continue
		}
		switch {
		case t.Text == "else" && nextMeaningful(toks, i) != "if":
			if brace := findBodyBrace(toks, i, bodyEnd); brace != -1 {
				real[brace] = true
			}
		case controlKeywords[t.Text]:
			brace := findBodyBrace(toks, i, bodyEnd)
			if brace == -1 {
				continue
			}
			isElseIf := t.Text == "if" && prevMeaningful(toks, i) == "else"
			real[brace] = !isElseIf
		}
	}
	return real
}

// findBodyBrace scans forward from just after toks[from] for the first
// "{" at paren/bracket depth 0 (skipping over a condition/pattern's own
// "(...)"/"[...]"), returning -1 if none appears by limit. Swift
// disallows a trailing closure directly in an if/while/guard condition
// (exactly to avoid this ambiguity), so a depth-0 "{" is always the
// construct's real body.
func findBodyBrace(toks []swiftlex.Token, from, limit int) int {
	depth := 0
	for i := from + 1; i <= limit && i < len(toks); i++ {
		switch toks[i].Text {
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
		}
	}
	return -1
}

func nextMeaningful(toks []swiftlex.Token, i int) string {
	for j := i + 1; j < len(toks); j++ {
		if toks[j].Kind == swiftlex.Comment {
			continue
		}
		return toks[j].Text
	}
	return ""
}

func prevMeaningful(toks []swiftlex.Token, i int) string {
	for j := i - 1; j >= 0; j-- {
		if toks[j].Kind == swiftlex.Comment {
			continue
		}
		return toks[j].Text
	}
	return ""
}
