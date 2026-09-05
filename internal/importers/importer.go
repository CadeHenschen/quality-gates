// Package importers defines the common interface each language importer
// implements to build a cycle.Graph.
package importers

import "git.roost-r.com/cadeh/quality-gates/internal/cycle"

// Options carries the inputs an importer needs.
type Options struct {
	// Dir is the source directory to scan.
	Dir string
}

// Importer builds an import graph for one language's source files, and
// reports how many files it analyzed.
type Importer interface {
	Import(opts Options) (cycle.Graph, int, error)
}
