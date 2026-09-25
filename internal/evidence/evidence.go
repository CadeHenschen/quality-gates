// Package evidence checks whether a quality gate examined the files it was
// expected to examine. A clean finding list is meaningful only with input.
package evidence

import (
	"fmt"
	"git.roost-r.com/cadeh/quality-gates/internal/reportio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Report aliases the reusable report shape without making core metric
// packages depend on the file-discovery implementation.
type Report = reportio.Analysis

// Changed is exact repo-root-relative matching for an analyzed file.
func Changed(dir, file string, changed map[string]bool) bool {
	if changed == nil {
		return true
	}
	abs, err := filepath.Abs(filepath.Join(dir, file))
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(findRoot(dir), abs)
	return err == nil && changed[filepath.ToSlash(rel)]
}

// Eligible returns source or test files in dir, relative to dir. It uses
// the same broad language file conventions as the scanners; callers can
// further narrow the result for gate-specific exclusions.
func Eligible(dir, lang string, tests bool) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if path != dir && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "testdata" || name == ".build" || name == ".swiftpm" || name == "Pods" || name == "venv" || name == "__pycache__" || (lang == "swift" && !tests && name == "Tests")) {
				return filepath.SkipDir
			}
			return nil
		}
		name := entry.Name()
		if !languageFile(name, lang, tests) {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(files)
	return files, err
}

func languageFile(name, lang string, tests bool) bool {
	switch lang {
	case "go", "golang":
		return strings.HasSuffix(name, "_test.go") == tests && strings.HasSuffix(name, ".go")
	case "python", "py":
		isTest := strings.HasPrefix(name, "test_") || strings.HasSuffix(name, "_test.py")
		return strings.HasSuffix(name, ".py") && isTest == tests
	case "ts", "typescript", "js", "javascript":
		if strings.HasSuffix(name, ".d.ts") {
			return false
		}
		ext := strings.ToLower(filepath.Ext(name))
		isTest := strings.Contains(name, ".test.") || strings.Contains(name, ".spec.")
		return (ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".jsx") && isTest == tests
	case "swift":
		isTest := strings.HasSuffix(name, "Tests.swift") || strings.HasSuffix(name, "Test.swift")
		return strings.HasSuffix(name, ".swift") && isTest == tests
	}
	return false
}

// Check compares eligible files with file paths actually recorded by a
// scanner. Changed paths are repo-root-relative; findRoot makes matching
// exact so same-named files in other directories cannot satisfy evidence.
// A ratchet with no eligible changed files is not applicable and passes.
func Check(dir, lang string, tests bool, analyzed []string, changed map[string]bool) (Report, error) {
	return CheckFiltered(dir, lang, tests, analyzed, changed, nil)
}

// CheckFiltered excludes intentionally ignored paths from the evidence
// expectation, using the same predicate as the analyzer's own exclusions.
func CheckFiltered(dir, lang string, tests bool, analyzed []string, changed map[string]bool, excluded func(string) bool) (Report, error) {
	eligible, err := Eligible(dir, lang, tests)
	if err != nil {
		return Report{}, err
	}
	if excluded != nil {
		var kept []string
		for _, file := range eligible {
			if !excluded(file) {
				kept = append(kept, file)
			}
		}
		eligible = kept
	}
	return checkFiles(dir, lang, eligible, analyzed, changed)
}

// CheckAll includes both source and test files, as import-graph gates do.
func CheckAll(dir, lang string, analyzed []string, changed map[string]bool) (Report, error) {
	source, err := Eligible(dir, lang, false)
	if err != nil {
		return Report{}, err
	}
	tests, err := Eligible(dir, lang, true)
	if err != nil {
		return Report{}, err
	}
	return checkFiles(dir, lang, append(source, tests...), analyzed, changed)
}

func checkFiles(dir, lang string, eligible, analyzed []string, changed map[string]bool) (Report, error) {
	if changed != nil {
		root := findRoot(dir)
		var scoped []string
		for _, file := range eligible {
			abs, err := filepath.Abs(filepath.Join(dir, file))
			if err != nil {
				return Report{}, err
			}
			rel, err := filepath.Rel(root, abs)
			if err != nil {
				return Report{}, err
			}
			if changed[filepath.ToSlash(rel)] {
				scoped = append(scoped, file)
			}
		}
		eligible = scoped
	}
	seen := map[string]bool{}
	for _, file := range analyzed {
		seen[filepath.ToSlash(filepath.Clean(file))] = true
	}
	r := Report{Required: len(eligible), Passed: true}
	for _, file := range eligible {
		if seen[file] {
			r.Analyzed++
		} else {
			r.Missing = append(r.Missing, file)
		}
	}
	if changed == nil && r.Required == 0 {
		r.Passed = false
		r.Reason = fmt.Sprintf("insufficient analysis: no eligible %s files under %s", lang, dir)
	} else if len(r.Missing) > 0 {
		r.Passed = false
		r.Reason = fmt.Sprintf("insufficient analysis: %d eligible file(s) were not analyzed", len(r.Missing))
	}
	return r, nil
}

func findRoot(dir string) string {
	cur, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	for {
		if _, err := os.Stat(filepath.Join(cur, ".git")); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	abs, _ := filepath.Abs(dir)
	return abs
}
