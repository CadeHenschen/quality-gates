package escape

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"git.roost-r.com/cadeh/quality-gates/internal/safefile"
)

// Hatch is one matched escape-hatch occurrence.
type Hatch struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Pattern string `json:"pattern"`
	Text    string `json:"text"`
}

// Options carries the inputs Scan needs.
type Options struct {
	Dir  string
	Lang string
}

// Result is everything a scan produces: every matched hatch, the total
// lines analyzed, and each file's own line count — the latter so a
// ratcheted gate (--only-files) can recompute a rate scoped to just the
// files it cares about, rather than diluting a small change's hatches
// against the whole repo's line count.
type Result struct {
	Hatches     []Hatch
	TotalLines  int
	LinesByFile map[string]int
}

// Scan walks Dir for Lang's source files (skipping vendor/node_modules/
// testdata/dotdirs and that language's test-file convention) and matches
// every line against that language's patterns.
func Scan(opts Options) (result Result, retErr error) {
	lp, ok := Resolve(opts.Lang)
	if !ok {
		return Result{}, fmt.Errorf("unknown --lang %q", opts.Lang)
	}

	result = Result{LinesByFile: map[string]int{}}
	root, err := safefile.OpenRoot(opts.Dir)
	if err != nil {
		return Result{}, err
	}
	defer func() {
		retErr = errors.Join(retErr, root.Close())
	}()

	err = filepath.WalkDir(opts.Dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == "node_modules" || name == "testdata" ||
				(strings.HasPrefix(name, ".") && path != opts.Dir) {
				return filepath.SkipDir
			}
			return nil
		}
		if !hasAnyExt(path, lp.Extensions) || hasAnySuffix(path, lp.TestSuffixes) {
			return nil
		}

		rel, relErr := filepath.Rel(opts.Dir, path)
		if relErr != nil {
			return relErr
		}
		fileHatches, lines, err := scanFile(root, filepath.ToSlash(rel), rel, lp.Patterns)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		result.Hatches = append(result.Hatches, fileHatches...)
		result.TotalLines += lines
		result.LinesByFile[rel] = lines
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

func scanFile(root *os.Root, openPath, reportPath string, patterns []Pattern) ([]Hatch, int, error) {
	src, err := safefile.ReadFileAt(root, openPath)
	if err != nil {
		return nil, 0, err
	}

	var hatches []Hatch
	lineNo := 0
	scanner := bufio.NewScanner(bytes.NewReader(src))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		for _, p := range patterns {
			if p.Regex.MatchString(line) {
				hatches = append(hatches, Hatch{
					File:    reportPath,
					Line:    lineNo,
					Pattern: p.Name,
					Text:    strings.TrimSpace(line),
				})
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, lineNo, err
	}
	return hatches, lineNo, nil
}

func hasAnyExt(path string, exts []string) bool {
	for _, e := range exts {
		if strings.HasSuffix(path, e) {
			return true
		}
	}
	return false
}

func hasAnySuffix(path string, suffixes []string) bool {
	for _, s := range suffixes {
		if strings.HasSuffix(path, s) {
			return true
		}
	}
	return false
}
