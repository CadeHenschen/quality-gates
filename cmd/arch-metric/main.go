// Command arch-metric gates CI on declared layer rules: a small config
// file lists boundaries like "domain may not import infra" or "cmd/* may
// not import each other", checked against the real import graph. Go's
// compiler stops import cycles but has no opinion on layering — this
// catches something nothing else in this module does.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"git.roost-r.com/cadeh/quality-gates/internal/arch"
	"git.roost-r.com/cadeh/quality-gates/internal/cycle"
	"git.roost-r.com/cadeh/quality-gates/internal/evidence"
	"git.roost-r.com/cadeh/quality-gates/internal/importers"
	"git.roost-r.com/cadeh/quality-gates/internal/importers/golang"
	"git.roost-r.com/cadeh/quality-gates/internal/importers/python"
	"git.roost-r.com/cadeh/quality-gates/internal/importers/typescript"
	"git.roost-r.com/cadeh/quality-gates/internal/ratchet"
)

const usage = `usage:
  arch-metric check     --lang python|ts|go --dir DIR [--rules PATH] [--fail-above N] [--top N] [--require-analysis] [--only-files PATH] [--json PATH]
  arch-metric stability --lang python|ts|go --dir DIR [--fail-above N] [--top N] [--require-analysis] [--only-files PATH | --baseline PATH] [--json PATH]
  arch-metric diff      --old PATH --new PATH [--top N] [--json]
  arch-metric version`

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
	switch args[0] {
	case "version":
		fmt.Fprintln(stdout, "arch-metric", version)
		return 0
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "stability":
		return runStability(args[1:], stdout, stderr)
	case "diff":
		return runDiff(args[1:], stdout, stderr)
	default:
		fmt.Fprintln(stderr, usage)
		return 2
	}
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
	lang := fs.String("lang", "", "language to analyze: python, ts, or go")
	dir := fs.String("dir", ".", "source directory to analyze")
	rulesPath := fs.String("rules", "", fmt.Sprintf("path to the layer-rules JSON file (default: <dir>/%s)", arch.DefaultRulesFile))
	failAbove := fs.Int("fail-above", 0, "number of violations above which the gate fails")
	top := fs.Int("top", 20, "number of violations to print (0 = all)")
	onlyFilesPath := fs.String("only-files", "", "path to a newline-separated changed-file list (e.g. `git diff --name-only`) — ratchets the gate to violations touching these files, so pre-existing ones don't block; omit to check the whole --dir as before")
	requireAnalysis := fs.Bool("require-analysis", false, "fail when eligible source files are not in the import graph")
	jsonOut := fs.String("json", "arch-report.json", "path to write the full JSON report")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	onlyFiles, ok := ratchet.Load(*onlyFilesPath, "arch-metric", stderr)
	if !ok {
		return 2
	}

	importer, err := importerFor(*lang)
	if err != nil {
		fmt.Fprintln(stderr, "arch-metric:", err)
		return 2
	}

	resolvedRulesPath := resolveRulesPath(*rulesPath, *dir)
	ruleSet, err := loadRuleSet(resolvedRulesPath)
	if err != nil {
		fmt.Fprintln(stderr, "arch-metric:", err)
		return 2
	}

	graph, filesAnalyzed, err := importer.Import(importers.Options{Dir: *dir})
	if err != nil {
		fmt.Fprintln(stderr, "arch-metric: import:", err)
		return 2
	}

	// The full report (every violation found) is always what gets printed
	// and written to --json — --only-files narrows the *gate* only, so
	// nothing is hidden, just not blocking.
	edges := arch.EdgesFromFileGraph(graph)
	violations := ruleSet.Check(edges)
	report := arch.NewReport(filesAnalyzed, violations, *failAbove)
	report.UnresolvedImports = graph.Unresolved
	analyzedFiles := analyzedFileNames(graph.Edges)
	if *requireAnalysis {
		analysis, err := evidence.CheckAll(*dir, *lang, analyzedFiles, nil)
		if err != nil {
			fmt.Fprintln(stderr, "arch-metric: analysis evidence:", err)
			return 2
		}
		analysis = analysis.WithUnresolved(unresolvedFor(*dir, graph.Unresolved, nil))
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
			fmt.Fprintln(stderr, "arch-metric: write report:", err)
			return 2
		}
	}

	if onlyFiles == nil {
		return report.ExitCode()
	}
	scope := ratchetScope{dir: *dir, lang: *lang, failAbove: *failAbove, filesAnalyzed: filesAnalyzed,
		requireAnalysis: *requireAnalysis, analyzedFiles: analyzedFiles,
		unresolved: graph.Unresolved, onlyFiles: onlyFiles}
	return checkRatchet(scope, resolvedRulesPath, violations, report, stdout, stderr)
}

