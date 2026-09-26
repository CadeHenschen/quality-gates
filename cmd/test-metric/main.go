// Command test-metric grades the tests themselves. `check` statically
// finds tests that can't fail or aren't running (no assertions, skipped,
// focused, mock-only assertions, leaked temp dirs); `mutation` gates on the
// mutation score from a mutation-testing tool's report — whether the suite
// actually catches injected bugs.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"git.roost-r.com/cadeh/quality-gates/internal/mutation"
	"git.roost-r.com/cadeh/quality-gates/internal/ratchet"
	"git.roost-r.com/cadeh/quality-gates/internal/safefile"
	"git.roost-r.com/cadeh/quality-gates/internal/testmetric"
	"git.roost-r.com/cadeh/quality-gates/internal/testscanners"
	golangscan "git.roost-r.com/cadeh/quality-gates/internal/testscanners/golang"
	pythonscan "git.roost-r.com/cadeh/quality-gates/internal/testscanners/python"
	swiftscan "git.roost-r.com/cadeh/quality-gates/internal/testscanners/swift"
	typescriptscan "git.roost-r.com/cadeh/quality-gates/internal/testscanners/typescript"
)

const usage = `usage:
  test-metric check    --lang go|python|ts|swift --dir DIR [--fail-above N] [--min-assertions N] [--ignore KIND,...] [--include KIND,...] [--top N] [--require-analysis] [--only-files PATH] [--json PATH]
  test-metric mutation --report PATH [--dir DIR] [--lang go|python|ts|swift] [--fail-below PCT] [--covered-only] [--min-mutants N] [--minimum-graded N] [--top N] [--only-files PATH] [--json PATH]
  test-metric version`

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
		fmt.Fprintln(stdout, "test-metric", version)
		return 0
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "mutation":
		return runMutation(args[1:], stdout, stderr)
	}
	fmt.Fprintln(stderr, usage)
	return 2
}

// scannerFor maps --lang to a test scanner.
func scannerFor(lang string) (testscanners.Scanner, error) {
	switch lang {
	case "go", "golang":
		return golangscan.Scanner{}, nil
	case "python", "py":
		return pythonscan.Scanner{}, nil
	case "ts", "typescript", "js", "javascript":
		return typescriptscan.Scanner{}, nil
	case "swift":
		return swiftscan.Scanner{}, nil
	}
	return nil, fmt.Errorf("unknown --lang %q (want go, python, ts, or swift)", lang)
}

// parseKinds splits a comma-separated list of check names, rejecting any
// that aren't real checks (flag names the offending flag in the error).
func parseKinds(flagName, list string) ([]string, error) {
	var kinds []string
	for _, k := range strings.Split(list, ",") {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if !slices.Contains(testmetric.Kinds, k) {
			return nil, fmt.Errorf("unknown --%s check %q (want one of %s)", flagName, k, strings.Join(testmetric.Kinds, ", "))
		}
		kinds = append(kinds, k)
	}
	return kinds, nil
}

// disabledChecks returns the set of checks to skip: the opt-in ones unless
// named in --include, plus everything named in --ignore (which wins over
// --include).
func disabledChecks(ignoreList, includeList string) (map[string]bool, error) {
	ignored, err := parseKinds("ignore", ignoreList)
	if err != nil {
		return nil, err
	}
	included, err := parseKinds("include", includeList)
	if err != nil {
		return nil, err
	}
	disabled := map[string]bool{}
	for _, k := range testmetric.OptIn {
		disabled[k] = !slices.Contains(included, k)
	}
	for _, k := range ignored {
		disabled[k] = true
	}
	return disabled, nil
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lang := fs.String("lang", "", "language to scan: go, python, ts, or swift")
	dir := fs.String("dir", ".", "source directory to scan")
	failAbove := fs.Int("fail-above", 0, "number of findings above which the gate fails")
	minAssertions := fs.Int("min-assertions", 1, "assertions a test needs; fewer is flagged (0 assertions is always flagged)")
	ignoreList := fs.String("ignore", "", "comma-separated checks to disable: "+strings.Join(testmetric.Kinds, ", "))
	includeList := fs.String("include", "", "comma-separated opt-in checks to enable (off by default: "+strings.Join(testmetric.OptIn, ", ")+")")
	top := fs.Int("top", 20, "number of findings to print (0 = all)")
	onlyFilesPath := fs.String("only-files", "", "path to a newline-separated changed-file list (e.g. `git diff --name-only`) — ratchets the gate to findings in these files, so pre-existing ones elsewhere don't block; omit to check the whole --dir")
	requireAnalysis := fs.Bool("require-analysis", false, "fail when expected test files produce no analyzed tests")
	jsonOut := fs.String("json", "test-report.json", "path to write the full JSON report")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *lang == "" {
		fmt.Fprintln(stderr, "test-metric: --lang is required")
		return 2
	}
	scanner, err := scannerFor(*lang)
	if err != nil {
		fmt.Fprintln(stderr, "test-metric:", err)
		return 2
	}
	ignore, err := disabledChecks(*ignoreList, *includeList)
	if err != nil {
		fmt.Fprintln(stderr, "test-metric:", err)
		return 2
	}
	onlyFiles, ok := ratchet.Load(*onlyFilesPath, "test-metric", stderr)
	if !ok {
		return 2
	}

	tests, err := scanner.Scan(*dir)
	if err != nil {
		fmt.Fprintln(stderr, "test-metric: scan:", err)
		return 2
	}

	opts := testmetric.Options{MinAssertions: *minAssertions, Ignore: ignore}
	numTests, assertions := testmetric.Counts(tests)
	report := testmetric.NewReport(testmetric.Analyze(tests, opts), numTests, assertions, *failAbove)
	if *requireAnalysis {
		report, err = withTestAnalysis(report, *dir, *lang, tests, nil)
		if err != nil {
			fmt.Fprintln(stderr, "test-metric: analysis evidence:", err)
			return 2
		}
	}
	if err := publishCheckReport(report, stdout, *top, *jsonOut); err != nil {
		fmt.Fprintln(stderr, "test-metric: write report:", err)
		return 2
	}
	if onlyFiles == nil {
		return report.ExitCode()
	}

	// The full report above is what's printed and written; --only-files
	// narrows the gate only, by re-analyzing just the touched files' tests.
	touched := filterTests(tests, onlyFiles)
	touchedTests, touchedAsserts := testmetric.Counts(touched)
	scoped := testmetric.NewReport(testmetric.Analyze(touched, opts), touchedTests, touchedAsserts, *failAbove)
	if *requireAnalysis {
		scoped, err = withTestAnalysis(scoped, *dir, *lang, tests, onlyFiles)
		if err != nil {
			fmt.Fprintln(stderr, "test-metric: analysis evidence:", err)
			return 2
		}
		scoped.Analysis.WriteText(stdout)
	}
	writeTestRatchet(stdout, scoped, *failAbove)
	return scoped.ExitCode()
}

