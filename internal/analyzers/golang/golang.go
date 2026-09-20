// Package golang analyzes Go source natively via go/parser and go/ast — no
// subprocess needed for complexity. Coverage comes from a `go test
// -coverprofile` file, parsed directly (Go's profile format is simple
// enough that depending on golang.org/x/tools/cover isn't worth it).
package golang

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"git.roost-r.com/cadeh/quality-gates/internal/analyzers"
	"git.roost-r.com/cadeh/quality-gates/internal/crap"
)

type Analyzer struct{}

func (Analyzer) Analyze(opts analyzers.Options) ([]crap.Function, error) {
	modRoot, modPath, err := findModule(opts.Dir)
	if err != nil {
		return nil, err
	}

	var blocks []coverBlock
	if opts.CoveragePath != "" {
		blocks, err = parseCoverProfile(opts.CoveragePath)
		if err != nil {
			return nil, fmt.Errorf("parse cover profile: %w", err)
		}
	}

	var out []crap.Function
	err = filepath.WalkDir(opts.Dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// "vendor" and "testdata" mirror the go tool's own special-casing
			// (fixture/dependency code, not code this repo owns); dotdirs
			// (.git, etc.) are never source either.
			if d.Name() == "vendor" || d.Name() == "testdata" || (strings.HasPrefix(d.Name(), ".") && path != opts.Dir) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fns, err := analyzeFile(path, modRoot, modPath, blocks, opts.CoveragePath != "")
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, fns...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func analyzeFile(path, modRoot, modPath string, blocks []coverBlock, haveCoverage bool) ([]crap.Function, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(modRoot, absPath)
	if err != nil {
		return nil, err
	}
	importPath := modPath + "/" + filepath.ToSlash(rel)

	unmeasured := haveCoverage && !fileInProfile(blocks, importPath)

	var out []crap.Function
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		start := fset.Position(fn.Pos()).Line
		end := fset.Position(fn.End()).Line
		total, covered, uncovered := coverageForRange(blocks, importPath, start, end)

		out = append(out, crap.Function{
			File:           rel,
			Name:           funcName(fn),
			StartLine:      start,
			EndLine:        end,
			Complexity:     cyclomaticComplexity(fn),
			LinesTotal:     total,
			LinesCovered:   covered,
			UncoveredLines: uncovered,
			Unmeasured:     unmeasured,
		})
	}
	return out, nil
}

func funcName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recv := fn.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		if ident, ok := star.X.(*ast.Ident); ok {
			return "(*" + ident.Name + ")." + fn.Name.Name
		}
	}
	if ident, ok := recv.(*ast.Ident); ok {
		return "(" + ident.Name + ")." + fn.Name.Name
	}
	return fn.Name.Name
}

// cyclomaticComplexity follows the same well-established counting rule as
// gocyclo: start at 1, add one for every branch point (if/for/range/case/
// select-comm) and one for every short-circuit && / || operand.
func cyclomaticComplexity(fn *ast.FuncDecl) int {
	complexity := 1
	ast.Inspect(fn, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.CaseClause, *ast.CommClause:
			complexity++
		case *ast.BinaryExpr:
			if n.Op == token.LAND || n.Op == token.LOR {
				complexity++
			}
		}
		return true
	})
	return complexity
}

// findModule locates the nearest go.mod at or above dir and returns its
// directory and module path, so source files can be matched against cover
// profile entries (which are keyed by full import path).
func findModule(dir string) (root, modulePath string, err error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	for d := abs; ; {
		modFile := filepath.Join(d, "go.mod")
		if data, err := os.ReadFile(modFile); err == nil {
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

type coverBlock struct {
	file               string
	startLine, endLine int
	numStmt, count     int
}

// parseCoverProfile reads a `go test -coverprofile` file. Format:
//
//	mode: set
//	import/path/file.go:12.3,18.4 3 1
//
// (file:startLine.startCol,endLine.endCol numStatements executionCount)
func parseCoverProfile(path string) ([]coverBlock, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var blocks []coverBlock
	scanner := bufio.NewScanner(f)
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			first = false
			continue // "mode: ..." header
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		b, err := parseCoverLine(line)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", line, err)
		}
		blocks = append(blocks, b)
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return nil, err
	}
	return blocks, nil
}

func parseCoverLine(line string) (coverBlock, error) {
	fields := strings.Fields(line)
	if len(fields) != 3 {
		return coverBlock{}, fmt.Errorf("expected 3 fields, got %d", len(fields))
	}

	colon := strings.LastIndex(fields[0], ":")
	if colon < 0 {
		return coverBlock{}, fmt.Errorf("missing ':' in position field")
	}
	file := fields[0][:colon]
	rng := fields[0][colon+1:]

	parts := strings.SplitN(rng, ",", 2)
	if len(parts) != 2 {
		return coverBlock{}, fmt.Errorf("malformed range %q", rng)
	}
	startLine, err := leadingInt(parts[0])
	if err != nil {
		return coverBlock{}, err
	}
	endLine, err := leadingInt(parts[1])
	if err != nil {
		return coverBlock{}, err
	}

	numStmt, err := strconv.Atoi(fields[1])
	if err != nil {
		return coverBlock{}, err
	}
	count, err := strconv.Atoi(fields[2])
	if err != nil {
		return coverBlock{}, err
	}

	return coverBlock{file: file, startLine: startLine, endLine: endLine, numStmt: numStmt, count: count}, nil
}

// leadingInt parses the line number out of a "line.col" position.
func leadingInt(s string) (int, error) {
	dot := strings.IndexByte(s, '.')
	if dot < 0 {
		return strconv.Atoi(s)
	}
	return strconv.Atoi(s[:dot])
}

// fileInProfile reports whether the cover profile has any block for file.
// A file with functions but no blocks is one no test's package ever built.
func fileInProfile(blocks []coverBlock, file string) bool {
	for _, b := range blocks {
		if b.file == file {
			return true
		}
	}
	return false
}

// coverageForRange sums statement counts (Go's coverage granularity is
// per-statement-block, not per-physical-line) for every profile block that
// falls inside [start, end] of the given file, returning (total, covered,
// uncoveredRanges) — the third so a hotspot points at exactly which lines
// need a test, not just how risky the function is overall.
func coverageForRange(blocks []coverBlock, file string, start, end int) (total, covered int, uncovered []crap.LineRange) {
	for _, b := range blocks {
		if b.file != file {
			continue
		}
		if b.startLine < start || b.endLine > end {
			continue
		}
		total += b.numStmt
		if b.count > 0 {
			covered += b.numStmt
		} else {
			uncovered = append(uncovered, crap.LineRange{Start: b.startLine, End: b.endLine})
		}
	}
	sort.Slice(uncovered, func(i, j int) bool { return uncovered[i].Start < uncovered[j].Start })
	return total, covered, uncovered
}