func checkRatchet(scope ratchetScope, rulesPath string, violations []arch.Violation,
	report arch.Report, stdout, stderr io.Writer) int {
	// Rules and exceptions change the meaning of every edge, not merely
	// the edges in a source file named in --only-files. A ratchet must not
	// let a stricter policy file pass simply because no violating source
	// file changed in the same commit.
	if ruleFileChanged(rulesPath, scope.dir, scope.onlyFiles) {
		fmt.Fprintln(stdout, "\nratchet scope: layer policy changed; evaluating every violation")
		return report.ExitCode()
	}

	scopedViolations := filterForRatchet(violations, scope.onlyFiles,
		func(v arch.Violation) string { return v.File },
		func(v arch.Violation) string { return v.Import },
	)
	scoped := arch.NewReport(scope.filesAnalyzed, scopedViolations, scope.failAbove)
	if scope.requireAnalysis {
		analysis, err := evidence.CheckAll(scope.dir, scope.lang, scope.analyzedFiles, scope.onlyFiles)
		if err != nil {
			fmt.Fprintln(stderr, "arch-metric: analysis evidence:", err)
			return 2
		}
		analysis = analysis.WithUnresolved(unresolvedFor(scope.dir, scope.unresolved, scope.onlyFiles))
		scoped.Analysis = &analysis
		scoped.Passed = scoped.Passed && analysis.Passed
		analysis.WriteText(stdout)
	}
	fmt.Fprintf(stdout, "\nratchet scope: %d violation(s) touching changed files\n", len(scoped.Violations))
	if scoped.Passed {
		fmt.Fprintf(stdout, "PASS (ratcheted): %d violation(s) is at or under %d\n", len(scoped.Violations), scope.failAbove)
	} else {
		fmt.Fprintf(stdout, "FAIL (ratcheted): %d violation(s) exceeds %d\n", len(scoped.Violations), scope.failAbove)
	}
	return scoped.ExitCode()
}