func runMutation(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mutation", flag.ContinueOnError)
	fs.SetOutput(stderr)
	reportPath := fs.String("report", "", "mutation report to gate on: Stryker JSON, go-gremlins --output JSON, or the generic {\"mutants\":[...]} format")
	dir := fs.String("dir", "", "relativize absolute paths in the report to this directory")
	lang := fs.String("lang", "", "source language, required with --minimum-graded and --only-files")
	failBelow := fs.Float64("fail-below", 60, "mutation score (percent) below which the gate fails")
	coveredOnly := fs.Bool("covered-only", false, "leave never-executed mutants out of the score, so it measures assertion strength only (line coverage is crap-metric's job)")
	minMutants := fs.Int("min-mutants", 0, "legacy waiver: pass when fewer than this many mutants are graded")
	minimumGraded := fs.Int("minimum-graded", 0, "fail when fewer than this many mutants are graded (0 disables the evidence floor)")
	top := fs.Int("top", 20, "number of surviving mutants to print (0 = all)")
	onlyFilesPath := fs.String("only-files", "", "path to a newline-separated changed-file list — ratchets the gate to mutants in these files; omit to score the whole report")
	jsonOut := fs.String("json", "mutation-report.json", "path to write the full JSON report")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *reportPath == "" {
		fmt.Fprintln(stderr, "test-metric: --report is required")
		return 2
	}
	if *minimumGraded > 0 && *onlyFilesPath != "" && (*lang == "" || *dir == "") {
		fmt.Fprintln(stderr, "test-metric: --lang and --dir are required for ratcheted --minimum-graded")
		return 2
	}
	onlyFiles, ok := ratchet.Load(*onlyFilesPath, "test-metric", stderr)
	if !ok {
		return 2
	}
	data, err := os.ReadFile(*reportPath)
	if err != nil {
		fmt.Fprintln(stderr, "test-metric: reading --report:", err)
		return 2
	}
	mutants, err := mutation.Parse(data, mutation.Options{Dir: *dir})
	if err != nil {
		fmt.Fprintln(stderr, "test-metric:", err)
		return 2
	}

	report := mutation.NewReport(mutants, *failBelow, *coveredOnly).WithMinMutants(*minMutants).WithMinimumGraded(*minimumGraded)
	report.WriteTable(stdout, *top)

	if *jsonOut != "" {
		if err := writeJSON(report.WriteJSON, *jsonOut); err != nil {
			fmt.Fprintln(stderr, "test-metric: write report:", err)
			return 2
		}
	}
	if onlyFiles == nil {
		return report.ExitCode()
	}

	scopedMutants := filterMutants(mutants, onlyFiles)
	scoped, scopeEvidence, err := scopedMutationReport(scopedMutants, mutationScopeOptions{Dir: *dir, Lang: *lang, Changed: onlyFiles, FailBelow: *failBelow, CoveredOnly: *coveredOnly, MinMutants: *minMutants, MinimumGraded: *minimumGraded})
	if err != nil {
		fmt.Fprintln(stderr, "test-metric: analysis evidence:", err)
		return 2
	}
	fmt.Fprintf(stdout, "\nratchet scope: %d mutant(s) in changed files, %.1f%% score\n", len(scoped.Mutants), scoped.Score)
	if scopeEvidence != nil {
		scopeEvidence.WriteText(stdout)
	}
	if scoped.InsufficientEvidence && (scopeEvidence == nil || scopeEvidence.Passed) {
		fmt.Fprintf(stdout, "FAIL: insufficient analysis: %d graded mutants, need at least %d\n", scoped.Graded, scoped.MinimumGraded)
	}
	verdict := "PASS"
	if !scoped.Passed {
		verdict = "FAIL"
	}
	fmt.Fprintf(stdout, "%s (ratcheted): %.1f%% vs gate of %.1f%%\n", verdict, scoped.Score, *failBelow)
	return scoped.ExitCode()
}

func writeJSON(write func(io.Writer) error, path string) error {
	f, err := safefile.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return write(f)
}
