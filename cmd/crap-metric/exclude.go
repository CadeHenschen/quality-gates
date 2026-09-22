package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"git.roost-r.com/cadeh/quality-gates/internal/crap"
	"git.roost-r.com/cadeh/quality-gates/internal/exclude"
)

// stringList is a repeatable string flag.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

func (l *stringList) Set(v string) error {
	*l = append(*l, v)
	return nil
}

// dropExcluded removes functions in --exclude'd files and returns how many
// it dropped. Applied before any report is built: exclusion is a scope
// declaration, not a ratchet.
func dropExcluded(fns []crap.Function, set exclude.Set) ([]crap.Function, int) {
	out := make([]crap.Function, 0, len(fns))
	for _, f := range fns {
		if !set.Matches(f.File) {
			out = append(out, f)
		}
	}
	return out, len(fns) - len(out)
}

// applyExcludes loads --exclude-file/--exclude patterns, compiles them, and
// drops matching functions from fns — the full sequence runCheck needs,
// factored out so runCheck itself stays under its own --max-lines default.
func applyExcludes(dir, excludeFile string, excludes stringList, fns []crap.Function, stdout io.Writer) ([]crap.Function, error) {
	patterns, err := loadExcludePatterns(dir, excludeFile, excludes, stdout)
	if err != nil {
		return nil, fmt.Errorf("reading exclude file: %w", err)
	}
	excluded, err := exclude.Compile(patterns)
	if err != nil {
		return nil, fmt.Errorf("bad --exclude pattern: %w", err)
	}
	fns, nExcluded := dropExcluded(fns, excluded)
	if nExcluded > 0 {
		fmt.Fprintf(stdout, "excluded: %d function(s) by --exclude\n\n", nExcluded)
	}
	return fns, nil
}

// loadExcludePatterns merges the exclude file's globs with --exclude flags.
// An explicit --exclude-file must exist; the default file in dir is optional
// and silently skipped when absent. The file used is announced so CI logs
// show where exclusions came from.
func loadExcludePatterns(dir, explicit string, flags []string, stdout io.Writer) ([]string, error) {
	path := explicit
	if path == "" {
		path = filepath.Join(dir, exclude.DefaultFile)
	}
	fromFile, err := exclude.ReadFile(path)
	switch {
	case err == nil:
		fmt.Fprintf(stdout, "exclude file: %s (%d pattern(s))\n", path, len(fromFile))
	case explicit == "" && errors.Is(err, fs.ErrNotExist):
		// default file is optional
	default:
		return nil, err
	}
	return append(fromFile, flags...), nil
}
