// Command escape-metric counts "escape hatches" — suppression comments
// and a few high-confidence patterns where code opts out of type-checking,
// linting, or error handling rather than resolving it — and gates CI on
// the resulting rate per 1000 lines.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"git.roost-r.com/cadeh/quality-gates/internal/escape"
	"git.roost-r.com/cadeh/quality-gates/internal/ratchet"
	"git.roost-r.com/cadeh/quality-gates/internal/safefile"
)

var usage = `usage:
  escape-metric check --lang ` + escape.CanonicalLanguages + ` --dir DIR [--fail-above RATE] [--forbid-pattern NAME]... [--top N] [--require-analysis] [--only-files PATH] [--json PATH]
  escape-metric version`

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
		fmt.Fprintln(stdout, "escape-metric", version)
		return 0
	}
	if args[0] != "check" {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	return runCheck(args[1:], stdout, stderr)
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lang := fs.String("lang", "", "language to scan: "+escape.CanonicalLanguages)
	dir := fs.String("dir", ".", "source directory to scan")
	failAbove := fs.Float64("fail-above", 1, "escape hatches per 1000 lines above which the gate fails")
	var forbiddenPatterns stringListFlag
	fs.Var(&forbiddenPatterns, "forbid-pattern", "pattern name that always fails the gate (repeatable)")
	top := fs.Int("top", 20, "number of hatches to print (0 = all)")
	onlyFilesPath := fs.String("only-files", "", "path to a newline-separated changed-file list (e.g. `git diff --name-only`) — ratchets the gate to hatches in these files, so pre-existing ones elsewhere don't block; omit to check the whole --dir as before")
	requireAnalysis := fs.Bool("require-analysis", false, "fail when eligible source files are not scanned")
	jsonOut := fs.String("json", "escape-report.json", "path to write the full JSON report")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *lang == "" {
		fmt.Fprintln(stderr, "escape-metric: --lang is required")
		return 2
	}
	if _, ok := escape.Resolve(*lang); !ok {
		fmt.Fprintf(stderr, "escape-metric: unknown --lang %q (want %s)\n", *lang, escape.CanonicalLanguages)
		return 2
	}
	if err := validateForbiddenPatterns(*lang, forbiddenPatterns); err != nil {
		fmt.Fprintln(stderr, "escape-metric:", err)
		return 2
	}

	onlyFiles, ok := ratchet.Load(*onlyFilesPath, "escape-metric", stderr)
	if !ok {
		return 2
	}

	result, err := escape.Scan(escape.Options{Dir: *dir, Lang: *lang})
	if err != nil {
		fmt.Fprintln(stderr, "escape-metric: scan:", err)
		return 2
	}

	report, err := writeScanReport(result, reportOptions{
		failAbove: *failAbove, forbidden: forbiddenPatterns, requireAnalysis: *requireAnalysis,
		dir: *dir, lang: *lang, top: *top, jsonPath: *jsonOut, stdout: stdout,
	})
	if err != nil {
		fmt.Fprintln(stderr, "escape-metric: write report:", err)
		return 2
	}

	if onlyFiles == nil {
		return report.ExitCode()
	}

	scopedHatches, scopedLines := filterForRatchet(result, onlyFiles)
	scoped := escape.NewReport(scopedHatches, scopedLines, *failAbove, forbiddenPatterns...)
	if *requireAnalysis {
		scoped, err = withEscapeEvidence(scoped, result, *dir, *lang, onlyFiles)
		if err != nil {
			fmt.Fprintln(stderr, "escape-metric: analysis evidence:", err)
			return 2
		}
		scoped.Analysis.WriteText(stdout)
	}
	fmt.Fprintf(stdout, "\nratchet scope: %d hatch(es) in changed files, %.2f per 1000 changed-file lines\n",
		len(scoped.Hatches), scoped.Rate)
	if scoped.Passed {
		fmt.Fprintf(stdout, "PASS (ratcheted): %.2f per 1000 lines is at or under %.2f\n", scoped.Rate, *failAbove)
	} else {
		fmt.Fprintf(stdout, "FAIL (ratcheted): %.2f per 1000 lines exceeds %.2f\n", scoped.Rate, *failAbove)
	}
	return scoped.ExitCode()
}

func validateForbiddenPatterns(lang string, forbidden []string) error {
	language, _ := escape.Resolve(lang)
	known := make(map[string]bool, len(language.Patterns))
	for _, pattern := range language.Patterns {
		known[pattern.Name] = true
	}
	for _, pattern := range forbidden {
		if !known[pattern] {
			return fmt.Errorf("unknown --forbid-pattern %q for --lang %s", pattern, lang)
		}
	}
	return nil
}

type reportOptions struct {
	failAbove       float64
	forbidden       []string
	requireAnalysis bool
	dir, lang       string
	top             int
	jsonPath        string
	stdout          io.Writer
}

func writeScanReport(result escape.Result, opts reportOptions) (escape.Report, error) {
	// The full report shows every hatch; --only-files narrows only the gate.
	report := escape.NewReport(result.Hatches, result.TotalLines, opts.failAbove, opts.forbidden...)
	if opts.requireAnalysis {
		var err error
		report, err = withEscapeEvidence(report, result, opts.dir, opts.lang, nil)
		if err != nil {
			return escape.Report{}, fmt.Errorf("analysis evidence: %w", err)
		}
	}
	report.WriteTable(opts.stdout, opts.top)
	if report.Analysis != nil {
		report.Analysis.WriteText(opts.stdout)
	}
	if opts.jsonPath != "" {
		if err := writeJSONReport(report, opts.jsonPath); err != nil {
			return escape.Report{}, fmt.Errorf("write report: %w", err)
		}
	}
	return report, nil
}

func writeJSONReport(report escape.Report, path string) error {
	f, err := safefile.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return report.WriteJSON(f)
}

type stringListFlag []string

func (f *stringListFlag) String() string { return strings.Join(*f, ",") }

func (f *stringListFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("pattern name must not be empty")
	}
	*f = append(*f, value)
	return nil
}