func runStability(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("stability", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lang := fs.String("lang", "", "language to analyze: python, ts, or go")
	dir := fs.String("dir", ".", "source directory to analyze")
	failAbove := fs.Int("fail-above", 0, "number of stable-dependency violations above which the gate fails")
	top := fs.Int("top", 20, "number of violations to print (0 = all)")
	onlyFilesPath := fs.String("only-files", "", "path to a newline-separated changed-file list (e.g. `git diff --name-only`) — ratchets the gate to violations touching these files, so pre-existing ones don't block; omit to check the whole --dir as before")
	requireAnalysis := fs.Bool("require-analysis", false, "fail when eligible source files are not in the import graph")
	baselinePath := fs.String("baseline", "", "previous arch-stability JSON report; gates only violations newly introduced since it (cannot be combined with --only-files)")
	jsonOut := fs.String("json", "arch-stability-report.json", "path to write the full JSON report")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *baselinePath != "" && *onlyFilesPath != "" {
		fmt.Fprintln(stderr, "arch-metric: --baseline and --only-files cannot be combined")
		return 2
	}

	onlyFiles, ok := ratchet.Load(*onlyFilesPath, "arch-metric", stderr)
	if !ok {
		return 2
	}

	importer, err := importerFor(*lang)
	if err != nil {
		fmt.Fprintln(stderr, "arch-metric:", err)
		return 2
	}

	graph, filesAnalyzed, err := importer.Import(importers.Options{Dir: *dir})
	if err != nil {
		fmt.Fprintln(stderr, "arch-metric: import:", err)
		return 2
	}

	edges := arch.EdgesFromFileGraph(graph)
	violations := arch.CheckStability(edges)
	report := arch.NewStabilityReport(filesAnalyzed, violations, *failAbove)
	report.UnresolvedImports = graph.Unresolved
	analyzedFiles := analyzedFileNames(graph.Edges)
	if *requireAnalysis {
		analysis, err := evidence.CheckAll(*dir, *lang, analyzedFiles, nil)
		if err != nil {
			fmt.Fprintln(stderr, "arch-metric: analysis evidence:", err)
			return 2
		}
		analysis = analysis.WithUnresolved(unresolvedFor(*dir, graph.Unresolved, nil))
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
			fmt.Fprintln(stderr, "arch-metric: write report:", err)
			return 2
		}
	}

	if onlyFiles == nil {
		return stabilityBaseline(*baselinePath, *failAbove, filesAnalyzed, report, stdout, stderr)
	}
	scope := ratchetScope{dir: *dir, lang: *lang, failAbove: *failAbove, filesAnalyzed: filesAnalyzed,
		requireAnalysis: *requireAnalysis, analyzedFiles: analyzedFiles,
		unresolved: graph.Unresolved, onlyFiles: onlyFiles}
	return stabilityRatchet(scope, violations, stdout, stderr)
}

func stabilityBaseline(path string, failAbove, filesAnalyzed int, report arch.StabilityReport, stdout, stderr io.Writer) int {
	if path == "" {
		return report.ExitCode()
	}
	baseline, err := readStabilityReportFile(path)
	if err != nil {
		fmt.Fprintln(stderr, "arch-metric: reading --baseline:", err)
		return 2
	}
	regressions := arch.StabilityRegressions(baseline, report)
	gated := arch.NewStabilityReport(filesAnalyzed, regressions, failAbove)
	if report.Analysis != nil {
		gated.Passed = gated.Passed && report.Analysis.Passed
	}
	fmt.Fprintf(stdout, "\nbaseline scope: %d new stability violation(s)\n", len(gated.Violations))
	if gated.Passed {
		fmt.Fprintf(stdout, "PASS (baseline): %d new violation(s) is at or under %d\n", len(gated.Violations), failAbove)
	} else {
		fmt.Fprintf(stdout, "FAIL (baseline): %d new violation(s) exceeds %d\n", len(gated.Violations), failAbove)
	}
	return gated.ExitCode()
}

func stabilityRatchet(scope ratchetScope, violations []arch.StabilityViolation, stdout, stderr io.Writer) int {
	scopedViolations := filterForRatchet(violations, scope.onlyFiles,
		func(v arch.StabilityViolation) string { return v.File },
		func(v arch.StabilityViolation) string { return v.Import },
	)
	scoped := arch.NewStabilityReport(scope.filesAnalyzed, scopedViolations, scope.failAbove)
	if scope.requireAnalysis {
		analysis, err := evidence.CheckAll(scope.dir, scope.lang, scope.analyzedFiles, scope.onlyFiles)
		if err != nil {
			fmt.Fprintln(stderr, "arch-metric: analysis evidence:", err)
			return 2
		}
		analysis = analysis.WithUnresolved(unresolvedFor(scope.dir, scope.unresolved, scope.onlyFiles))
		scoped.Analysis = &analysis
		scoped.Passed = scoped.Passed && analysis.Passed
		analysis.WriteText(stdout)
	}
	fmt.Fprintf(stdout, "\nratchet scope: %d violation(s) touching changed files\n", len(scoped.Violations))
	if scoped.Passed {
		fmt.Fprintf(stdout, "PASS (ratcheted): %d violation(s) is at or under %d\n", len(scoped.Violations), scope.failAbove)
	} else {
		fmt.Fprintf(stdout, "FAIL (ratcheted): %d violation(s) exceeds %d\n", len(scoped.Violations), scope.failAbove)
	}
	return scoped.ExitCode()
}

