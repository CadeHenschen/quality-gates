// Package testscanners defines the interface each language's test scanner
// implements to produce testmetric.Test facts. Same shape as the
// tokenizers/importers/analyzers trees: one adapter per language, all
// resolving File relative to Dir (see CLAUDE.md — ratchet matching
// silently breaks otherwise).
package testscanners

import "git.roost-r.com/cadeh/quality-gates/internal/testmetric"

// Scanner extracts per-test facts from one language's test files under dir.
type Scanner interface {
	Scan(dir string) ([]testmetric.Test, error)
}
