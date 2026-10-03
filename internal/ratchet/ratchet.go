// Package ratchet implements the shared half of --only-files: loading a
// changed-file list and matching an analyzer/tokenizer/scanner/importer's
// own --dir-relative file path against it. Each of the four CLIs (crap,
// dupe, escape, cycle) still keeps its own tool-specific filtering logic
// (a Function, a Clone pair, a Hatch, a Cycle each need different
// matching semantics — e.g. a Clone counts as touched if *either* side
// matches), but the load-and-match primitive underneath all four was
// identical, byte-for-byte, when this was four separate repos —
// extracted here as part of the migration into one module specifically
// to stop that duplication, see the repo's CLAUDE.md.
package ratchet

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"git.roost-r.com/cadeh/quality-gates/internal/repopath"
	"git.roost-r.com/cadeh/quality-gates/internal/safefile"
)

// LoadFiles reads a newline-separated list of file paths (as produced by
// `git diff --name-only`, relative to the repo root) into a set for
// matching via Matches.
func LoadFiles(path string) (map[string]bool, error) {
	data, err := safefile.ReadFile(path)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			set[filepath.ToSlash(filepath.Clean(line))] = true
		}
	}
	return set, nil
}

// MatchesInDir compares a report path relative to dir with changed paths
// relative to the repository root. If dir is outside Git, changed paths are
// interpreted relative to dir. Both paths are canonicalized before exact
// comparison, so a matching suffix in a different directory cannot match.
func MatchesInDir(itemFile string, onlyFiles map[string]bool, dir string) bool {
	if len(onlyFiles) == 0 {
		return false
	}
	root := repopath.Root(dir)
	baseDir := dir
	if dir == "" {
		// Reports without --dir (notably mutation reports) commonly use
		// repository-root-relative paths.
		baseDir = root
	}
	base, err := filepath.Abs(baseDir)
	if err != nil {
		return false
	}
	item, err := repopath.Relative(root, filepath.Join(base, itemFile))
	if err != nil {
		return false
	}
	return onlyFiles[item]
}

// MatchesPath compares a path expressed from the current working directory
// (or absolute) with repository-root-relative changed paths. It supports
// policy files that live outside the source scan directory.
func MatchesPath(path string, onlyFiles map[string]bool) bool {
	if len(onlyFiles) == 0 {
		return false
	}
	base := "."
	if filepath.IsAbs(path) {
		base = filepath.Dir(path)
	} else {
		path = filepath.Join(".", path)
	}
	root := repopath.Root(base)
	rel, err := repopath.Relative(root, path)
	return err == nil && onlyFiles[rel]
}

// Load wraps LoadFiles for a CLI's --only-files flag: path == "" means the
// flag wasn't passed (files is nil, ok is true, meaning "don't ratchet").
// A read failure is reported to stderr prefixed with toolName, matching
// the message shape every one of these CLIs used before this was
// extracted (identical across all four except for that prefix).
func Load(path, toolName string, stderr io.Writer) (files map[string]bool, ok bool) {
	if path == "" {
		return nil, true
	}
	loaded, err := LoadFiles(path)
	if err != nil {
		fmt.Fprintln(stderr, toolName+": reading --only-files:", err)
		return nil, false
	}
	return loaded, true
}
