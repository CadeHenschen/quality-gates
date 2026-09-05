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
	"os"
	"path/filepath"
	"strings"
)

// LoadFiles reads a newline-separated list of file paths (as produced by
// `git diff --name-only`, relative to the repo root) into a set for
// matching via Matches.
func LoadFiles(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			set[filepath.ToSlash(line)] = true
		}
	}
	return set, nil
}

// Matches reports whether itemFile — as recorded by a tool, relative to
// --dir — corresponds to one of the changed paths in onlyFiles —
// relative to the repo root, from `git diff --name-only`. The two are
// relative to different roots (a tool doesn't know the repo root), so
// matching is by path suffix on "/" boundaries rather than exact
// equality: "app/src/pages/Foo.tsx" (changed, repo-root-relative)
// matches item file "pages/Foo.tsx" (--dir-relative) because the former
// ends with "/pages/Foo.tsx".
func Matches(itemFile string, onlyFiles map[string]bool) bool {
	item := filepath.ToSlash(itemFile)
	for changed := range onlyFiles {
		if changed == item || strings.HasSuffix(changed, "/"+item) {
			return true
		}
	}
	return false
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