func analyzedFileNames(edges map[string][]string) []string {
	var files []string
	for file := range edges {
		files = append(files, file)
	}
	return files
}

// jsonReport is implemented by both arch.Report and arch.StabilityReport —
// the two gates' report shapes differ, but writing either to a --json
// path is identical.
type jsonReport interface {
	WriteJSON(w io.Writer) error
}

func writeJSONReport(report jsonReport, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return report.WriteJSON(f)
}

// runDiff compares two --json reports from previous `check` runs and
// prints every violation that's new or fixed between them — informational
// only, always exits 0, mirroring crap-metric's own diff subcommand.
func runDiff(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	oldPath := fs.String("old", "", "path to the baseline JSON report")
	newPath := fs.String("new", "", "path to the JSON report to compare against the baseline")
	top := fs.Int("top", 20, "number of changed-violation rows to print (0 = all)")
	asJSON := fs.Bool("json", false, "print the diff entries as JSON instead of a table")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *oldPath == "" || *newPath == "" {
		fmt.Fprintln(stderr, "arch-metric: diff requires both --old and --new")
		return 2
	}

	old, err := readReportFile(*oldPath)
	if err != nil {
		fmt.Fprintln(stderr, "arch-metric: reading --old:", err)
		return 2
	}
	newReport, err := readReportFile(*newPath)
	if err != nil {
		fmt.Fprintln(stderr, "arch-metric: reading --new:", err)
		return 2
	}

	entries := arch.Diff(old, newReport)
	if *asJSON {
		if err := arch.WriteDiffJSON(stdout, entries); err != nil {
			fmt.Fprintln(stderr, "arch-metric: write diff:", err)
			return 2
		}
		return 0
	}
	arch.WriteDiffTable(stdout, entries, *top)
	return 0
}

func readReportFile(path string) (arch.Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return arch.Report{}, err
	}
	defer f.Close()
	return arch.ReadReport(f)
}

func readStabilityReportFile(path string) (arch.StabilityReport, error) {
	f, err := os.Open(path)
	if err != nil {
		return arch.StabilityReport{}, err
	}
	defer f.Close()
	return arch.ReadStabilityReport(f)
}

func resolveRulesPath(rulesPath, dir string) string {
	if rulesPath == "" {
		return filepath.Join(dir, arch.DefaultRulesFile)
	}
	return rulesPath
}

func loadRuleSet(rulesPath string) (arch.RuleSet, error) {
	rules, exceptions, err := arch.LoadRules(rulesPath)
	if err != nil {
		return arch.RuleSet{}, fmt.Errorf("loading rules: %w", err)
	}
	return arch.Compile(rules, exceptions)
}

// ruleFileChanged recognizes both the --dir-relative spelling the default
// rules path uses and an explicit, cwd-relative --rules path. Either match is
// enough to take the safe full-policy verdict.
func ruleFileChanged(rulesPath, dir string, onlyFiles map[string]bool) bool {
	rel, err := filepath.Rel(dir, rulesPath)
	if err == nil && ratchet.Matches(rel, onlyFiles) {
		return true
	}
	return !filepath.IsAbs(rulesPath) && ratchet.Matches(rulesPath, onlyFiles)
}

func importerFor(lang string) (importers.Importer, error) {
	switch lang {
	case "python", "py":
		return python.Importer{}, nil
	case "ts", "typescript", "js", "javascript":
		return typescript.Importer{}, nil
	case "go", "golang":
		return golang.Importer{}, nil
	case "swift":
		return nil, fmt.Errorf("--lang swift isn't supported: Swift files within one module never import each other (no per-file import graph exists the way Python/TS/Go have one), and cross-module resolution would need Package.swift-level target-to-directory mapping this tool doesn't implement — see README")
	case "":
		return nil, fmt.Errorf("--lang is required")
	default:
		return nil, fmt.Errorf("unknown --lang %q (want python, ts, or go)", lang)
	}
}
