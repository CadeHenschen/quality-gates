// Package arch implements declared layer rules: a small, explicit set of
// "package X may not import package Y" boundaries, checked structurally
// against the real import graph. This is the Dependency Inversion
// Principle as a CI gate — Go's compiler already refuses to build an
// import cycle (see internal/cycle's package doc) but has no opinion at
// all on layering: a perfectly acyclic, perfectly buildable program can
// still have its leaf packages reaching back into ones that are supposed
// to depend on them instead. A rule failure is binary (the import either
// exists or it doesn't), never a judgment call, which is exactly what a
// gate needs.
package arch

import (
	"fmt"

	"git.roost-r.com/cadeh/quality-gates/internal/exclude"
)

// Rule declares one boundary: a package matching From may not import any
// package matching one of Deny.
//
// Patterns reuse internal/exclude's glob syntax unchanged — the same one
// --exclude and dead-metric's --ignore already use — matched against a
// --dir-relative package (directory) path: a pattern with no wildcard
// matches that directory and everything under it ("internal/domain"
// matches "internal/domain" itself and any "internal/domain/...");
// "*" stays within one path segment ("cmd/*" matches each "cmd/X" and
// everything under it, but not "cmd" itself or "cmd/x/y"); "**" spans
// segments.
type Rule struct {
	Name string   `json:"name"`
	From string   `json:"from"`
	Deny []string `json:"deny"`
}

// compiledRule is a Rule with its patterns pre-compiled.
type compiledRule struct {
	Rule
	from exclude.Set
	deny exclude.Set
}

// RuleSet is a validated, compiled set of rules and their exceptions,
// ready to Check edges against.
type RuleSet struct {
	rules      []compiledRule
	exceptions []compiledException
}

// Compile validates and compiles rules and exceptions into a RuleSet.
func Compile(rules []Rule, exceptions []Exception) (RuleSet, error) {
	var rs RuleSet
	for _, r := range rules {
		if r.Name == "" {
			return RuleSet{}, fmt.Errorf("rule missing \"name\"")
		}
		if r.From == "" {
			return RuleSet{}, fmt.Errorf("rule %q: missing \"from\"", r.Name)
		}
		if len(r.Deny) == 0 {
			return RuleSet{}, fmt.Errorf("rule %q: missing \"deny\"", r.Name)
		}
		from, err := exclude.Compile([]string{r.From})
		if err != nil {
			return RuleSet{}, fmt.Errorf("rule %q: bad \"from\" pattern %q: %w", r.Name, r.From, err)
		}
		deny, err := exclude.Compile(r.Deny)
		if err != nil {
			return RuleSet{}, fmt.Errorf("rule %q: bad \"deny\" pattern: %w", r.Name, err)
		}
		rs.rules = append(rs.rules, compiledRule{Rule: r, from: from, deny: deny})
	}

	compiledExceptions, err := compileExceptions(exceptions)
	if err != nil {
		return RuleSet{}, err
	}
	rs.exceptions = compiledExceptions

	return rs, nil
}

// isExempt reports whether v is grandfathered by one of rs's exceptions.
func (rs RuleSet) isExempt(v Violation) bool {
	for _, ex := range rs.exceptions {
		if ex.matches(v) {
			return true
		}
	}
	return false
}
