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
	"git.roost-r.com/cadeh/quality-gates/internal/importers"
	"git.roost-r.com/cadeh/quality-gates/internal/importers/golang"
	"git.roost-r.com/cadeh/quality-gates/internal/importers/python"
	"git.roost-r.com/cadeh/quality-gates/internal/importers/typescript"
	"git.roost-r.com/cadeh/quality-gates/internal/ratchet"
)

const usage = `usage:
  arch-metric check     --lang python|ts|go --dir DIR [--rules PATH] [--fail-above N] [--top N] [--only-files PATH] [--json PATH]
  arch-metric stability --lang python|ts|go --dir DIR [--fail-above N] [--top N] [--only-files PATH] [--json PATH]
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

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lang := fs.String("lang", "", "language to analyze: python, ts, or go")
	dir := fs.String("dir", ".", "source directory to analyze")
	rulesPath := fs.String("rules", "", fmt.Sprintf("path to the layer-rules JSON file (default: <dir>/%s)", arch.DefaultRulesFile))
	failAbove := fs.Int("fail-above", 0, "number of violations above which the gate fails")
	top := fs.Int("top", 20, "number of violations to print (0 = all)")
	onlyFilesPath := fs.String("only-files", "", "path to a newline-separated changed-file list (e.g. `git diff --name-only`) — ratchets the gate to violations touching these files, so pre-existing ones don't block; omit to check the whole --dir as before")
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

	ruleSet, err := loadRuleSet(*rulesPath, *dir)
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
	report.WriteTable(stdout, *top)

	if *jsonOut != "" {
		if err := writeJSONReport(report, *jsonOut); err != nil {
			fmt.Fprintln(stderr, "arch-metric: write report:", err)
			return 2
		}
	}

	if onlyFiles == nil {
		return report.ExitCode()
	}

	scopedViolations := filterForRatchet(violations, onlyFiles,
		func(v arch.Violation) string { return v.File },
		func(v arch.Violation) string { return v.Import },
	)
	scoped := arch.NewReport(filesAnalyzed, scopedViolations, *failAbove)
	fmt.Fprintf(stdout, "\nratchet scope: %d violation(s) touching changed files\n", len(scoped.Violations))
	if scoped.Passed {
		fmt.Fprintf(stdout, "PASS (ratcheted): %d violation(s) is at or under %d\n", len(scoped.Violations), *failAbove)
	} else {
		fmt.Fprintf(stdout, "FAIL (ratcheted): %d violation(s) exceeds %d\n", len(scoped.Violations), *failAbove)
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
	jsonOut := fs.String("json", "arch-stability-report.json", "path to write the full JSON report")
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

	graph, filesAnalyzed, err := importer.Import(importers.Options{Dir: *dir})
	if err != nil {
		fmt.Fprintln(stderr, "arch-metric: import:", err)
		return 2
	}

	edges := arch.EdgesFromFileGraph(graph)
	violations := arch.CheckStability(edges)
	report := arch.NewStabilityReport(filesAnalyzed, violations, *failAbove)
	report.WriteTable(stdout, *top)

	if *jsonOut != "" {
		if err := writeJSONReport(report, *jsonOut); err != nil {
			fmt.Fprintln(stderr, "arch-metric: write report:", err)
			return 2
		}
	}

	if onlyFiles == nil {
		return report.ExitCode()
	}

	scopedViolations := filterForRatchet(violations, onlyFiles,
		func(v arch.StabilityViolation) string { return v.File },
		func(v arch.StabilityViolation) string { return v.Import },
	)
	scoped := arch.NewStabilityReport(filesAnalyzed, scopedViolations, *failAbove)
	fmt.Fprintf(stdout, "\nratchet scope: %d violation(s) touching changed files\n", len(scoped.Violations))
	if scoped.Passed {
		fmt.Fprintf(stdout, "PASS (ratcheted): %d violation(s) is at or under %d\n", len(scoped.Violations), *failAbove)
	} else {
		fmt.Fprintf(stdout, "FAIL (ratcheted): %d violation(s) exceeds %d\n", len(scoped.Violations), *failAbove)
	}
	return scoped.ExitCode()
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

func loadRuleSet(rulesPath, dir string) (arch.RuleSet, error) {
	if rulesPath == "" {
		rulesPath = filepath.Join(dir, arch.DefaultRulesFile)
	}
	rules, exceptions, err := arch.LoadRules(rulesPath)
	if err != nil {
		return arch.RuleSet{}, fmt.Errorf("loading rules: %w", err)
	}
	return arch.Compile(rules, exceptions)
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
