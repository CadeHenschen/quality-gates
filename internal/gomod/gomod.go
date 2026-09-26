// Package gomod resolves a directory's nearest enclosing Go module: its
// root directory and module path, both needed to turn a source file's
// on-disk location into its Go import path. Extracted out of
// internal/analyzers/golang so arch-metric's Go importer doesn't grow a
// second, silently-drifting copy of the same nearest-go.mod walk — see
// the repo's CLAUDE.md on keeping genuine duplication out.
package gomod

import (
	"fmt"
	"path/filepath"
	"strings"

	"git.roost-r.com/cadeh/quality-gates/internal/safefile"
)

// Find locates the nearest go.mod at or above dir and returns its
// directory and module path.
func Find(dir string) (root, modulePath string, err error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	for d := abs; ; {
		modFile := filepath.Join(d, "go.mod")
		if data, err := safefile.ReadFile(modFile); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "module ") {
					return d, strings.TrimSpace(strings.TrimPrefix(line, "module")), nil
				}
			}
			return "", "", fmt.Errorf("%s: no module directive found", modFile)
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", "", fmt.Errorf("no go.mod found above %s", abs)
		}
		d = parent
	}
}
