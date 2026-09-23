package arch

import (
	"fmt"
	"io"
	"sort"

	"git.roost-r.com/cadeh/quality-gates/internal/reportio"
)

// PackageStability holds Robert Martin's afferent/efferent coupling and
// derived instability for one package.
type PackageStability struct {
	Package     string  `json:"package"`
	Afferent    int     `json:"afferent"`    // Ca: distinct packages that depend on this one
	Efferent    int     `json:"efferent"`    // Ce: distinct packages this one depends on
	Instability float64 `json:"instability"` // Ce / (Ca + Ce); 0 (maximally stable) when Ca+Ce == 0
}

// StabilityViolation is one package dependency that points the wrong
// way: a more stable package (lower instability) depending on a less
// stable one, violating Martin's Stable Dependencies Principle — "depend
// in the direction of stability."
type StabilityViolation struct {
	FromPkg         string  `json:"from_pkg"`
	FromInstability float64 `json:"from_instability"`
	ToPkg           string  `json:"to_pkg"`
	ToInstability   float64 `json:"to_instability"`
	File            string  `json:"file"`
	Import          string  `json:"import"`
}

// packagePairs collapses file-level edges to their unique (FromPkg,
// ToPkg) package pairs, dropping intra-package edges. Martin's metrics
// are a property of the package graph, not the file graph — counting raw
// edges would let Go's importer (which fans a single package import out
// to every file in the target package, see internal/importers/golang)
// inflate a package's coupling by its target's file count instead of by
// how many packages it actually depends on.
func packagePairs(edges []Edge) []Edge {
	type key struct{ from, to string }
	seen := map[key]Edge{}
	for _, e := range edges {
		if e.FromPkg == e.ToPkg {
			continue
		}
		k := key{e.FromPkg, e.ToPkg}
		if existing, ok := seen[k]; !ok || e.File < existing.File {
			seen[k] = e
		}
	}
	pairs := make([]Edge, 0, len(seen))
	for _, e := range seen {
		pairs = append(pairs, e)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].FromPkg != pairs[j].FromPkg {
			return pairs[i].FromPkg < pairs[j].FromPkg
		}
		return pairs[i].ToPkg < pairs[j].ToPkg
	})
	return pairs
}

// Stabilities computes Martin's afferent/efferent coupling and
// instability for every package that appears in edges, on either side.
func Stabilities(edges []Edge) map[string]PackageStability {
	pairs := packagePairs(edges)

	afferent := map[string]int{}
	efferent := map[string]int{}
	pkgs := map[string]bool{}
	for _, e := range pairs {
		efferent[e.FromPkg]++
		afferent[e.ToPkg]++
		pkgs[e.FromPkg] = true
		pkgs[e.ToPkg] = true
	}

	out := make(map[string]PackageStability, len(pkgs))
	for pkg := range pkgs {
		ca, ce := afferent[pkg], efferent[pkg]
		var instability float64
		if ca+ce > 0 {
			instability = float64(ce) / float64(ca+ce)
		}
		out[pkg] = PackageStability{Package: pkg, Afferent: ca, Efferent: ce, Instability: instability}
	}
	return out
}

// CheckStability returns every package dependency that violates the
// Stable Dependencies Principle: a package depending on another package
// that is less stable (more instable) than itself.
func CheckStability(edges []Edge) []StabilityViolation {
	stabilities := Stabilities(edges)

	var out []StabilityViolation
	for _, e := range packagePairs(edges) {
		from, to := stabilities[e.FromPkg], stabilities[e.ToPkg]
		if from.Instability < to.Instability {
			out = append(out, StabilityViolation{
				FromPkg:         e.FromPkg,
				FromInstability: from.Instability,
				ToPkg:           e.ToPkg,
				ToInstability:   to.Instability,
				File:            e.File,
				Import:          e.Import,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FromPkg != out[j].FromPkg {
			return out[i].FromPkg < out[j].FromPkg
		}
		return out[i].ToPkg < out[j].ToPkg
	})
	return out
}

// StabilityReport is a full stable-dependencies analysis.
type StabilityReport struct {
	Violations    []StabilityViolation `json:"violations"`
	FilesAnalyzed int                  `json:"files_analyzed"`
	FailAbove     int                  `json:"fail_above"`
	Passed        bool                 `json:"passed"`
}

// ReadStabilityReport reads a report previously written by WriteJSON.
func ReadStabilityReport(r io.Reader) (StabilityReport, error) {
	return reportio.ReadReport[StabilityReport](r)
}

// StabilityRegressions returns violations present in new but not old. It
// matches package pairs rather than the representative file edge: stability
// is a package-graph property, and a file moving within an unchanged package
// relationship is not a new architectural regression.
func StabilityRegressions(old, new StabilityReport) []StabilityViolation {
	type key struct{ from, to string }
	previous := make(map[key]bool, len(old.Violations))
	for _, v := range old.Violations {
		previous[key{v.FromPkg, v.ToPkg}] = true
	}
	var out []StabilityViolation
	for _, v := range new.Violations {
		if !previous[key{v.FromPkg, v.ToPkg}] {
			out = append(out, v)
		}
	}
	return out
}

// NewStabilityReport builds a StabilityReport and applies the fail-above
// gate.
func NewStabilityReport(filesAnalyzed int, violations []StabilityViolation, failAbove int) StabilityReport {
	return StabilityReport{
		Violations:    violations,
		FilesAnalyzed: filesAnalyzed,
		FailAbove:     failAbove,
		Passed:        len(violations) <= failAbove,
	}
}

// ExitCode maps a StabilityReport's verdict to a process exit code.
func (r StabilityReport) ExitCode() int {
	return reportio.ExitCode(r.Passed)
}

// WriteJSON writes the full report as JSON.
func (r StabilityReport) WriteJSON(w io.Writer) error {
	return reportio.WriteJSON(w, r)
}

// WriteTable writes a human-readable table to w, one violation per row.
func (r StabilityReport) WriteTable(w io.Writer, top int) {
	fmt.Fprintf(w, "%d stability violation(s) found (%d files analyzed)\n\n", len(r.Violations), r.FilesAnalyzed)

	if len(r.Violations) == 0 {
		fmt.Fprintln(w, "PASS: no stable-dependency violations found")
		return
	}

	rows, truncated := truncateRows(r.Violations, top)
	for i, v := range rows {
		fmt.Fprintf(w, "%d. %s (I=%.2f) -> %s (I=%.2f)\n", i+1, v.FromPkg, v.FromInstability, v.ToPkg, v.ToInstability)
		fmt.Fprintf(w, "   %s imports %s\n", v.File, v.Import)
	}
	if truncated > 0 {
		fmt.Fprintf(w, "... %d more violation(s) not shown\n", truncated)
	}

	writeGateVerdict(w, len(r.Violations), r.FailAbove, "violation", r.Passed)
}
