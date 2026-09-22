package arch

import (
	"fmt"

	"git.roost-r.com/cadeh/quality-gates/internal/exclude"
)

// Exception grandfathers one specific, already-known violation of a
// named rule: an edge whose FromPkg matches From and whose ToPkg matches
// To is dropped entirely — from both the report and the gate, the same
// way dead-metric's --ignore and crap-metric's --exclude drop a finding
// outright, unlike --only-files, which only narrows the gate. Reason is
// optional but conventionally carries a "why", so it survives review the
// same way an --exclude file's trailing comment does.
//
// An exception is scoped to one named Rule on purpose: a bare from/to
// glob with no rule name would silently exempt that package pair from
// *every* rule that happens to match it, including ones added later —
// an easy way to build an unintentional loophole.
type Exception struct {
	Rule   string `json:"rule"`
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason,omitempty"`
}

// compiledException is an Exception with its patterns pre-compiled.
type compiledException struct {
	Exception
	from exclude.Set
	to   exclude.Set
}

func compileExceptions(exceptions []Exception) ([]compiledException, error) {
	var out []compiledException
	for _, ex := range exceptions {
		if ex.Rule == "" {
			return nil, fmt.Errorf("exception missing \"rule\"")
		}
		if ex.From == "" {
			return nil, fmt.Errorf("exception for rule %q: missing \"from\"", ex.Rule)
		}
		if ex.To == "" {
			return nil, fmt.Errorf("exception for rule %q: missing \"to\"", ex.Rule)
		}
		from, err := exclude.Compile([]string{ex.From})
		if err != nil {
			return nil, fmt.Errorf("exception for rule %q: bad \"from\" pattern %q: %w", ex.Rule, ex.From, err)
		}
		to, err := exclude.Compile([]string{ex.To})
		if err != nil {
			return nil, fmt.Errorf("exception for rule %q: bad \"to\" pattern %q: %w", ex.Rule, ex.To, err)
		}
		out = append(out, compiledException{Exception: ex, from: from, to: to})
	}
	return out, nil
}

// matches reports whether v is exactly the violation this exception
// grandfathers.
func (ce compiledException) matches(v Violation) bool {
	return ce.Rule == v.Rule && ce.from.Matches(v.FromPkg) && ce.to.Matches(v.ToPkg)
}
