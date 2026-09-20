// Command crap-metric computes CRAP (Change Risk Anti-Patterns) scores —
// complexity² × (1−coverage)³ + complexity — per function, and gates CI on
// a threshold.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"git.roost-r.com/cadeh/quality-gates/internal/analyzers"
	"git.roost-r.com/cadeh/quality-gates/internal/analyzers/golang"
	"git.roost-r.com/cadeh/quality-gates/internal/analyzers/python"
	swiftanalyzer "git.roost-r.com/cadeh/quality-gates/internal/analyzers/swift"
	"git.roost-r.com/cadeh/quality-gates/internal/analyzers/typescript"
	"git.roost-r.com/cadeh/quality-gates/internal/crap"
	"git.roost-r.com/cadeh/quality-gates/internal/exclude"
	"git.roost-r.com/cadeh/quality-gates/internal/ratchet"
)

const usage = `usage:
  crap-metric check --lang python|go|ts|swift --dir DIR [--coverage PATH] [--fail-above N] [--top N] [--verbose] [--only-files PATH] [--exclude GLOB]... [--exclude-file PATH] [--json PATH]
  crap-metric diff --old PATH --new PATH [--top N] [--json]
  crap-metric version`

// version is overridden at build time via -ldflags "-X main.version=...";
// "dev" for a plain `go build`/`go run`.
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run implements the CLI without touching process state (os.Exit/os.Args),
// so it's directly testable.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, usage)
		return 2
	}

	switch args[0] {
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "diff":
		return runDiff(args[1:], stdout, stderr)
	case "version":
		fmt.Fprintln(stdout, "crap-metric", version)
		return 0
	default:
		fmt.Fprintln(stderr, usage)
		return 2
	}
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lang := fs.String("lang", "", "language to analyze: python, go, or ts")
	dir := fs.String("dir", ".", "source directory to analyze")
	coverage := fs.String("coverage", "", "path to that language's coverage report (coverage.json / cover profile / coverage-final.json)")
	failAbove := fs.Float64("fail-above", 30, "CRAP score above which the gate fails")
	top := fs.Int("top", 20, "number of hotspot rows to print (0 = all)")
	verbose := fs.Bool("verbose", false, "show each hotspot's uncovered line ranges")
	onlyFilesPath := fs.String("only-files", "", "path to a newline-separated changed-file list (e.g. `git diff --name-only`) — ratchets the gate to only functions in these files, so pre-existing hotspots elsewhere don't block; omit to check the whole --dir as before")
	var excludes stringList
	fs.Var(&excludes, "exclude", "glob of --dir-relative files to drop from analysis entirely (repeatable; e.g. 'internal/gen/**', '**/*_pb.go'). Unlike --only-files this removes files from the report too")
	excludeFile := fs.String("exclude-file", "", "file of exclude globs, one per line, '#' comments (default: "+exclude.DefaultFile+" in --dir, if present; commit it to keep exclusions reviewable)")
	jsonOut := fs.String("json", "crap-report.json", "path to write the full JSON report")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	onlyFiles, ok := ratchet.Load(*onlyFilesPath, "crap-metric", stderr)
	if !ok {
		return 2
	}

	analyzer, err := analyzerFor(*lang)
	if err != nil {
		fmt.Fprintln(stderr, "crap-metric:", err)
		return 2
	}

	fns, err := analyzer.Analyze(analyzers.Options{Dir: *dir, CoveragePath: *coverage})
	if err != nil {
		fmt.Fprintln(stderr, "crap-metric: analyze:", err)
		return 2
	}

	patterns, err := loadExcludePatterns(*dir, *excludeFile, excludes, stdout)
	if err != nil {
		fmt.Fprintln(stderr, "crap-metric: reading exclude file:", err)
		return 2
	}
	excluded, err := exclude.Compile(patterns)
	if err != nil {
		fmt.Fprintln(stderr, "crap-metric: bad --exclude pattern:", err)
		return 2
	}
	fns, nExcluded := dropExcluded(fns, excluded)
	if nExcluded > 0 {
		fmt.Fprintf(stdout, "excluded: %d function(s) by --exclude\n\n", nExcluded)
	}

	// The full report (every function in --dir) is always what gets
	// printed and written to --json — --only-files narrows the *gate*
	// only, so nothing is hidden, just not blocking.
	report := crap.NewReport(fns, *failAbove)
	report.WriteTable(stdout, *top, *verbose)

	if *jsonOut != "" {
		if err := writeJSONReport(report, *jsonOut); err != nil {
			fmt.Fprintln(stderr, "crap-metric: write report:", err)
			return 2
		}
	}

	if onlyFiles == nil {
		return report.ExitCode()
	}

	scoped := crap.NewReport(filterFunctions(fns, onlyFiles), *failAbove)
	fmt.Fprintf(stdout, "\nratchet scope: %d function(s) in changed files\n", len(scoped.Functions))
	if scoped.Passed {
		fmt.Fprintf(stdout, "PASS (ratcheted): no changed function exceeds CRAP %.1f\n", *failAbove)
	} else {
		fmt.Fprintf(stdout, "FAIL (ratcheted): at least one changed function exceeds CRAP %.1f\n", *failAbove)
	}
	return scoped.ExitCode()
}

func runDiff(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	oldPath := fs.String("old", "", "path to the baseline JSON report")
	newPath := fs.String("new", "", "path to the JSON report to compare against the baseline")
	top := fs.Int("top", 20, "number of changed-function rows to print (0 = all)")
	asJSON := fs.Bool("json", false, "print the deltas as JSON instead of a table")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *oldPath == "" || *newPath == "" {
		fmt.Fprintln(stderr, "crap-metric: diff requires both --old and --new")
		return 2
	}

	old, err := readReportFile(*oldPath)
	if err != nil {
		fmt.Fprintln(stderr, "crap-metric: reading --old:", err)
		return 2
	}
	newReport, err := readReportFile(*newPath)
	if err != nil {
		fmt.Fprintln(stderr, "crap-metric: reading --new:", err)
		return 2
	}

	deltas := crap.Diff(old, newReport)
	if *asJSON {
		if err := crap.WriteDiffJSON(stdout, deltas); err != nil {
			fmt.Fprintln(stderr, "crap-metric: write diff:", err)
			return 2
		}
		return 0
	}
	crap.WriteDiffTable(stdout, deltas, *top)
	return 0
}

func readReportFile(path string) (crap.Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return crap.Report{}, err
	}
	defer f.Close()
	return crap.ReadReport(f)
}

func writeJSONReport(report crap.Report, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return report.WriteJSON(f)
}

func analyzerFor(lang string) (analyzers.Analyzer, error) {
	switch lang {
	case "python", "py":
		return python.Analyzer{}, nil
	case "go", "golang":
		return golang.Analyzer{}, nil
	case "ts", "typescript", "js", "javascript":
		return typescript.Analyzer{}, nil
	case "swift":
		return swiftanalyzer.Analyzer{}, nil
	case "":
		return nil, fmt.Errorf("--lang is required")
	default:
		return nil, fmt.Errorf("unknown --lang %q (want python, go, ts, or swift)", lang)
	}
}
