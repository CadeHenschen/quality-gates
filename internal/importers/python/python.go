// Package python builds an import graph for Python source via regex
// extraction of import statements — no real parser needed, since Python's
// import syntax is regular enough to match reliably, and the resolution
// step (does this module correspond to a file we actually scanned) is
// what does the real work, not the extraction.
//
// Only imports that resolve to a file within the scanned directory become
// graph edges; anything else (stdlib, third-party, unresolvable dynamic
// imports) is silently dropped — cycles between your own modules are the
// point, not a dependency inventory.
package python

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

var (
	importRe     = regexp.MustCompile(`^\s*import\s+([\w.]+(?:\s*,\s*[\w.]+)*)`)
	fromImportRe = regexp.MustCompile(`^\s*from\s+(\.*)([\w.]*)\s+import\s+(.+)`)
)

func (Importer) Import(opts importers.Options) (graph cycle.Graph, scanned int, retErr error) {
	root, err := safefile.OpenRoot(opts.Dir)
	if err != nil {
		return cycle.Graph{}, 0, err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	files, err := walkPython(opts.Dir)
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

func walkPython(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "__pycache__" || name == "venv" || name == ".venv" || name == "testdata" ||
				(strings.HasPrefix(name, ".") && path != dir) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".py") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	return files, err
}

func resolveWithIssues(root *os.Root, file string, existing map[string]bool) ([]string, []cycle.ImportIssue, error) {
	data, err := safefile.ReadFileAt(root, file)
	if err != nil {
		return nil, nil, err
	}
	return resolveSourceImports(data, file, existing)
}

func resolveSourceImports(data []byte, file string, existing map[string]bool) ([]string, []cycle.ImportIssue, error) {
	fileDir := filepath.ToSlash(filepath.Dir(file)) // "." for a top-level file
	seen := map[string]bool{}
	var targets []string
	var unresolved []cycle.ImportIssue
	add := func(candidate string) {
		candidate = filepath.ToSlash(filepath.Clean(candidate))
		if candidate == file || seen[candidate] {
			return
		}
		if existing[candidate] {
			seen[candidate] = true
			targets = append(targets, candidate)
		}
	}
	tryModule := func(base string) {
		if base == "" {
			return
		}
		add(base + ".py")
		add(base + "/__init__.py")
	}

	for _, line := range strings.Split(string(data), "\n") {
		if m := fromImportRe.FindStringSubmatch(line); m != nil {
			dots, module, names := len(m[1]), m[2], m[3]
			before := len(targets)

			base := fileDir
			for i := 1; i < dots; i++ { // 1 dot = current dir; each extra goes up one more
				base = filepath.ToSlash(filepath.Dir(base))
			}
			moduleBase := joinModule(base, module, dots > 0)

			// "from a.b import c": c might be a submodule of a.b (try
			// that first) or a symbol defined inside a.b itself.
			for _, name := range splitNames(names) {
				tryModule(joinModule(moduleBase, name, true))
			}
			tryModule(moduleBase)
			if dots > 0 && module != "" && len(targets) == before {
				unresolved = append(unresolved, cycle.ImportIssue{File: file, Specifier: strings.Repeat(".", dots) + module})
			}
			continue
		}
		if m := importRe.FindStringSubmatch(line); m != nil {
			for _, mod := range strings.Split(m[1], ",") {
				tryModule(strings.ReplaceAll(strings.TrimSpace(mod), ".", "/"))
			}
		}
	}
	return targets, unresolved, nil
}

// joinModule builds a "/"-joined module path from a base directory (which
// may be "" or "." for the scan root) and a dotted module string
// (converted to "/"). relative controls whether an empty module leaves
// base as-is (relative imports: "from . import x" has no module, base IS
// the target directory) or produces "" (absolute imports have no base to
// fall back to).
func joinModule(base, dotted string, relative bool) string {
	modPath := strings.ReplaceAll(dotted, ".", "/")
	switch {
	case modPath == "" && relative:
		return base
	case modPath == "":
		return ""
	case base == "" || base == ".":
		return modPath
	default:
		return base + "/" + modPath
	}
}

// splitNames parses the comma-separated names after "import" — dropping
// "as alias", trailing parens/backslash continuations, and surrounding
// whitespace. Doesn't handle multi-line parenthesized import lists (a
// documented v1 limitation).
func splitNames(raw string) []string {
	raw = strings.TrimRight(raw, "\\")
	raw = strings.NewReplacer("(", "", ")", "").Replace(raw)
	var names []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if idx := strings.Index(part, " as "); idx >= 0 {
			part = part[:idx]
		}
		part = strings.TrimSpace(part)
		if part != "" && part != "*" {
			names = append(names, part)
		}
	}
	return names
}
