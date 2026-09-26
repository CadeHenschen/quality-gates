// Package typescript builds an import graph for TS/JS source via regex
// extraction of import/require specifiers — same reasoning as the Python
// importer: no real parser needed, since matching an import statement's
// string specifier is regular enough, and resolution (does this
// specifier correspond to a scanned file) is where the real work is.
//
// Only *relative* specifiers ("./x", "../x") are resolved — bare imports
// ("react") and path-alias imports ("@/lib/foo", needing tsconfig
// path-mapping to resolve) are treated as external and dropped. This is
// the overwhelming majority of same-package imports in practice, and the
// ones most likely to form an accidental cycle; see README.
package typescript

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"git.roost-r.com/cadeh/quality-gates/internal/cycle"
	"git.roost-r.com/cadeh/quality-gates/internal/importers"
	"git.roost-r.com/cadeh/quality-gates/internal/safefile"
)

type Importer struct{}

// Matches the specifier string in any of: `import ... from "x"`,
// `export ... from "x"`, `import "x"`, `import("x")`, `require("x")` —
// single or double quotes.
var specifierRe = regexp.MustCompile(`(?:from\s+|import\s*(?:\(\s*)?|require\s*\(\s*)['"]([^'"]+)['"]`)

var codeExtensions = []string{".ts", ".tsx", ".js", ".jsx"}

func (Importer) Import(opts importers.Options) (graph cycle.Graph, scanned int, retErr error) {
	root, err := safefile.OpenRoot(opts.Dir)
	if err != nil {
		return cycle.Graph{}, 0, err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	files, err := walkCode(opts.Dir)
	if err != nil {
		return cycle.Graph{}, 0, err
	}

	existing := map[string]bool{}
	for _, f := range files {
		existing[f] = true
	}

	graph = cycle.Graph{Edges: map[string][]string{}}
	for _, f := range files {
		targets, unresolved, err := resolveWithIssues(root, f, existing)
		if err != nil {
			return cycle.Graph{}, 0, err
		}
		graph.Edges[f] = targets
		graph.Unresolved = append(graph.Unresolved, unresolved...)
	}

	return graph, len(files), nil
}

func walkCode(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "node_modules" || name == "testdata" ||
				(strings.HasPrefix(name, ".") && path != dir) {
				return filepath.SkipDir
			}
			return nil
		}
		for _, ext := range codeExtensions {
			if strings.HasSuffix(path, ext) && !strings.HasSuffix(path, ".d.ts") {
				rel, err := filepath.Rel(dir, path)
				if err != nil {
					return err
				}
				files = append(files, filepath.ToSlash(rel))
				break
			}
		}
		return nil
	})
	return files, err
}

func resolveWithIssues(root *os.Root, file string, existing map[string]bool) ([]string, []cycle.ImportIssue, error) {
	data, err := safefile.ReadFileAt(root, file)
	if err != nil {
		return nil, nil, err
	}

	fileDir := filepath.ToSlash(filepath.Dir(file))
	seen := map[string]bool{}
	var targets []string
	var unresolved []cycle.ImportIssue

	for _, m := range specifierRe.FindAllStringSubmatch(withoutComments(data), -1) {
		spec := m[1]
		if !strings.HasPrefix(spec, ".") {
			continue // bare/aliased import — external, not resolved (see package doc)
		}

		joined := spec
		if fileDir != "." {
			joined = fileDir + "/" + spec
		}
		joined = filepath.ToSlash(filepath.Clean(joined))

		target := resolveModule(joined, existing)
		if target == "" {
			unresolved = append(unresolved, cycle.ImportIssue{File: file, Specifier: spec})
			continue
		}
		if target == file || seen[target] {
			continue
		}
		seen[target] = true
		targets = append(targets, target)
	}

	return targets, unresolved, nil
}

// withoutComments masks comments while preserving quoted import specifiers.
// A raw regex over comments would make strict unresolved-import checks fail
// on documentation examples that are not imports.
func withoutComments(src []byte) string {
	out := append([]byte(nil), src...)
	var quote byte
	line, block := false, false
	for i := 0; i < len(src); i++ {
		if line {
			if src[i] == '\n' {
				line = false
			} else {
				out[i] = ' '
			}
			continue
		}
		if block {
			if src[i] == '*' && i+1 < len(src) && src[i+1] == '/' {
				out[i], out[i+1] = ' ', ' '
				i++
				block = false
			} else if src[i] != '\n' {
				out[i] = ' '
			}
			continue
		}
		if quote != 0 {
			if src[i] == '\\' && i+1 < len(src) {
				i++
				continue
			}
			if src[i] == quote {
				quote = 0
			}
			continue
		}
		if src[i] == '\'' || src[i] == '"' || src[i] == '`' {
			quote = src[i]
			continue
		}
		if src[i] == '/' && i+1 < len(src) {
			switch src[i+1] {
			case '/':
				out[i], out[i+1] = ' ', ' '
				i++
				line = true
			case '*':
				out[i], out[i+1] = ' ', ' '
				i++
				block = true
			}
		}
	}
	return string(out)
}

// resolveModule tries a specifier (already joined + cleaned, extension-
// less) as: the exact path with each code extension, then as a directory
// index file with each extension.
func resolveModule(base string, existing map[string]bool) string {
	for _, ext := range codeExtensions {
		if existing[base+ext] {
			return base + ext
		}
	}
	for _, ext := range codeExtensions {
		candidate := base + "/index" + ext
		if existing[candidate] {
			return candidate
		}
	}
	return ""
}
