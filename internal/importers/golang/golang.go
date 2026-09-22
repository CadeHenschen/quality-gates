// Package golang builds an import graph for Go source via go/parser's
// import-only mode plus this module's own go.mod, resolving each local
// import path back to the scanned files in its package directory.
//
// cycle-metric deliberately excludes Go (the compiler already refuses to
// build an import cycle — see internal/cycle's package doc), but
// arch-metric needs it: layering is a policy the compiler has no opinion
// on at all, so a completely acyclic, perfectly buildable Go program can
// still violate a declared layer boundary. That asymmetry — the same
// language excluded from one tool for the opposite reason it's required
// in another — is the whole reason this importer exists.
//
// Go's import unit is the package (directory), not the file. Every other
// importer in this module (python, typescript) produces file-to-file
// edges because that's what their languages actually import; to keep
// cycle.Graph's contract the same across all three ("every edge points at
// a real scanned file") rather than inventing a directory-shaped node
// just for Go, a resolved import here fans out to every other scanned
// file in its target package directory — a file importing a package
// structurally depends on every file that makes up that package.
package golang

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"git.roost-r.com/cadeh/quality-gates/internal/cycle"
	"git.roost-r.com/cadeh/quality-gates/internal/gomod"
	"git.roost-r.com/cadeh/quality-gates/internal/importers"
)

type Importer struct{}

// Import scans every .go file (test files included — like cycle-metric's
// python/typescript importers, a layer boundary is a structural property
// of the whole import graph, and excluding tests could hide a real
// violation) under opts.Dir and builds the resulting graph.
func (Importer) Import(opts importers.Options) (cycle.Graph, int, error) {
	modRoot, modPath, err := gomod.Find(opts.Dir)
	if err != nil {
		return cycle.Graph{}, 0, err
	}

	absDir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return cycle.Graph{}, 0, err
	}
	dirRelToMod, err := filepath.Rel(modRoot, absDir)
	if err != nil {
		return cycle.Graph{}, 0, err
	}
	dirRelToMod = filepath.ToSlash(dirRelToMod)

	files, err := walkGo(opts.Dir)
	if err != nil {
		return cycle.Graph{}, 0, err
	}

	filesByPkg := map[string][]string{}
	for _, f := range files {
		pkg := filepath.ToSlash(filepath.Dir(f))
		filesByPkg[pkg] = append(filesByPkg[pkg], f)
	}

	// The full Go import path of every package actually scanned, so a
	// file's raw import specifiers can be resolved back to it — anything
	// not in this map is external (stdlib, third-party, or simply outside
	// --dir) and dropped, same as python/typescript's importers do for
	// bare/unresolvable specifiers.
	pkgByImportPath := map[string]string{}
	for pkg := range filesByPkg {
		pkgByImportPath[joinImportPath(modPath, dirRelToMod, pkg)] = pkg
	}

	graph := cycle.Graph{Edges: map[string][]string{}}
	for _, f := range files {
		specs, err := fileImports(filepath.Join(opts.Dir, f))
		if err != nil {
			return cycle.Graph{}, 0, err
		}

		seen := map[string]bool{}
		var targets []string
		for _, spec := range specs {
			pkg, ok := pkgByImportPath[spec]
			if !ok {
				continue
			}
			for _, t := range filesByPkg[pkg] {
				if t == f || seen[t] {
					continue
				}
				seen[t] = true
				targets = append(targets, t)
			}
		}
		sort.Strings(targets)
		graph.Edges[f] = targets
	}

	return graph, len(files), nil
}

func walkGo(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// "vendor" and "testdata" mirror the go tool's own
			// special-casing; dotdirs (.git, etc.) are never source.
			name := d.Name()
			if name == "vendor" || name == "testdata" || (strings.HasPrefix(name, ".") && path != dir) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
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

// joinImportPath builds the full Go import path of pkg (--dir-relative)
// given modPath (the enclosing module's own import path) and
// dirRelToMod (--dir's own path relative to the module root, "." when
// --dir is the module root itself).
func joinImportPath(modPath, dirRelToMod, pkg string) string {
	full := modPath
	if dirRelToMod != "." {
		full += "/" + dirRelToMod
	}
	if pkg != "." {
		full += "/" + pkg
	}
	return full
}

// fileImports extracts every import path string from a Go source file.
// Import-only parsing needs no type-checking or even a full package
// graph, so this can't fail on an otherwise-unbuildable fixture (a
// deliberately import-cycle-free but layer-violating one, say) the way
// building the package would.
func fileImports(path string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, imp := range file.Imports {
		spec, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		out = append(out, spec)
	}
	return out, nil
}
