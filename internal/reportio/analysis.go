package reportio

import (
	"fmt"
	"io"
)

// Analysis is the evidence behind a static gate's verdict.
type Analysis struct {
	Required   int      `json:"required"`
	Analyzed   int      `json:"analyzed"`
	Missing    []string `json:"missing,omitempty"`
	Unresolved []string `json:"unresolved_imports,omitempty"`
	Reason     string   `json:"reason,omitempty"`
	Passed     bool     `json:"passed"`
}

func (r Analysis) WithUnresolved(imports []string) Analysis {
	r.Unresolved = imports
	if len(imports) > 0 {
		r.Passed = false
		r.Reason = fmt.Sprintf("insufficient analysis: %d relative local import(s) could not be resolved", len(imports))
	}
	return r
}

func (r Analysis) WriteText(w io.Writer) {
	if r.Passed && r.Required == 0 {
		fmt.Fprintln(w, "analysis evidence: no eligible changed files; gate not applicable")
		return
	}
	fmt.Fprintf(w, "analysis evidence: %d/%d eligible file(s) analyzed\n", r.Analyzed, r.Required)
	if !r.Passed {
		fmt.Fprintf(w, "FAIL: %s\n", r.Reason)
		for _, file := range r.Missing {
			fmt.Fprintf(w, "  missing: %s\n", file)
		}
		for _, imp := range r.Unresolved {
			fmt.Fprintf(w, "  unresolved import: %s\n", imp)
		}
	}
}
