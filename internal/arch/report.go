package arch

import (
	"fmt"
	"io"
	"sort"

	"git.roost-r.com/cadeh/quality-gates/internal/cycle"
	"git.roost-r.com/cadeh/quality-gates/internal/reportio"
)

// Violation is one edge that breaks a declared layer rule.
type Violation struct {
	Rule    string `json:"rule"`
	FromPkg string `json:"from_pkg"`
	ToPkg   string `json:"to_pkg"`
	File    string `json:"file"`
	Import  string `json:"import"`
}

// Check runs every rule in rs against edges and returns every violation:
// an edge whose FromPkg matches a rule's From and whose ToPkg matches one
// of that rule's Deny patterns. An edge within a single package (FromPkg
// == ToPkg) is never a layering question, regardless of rules, and is
// always skipped. A violation matching one of rs's exceptions is dropped
// entirely — never returned at all, the same way an excluded file or an
// ignored dead-code finding never reaches its tool's report either.
func (rs RuleSet) Check(edges []Edge) []Violation {
	var out []Violation
	for _, e := range edges {
		if e.FromPkg == e.ToPkg {
			continue
		}
		for _, r := range rs.rules {
			if !r.from.Matches(e.FromPkg) || !r.deny.Matches(e.ToPkg) {
				continue
			}
			v := Violation{
				Rule:    r.Name,
				FromPkg: e.FromPkg,
				ToPkg:   e.ToPkg,
				File:    e.File,
				Import:  e.Import,
			}
			if rs.isExempt(v) {
				continue
			}
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Rule != out[j].Rule {
			return out[i].Rule < out[j].Rule
		}
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Import < out[j].Import
	})
	return out
}

// Report is a full layer-rule analysis.
type Report struct {
	Analysis          *reportio.Analysis  `json:"analysis,omitempty"`
	UnresolvedImports []cycle.ImportIssue `json:"unresolved_imports,omitempty"`
	Violations        []Violation         `json:"violations"`
	FilesAnalyzed     int                 `json:"files_analyzed"`
	FailAbove         int                 `json:"fail_above"`
	Passed            bool                `json:"passed"`
}

// NewReport builds a Report and applies the fail-above gate (a raw
// violation count — a boundary is either crossed or it isn't, not a rate
// to normalize).
func NewReport(filesAnalyzed int, violations []Violation, failAbove int) Report {
	return Report{
		Violations:    violations,
		FilesAnalyzed: filesAnalyzed,
		FailAbove:     failAbove,
		Passed:        len(violations) <= failAbove,
	}
}

// ExitCode maps a Report's verdict to a process exit code.
func (r Report) ExitCode() int {
	return reportio.ExitCode(r.Passed)
}

// WriteJSON writes the full report as JSON.
func (r Report) WriteJSON(w io.Writer) error {
	return reportio.WriteJSON(w, r)
}

// ReadReport reads back a report written by WriteJSON — used by the
// diff subcommand to load the two reports it compares.
func ReadReport(r io.Reader) (Report, error) {
	return reportio.ReadReport[Report](r)
}

// WriteTable writes a human-readable table to w, one violation per row
// group.
func (r Report) WriteTable(w io.Writer, top int) {
	fmt.Fprintf(w, "%d violation(s) found (%d files analyzed)\n\n", len(r.Violations), r.FilesAnalyzed)

	if len(r.Violations) == 0 {
		fmt.Fprintln(w, "PASS: no layer rule violations found")
		return
	}

	rows, truncated := truncateRows(r.Violations, top)
	for i, v := range rows {
		fmt.Fprintf(w, "%d. [%s] %s -> %s\n", i+1, v.Rule, v.FromPkg, v.ToPkg)
		fmt.Fprintf(w, "   %s imports %s\n", v.File, v.Import)
	}
	if truncated > 0 {
		fmt.Fprintf(w, "... %d more violation(s) not shown\n", truncated)
	}

	writeGateVerdict(w, len(r.Violations), r.FailAbove, "violation", r.Passed)
}
