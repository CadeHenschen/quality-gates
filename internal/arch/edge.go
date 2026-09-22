package arch

import (
	"path/filepath"

	"git.roost-r.com/cadeh/quality-gates/internal/cycle"
)

// Edge is one file's resolved import of another local file, already
// carrying both files' own package (directory), so rule matching never
// needs to re-derive it.
type Edge struct {
	File    string `json:"file"`     // importing file, --dir-relative
	FromPkg string `json:"from_pkg"` // File's package (directory), --dir-relative
	Import  string `json:"import"`   // imported file, --dir-relative
	ToPkg   string `json:"to_pkg"`   // Import's package (directory), --dir-relative
}

// EdgesFromFileGraph converts a file-level import graph — the same
// cycle.Graph cycle-metric's python/typescript importers already build —
// into Edges, deriving each side's package from its own directory. This
// is the reuse cycle-metric's importers make possible: neither importer
// changes at all, arch-metric just reads the graph they already produce.
func EdgesFromFileGraph(g cycle.Graph) []Edge {
	var edges []Edge
	for from, tos := range g.Edges {
		fromPkg := pkgOf(from)
		for _, to := range tos {
			edges = append(edges, Edge{
				File:    from,
				FromPkg: fromPkg,
				Import:  to,
				ToPkg:   pkgOf(to),
			})
		}
	}
	return edges
}

// pkgOf returns file's containing directory as a --dir-relative slash
// path, "." for a top-level file — the same convention every importer
// already uses for its own fileDir.
func pkgOf(file string) string {
	return filepath.ToSlash(filepath.Dir(file))
}
