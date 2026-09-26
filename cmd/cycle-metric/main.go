// Command cycle-metric finds import cycles and gates CI on the resulting
// count. Go is deliberately not supported — see internal/cycle's package
// doc for why.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"git.roost-r.com/cadeh/quality-gates/internal/cycle"
	"git.roost-r.com/cadeh/quality-gates/internal/evidence"
	"git.roost-r.com/cadeh/quality-gates/internal/importers"
	"git.roost-r.com/cadeh/quality-gates/internal/importers/python"
	"git.roost-r.com/cadeh/quality-gates/internal/importers/typescript"
	"git.roost-r.com/cadeh/quality-gates/internal/ratchet"
	"git.roost-r.com/cadeh/quality-gates/internal/safefile"
)

const usage = `usage:
  cycle-metric check --lang python|ts --dir DIR [--fail-above N] [--top N] [--require-analysis] [--only-files PATH] [--json PATH]
  cycle-metric version`

// version is overridden at build time via -ldflags "-X main.version=...";
// "dev" for a plain `go build`/`go run`.
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run implements the CLI without touching process state, so it's directly
// testable.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if args[0] == "version" {
		fmt.Fprintln(stdout, "cycle-metric", version)
		return 0
	}
	if args[0] != "check" {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	return runCheck(args[1:], stdout, stderr)
}

type ratchetScope struct {
	dir, lang                string
	failAbove, filesAnalyzed int
	requireAnalysis          bool
	analyzedFiles            []string
	unresolved               []cycle.ImportIssue
	onlyFiles                map[string]bool
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lang := fs.String("lang", "", "language to analyze: python or ts (not go — see internal/cycle's package doc)")
	dir := fs.String("dir", ".", "source directory to analyze")
	failAbove := fs.Int("fail-above", 0, "number of cycles above which the gate fails")
	top := fs.Int("top", 20, "number of cycles to print (0 = all)")
	onlyFilesPath := fs.String("only-files", "", "path to a newline-separated changed-file list (e.g. `git diff --name-only`) — ratchets the gate to cycles touching these files, so pre-existing ones don't block; omit to check the whole --dir as before")
	requireAnalysis := fs.Bool("require-analysis", false, "fail when eligible source files are not in the import graph")
	jsonOut := fs.String("json", "cycle-report.json", "path to write the full JSON report")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	onlyFiles, ok := ratchet.Load(*onlyFilesPath, "cycle-metric", stderr)
	if !ok {
		return 2
	}

	importer, err := importerFor(*lang)
	if err != nil {
		fmt.Fprintln(stderr, "cycle-metric:", err)
		return 2
	}

	graph, filesAnalyzed, err := importer.Import(importers.Options{Dir: *dir})
	if err != nil {
		fmt.Fprintln(stderr, "cycle-metric: import:", err)
		return 2
	}

	// The full report (every cycle found) is always what gets printed
	// and written to --json — --only-files narrows the *gate* only, so
	// nothing is hidden, just not blocking.
	cycles := cycle.FindCycles(graph)
	report := cycle.NewReport(filesAnalyzed, cycles, *failAbove)
	report.UnresolvedImports = graph.Unresolved
	var analyzedFiles []string
	for file := range graph.Edges {
		analyzedFiles = append(analyzedFiles, file)
	}
	if *requireAnalysis {
		analysis, err := evidence.CheckAll(*dir, *lang, analyzedFiles, nil)
		if err != nil {
			fmt.Fprintln(stderr, "cycle-metric: analysis evidence:", err)
			return 2
		}
		var unresolved []string
		for _, issue := range graph.Unresolved {
			unresolved = append(unresolved, issue.File+": "+issue.Specifier)
		}
		analysis = analysis.WithUnresolved(unresolved)
		report.Analysis = &analysis
		report.Passed = report.Passed && analysis.Passed
	}
	report.WriteTable(stdout, *top)
	if len(graph.Unresolved) > 0 {
		fmt.Fprintf(stdout, "%d unresolved relative import(s)\n", len(graph.Unresolved))
	}
	if report.Analysis != nil {
		report.Analysis.WriteText(stdout)
	}

	if *jsonOut != "" {
		if err := writeJSONReport(report, *jsonOut); err != nil {
			fmt.Fprintln(stderr, "cycle-metric: write report:", err)
			return 2
		}
	}

	if onlyFiles == nil {
		return report.ExitCode()
	}
	scope := ratchetScope{dir: *dir, lang: *lang, failAbove: *failAbove, filesAnalyzed: filesAnalyzed,
		requireAnalysis: *requireAnalysis, analyzedFiles: analyzedFiles,
		unresolved: graph.Unresolved, onlyFiles: onlyFiles}
	return cycleRatchet(scope, cycles, stdout, stderr)
}

func cycleRatchet(scope ratchetScope, cycles []cycle.Cycle, stdout, stderr io.Writer) int {
	scopedCycles := filterForRatchet(cycles, scope.onlyFiles)
	scoped := cycle.NewReport(scope.filesAnalyzed, scopedCycles, scope.failAbove)
	if scope.requireAnalysis {
		analysis, err := evidence.CheckAll(scope.dir, scope.lang, scope.analyzedFiles, scope.onlyFiles)
		if err != nil {
			fmt.Fprintln(stderr, "cycle-metric: analysis evidence:", err)
			return 2
		}
		var unresolvedNames []string
		for _, issue := range scope.unresolved {
			if evidence.Changed(scope.dir, issue.File, scope.onlyFiles) {
				unresolvedNames = append(unresolvedNames, issue.File+": "+issue.Specifier)
			}
		}
		analysis = analysis.WithUnresolved(unresolvedNames)
		scoped.Analysis = &analysis
		scoped.Passed = scoped.Passed && analysis.Passed
		analysis.WriteText(stdout)
	}
	fmt.Fprintf(stdout, "\nratchet scope: %d cycle(s) touching changed files\n", len(scoped.Cycles))
	if scoped.Passed {
		fmt.Fprintf(stdout, "PASS (ratcheted): %d cycle(s) is at or under %d\n", len(scoped.Cycles), scope.failAbove)
	} else {
		fmt.Fprintf(stdout, "FAIL (ratcheted): %d cycle(s) exceeds %d\n", len(scoped.Cycles), scope.failAbove)
	}
	return scoped.ExitCode()
}

func writeJSONReport(report cycle.Report, path string) error {
	f, err := safefile.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return report.WriteJSON(f)
}

func importerFor(lang string) (importers.Importer, error) {
	switch lang {
	case "python", "py":
		return python.Importer{}, nil
	case "ts", "typescript", "js", "javascript":
		return typescript.Importer{}, nil
	case "go", "golang":
		return nil, fmt.Errorf("--lang go isn't supported: the Go compiler already refuses to build a package-import cycle, so this check would always report zero — see README")
	case "swift":
		return nil, fmt.Errorf("--lang swift isn't supported: Swift files within one module never import each other (no per-file import graph exists to detect a cycle in), and cross-module detection would need Package.swift-level resolution this tool doesn't implement — see README")
	case "":
		return nil, fmt.Errorf("--lang is required")
	default:
		return nil, fmt.Errorf("unknown --lang %q (want python or ts)", lang)
	}
}
