package golang

import "go/ast"

// paramCount counts a function's parameters, one per name in a multi-name
// field ("a, b int" is 2) and one for an unnamed field (rare — an
// interface-shaped func type). The receiver isn't counted: go/ast keeps it
// separate from fn.Type.Params already, matching how crap-metric's
// complexity score never counted it either.
func paramCount(fn *ast.FuncDecl) int {
	if fn.Type.Params == nil {
		return 0
	}
	n := 0
	for _, field := range fn.Type.Params.List {
		if len(field.Names) == 0 {
			n++
		} else {
			n += len(field.Names)
		}
	}
	return n
}

// maxNestingDepth returns the deepest level of control-flow block nesting
// (if/for/range/switch/select) in fn's body, 0 for a flat function.
//
// Two deliberate v1 scope choices, so a long chain or a wide switch don't
// get penalized for something that reads just fine:
//   - A chained "else if" does NOT nest deeper than its "if" — it reads
//     like a switch's cases, the same reasoning size-metric exists for in
//     the first place (see the package's CLAUDE.md entry). Only a genuine
//     nested block (a bare "else { ... }", or an "if" inside one) adds a
//     level.
//   - Nested closures (a func literal passed to defer/go/or as a plain
//     expression) are opaque here: their own internal nesting isn't
//     walked. This differs from cyclomaticComplexity, which does walk
//     into them via ast.Inspect — under-counting nesting depth is the
//     safe direction for a generous, egregious-cases-only gate.
func maxNestingDepth(fn *ast.FuncDecl) int {
	if fn.Body == nil {
		return 0
	}
	return nestingDepth(fn.Body.List, 0)
}

func nestingDepth(stmts []ast.Stmt, depth int) int {
	max := depth
	for _, stmt := range stmts {
		if d := nestingOfStmt(stmt, depth); d > max {
			max = d
		}
	}
	return max
}

// nestingOfStmt returns the deepest nesting level reached within stmt,
// given stmt itself sits at depth.
func nestingOfStmt(stmt ast.Stmt, depth int) int {
	switch s := stmt.(type) {
	case *ast.IfStmt:
		return nestingOfIf(s, depth)
	case *ast.ForStmt:
		return nestingDepth(s.Body.List, depth+1)
	case *ast.RangeStmt:
		return nestingDepth(s.Body.List, depth+1)
	case *ast.SwitchStmt:
		return nestingOfClauses(s.Body.List, depth)
	case *ast.TypeSwitchStmt:
		return nestingOfClauses(s.Body.List, depth)
	case *ast.SelectStmt:
		return nestingOfComm(s.Body.List, depth)
	case *ast.BlockStmt:
		return nestingDepth(s.List, depth+1)
	default:
		return depth
	}
}

func nestingOfIf(s *ast.IfStmt, depth int) int {
	max := nestingDepth(s.Body.List, depth+1)
	switch e := s.Else.(type) {
	case *ast.IfStmt:
		// "else if": a chain link, not a deeper level.
		if d := nestingOfStmt(e, depth); d > max {
			max = d
		}
	case *ast.BlockStmt:
		if d := nestingDepth(e.List, depth+1); d > max {
			max = d
		}
	}
	return max
}

func nestingOfClauses(stmts []ast.Stmt, depth int) int {
	max := depth
	for _, stmt := range stmts {
		cc, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		if d := nestingDepth(cc.Body, depth+1); d > max {
			max = d
		}
	}
	return max
}

func nestingOfComm(stmts []ast.Stmt, depth int) int {
	max := depth
	for _, stmt := range stmts {
		cc, ok := stmt.(*ast.CommClause)
		if !ok {
			continue
		}
		if d := nestingDepth(cc.Body, depth+1); d > max {
			max = d
		}
	}
	return max
}
