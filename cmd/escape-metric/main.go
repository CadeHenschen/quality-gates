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

	"git.roost-r.com/cadeh/quality-gates/internal/escape"
	"git.roost-r.com/cadeh/quality-gates/internal/ratchet"
)

const usage = `usage:
  escape-metric check --lang python|go|ts --dir DIR [--fail-above RATE] [--top N] [--only-files PATH] [--json PATH]
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

	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lang := fs.String("lang", "", "language to scan: python, go, or ts")
	dir := fs.String("dir", ".", "source directory to scan")
	failAbove := fs.Float64("fail-above", 1, "escape hatches per 1000 lines above which the gate fails")
	top := fs.Int("top", 20, "number of hatches to print (0 = all)")
	onlyFilesPath := fs.String("only-files", "", "path to a newline-separated changed-file list (e.g. `git diff --name-only`) — ratchets the gate to hatches in these files, so pre-existing ones elsewhere don't block; omit to check the whole --dir as before")
	jsonOut := fs.String("json", "escape-report.json", "path to write the full JSON report")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	if *lang == "" {
		fmt.Fprintln(stderr, "escape-metric: --lang is required")
		return 2
	}
	if _, ok := escape.Resolve(*lang); !ok {
		fmt.Fprintf(stderr, "escape-metric: unknown --lang %q (want python, go, or ts)\n", *lang)
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

	// The full report (every hatch found, whole --dir's line count) is
	// always what gets printed and written to --json — --only-files
	// narrows the *gate* only, so nothing is hidden, just not blocking.
	report := escape.NewReport(result.Hatches, result.TotalLines, *failAbove)
	report.WriteTable(stdout, *top)

	if *jsonOut != "" {
		if err := writeJSONReport(report, *jsonOut); err != nil {
			fmt.Fprintln(stderr, "escape-metric: write report:", err)
			return 2
		}
	}

	if onlyFiles == nil {
		return report.ExitCode()
	}

	scopedHatches, scopedLines := filterForRatchet(result, onlyFiles)
	scoped := escape.NewReport(scopedHatches, scopedLines, *failAbove)
	fmt.Fprintf(stdout, "\nratchet scope: %d hatch(es) in changed files, %.2f per 1000 changed-file lines\n",
		len(scoped.Hatches), scoped.Rate)
	if scoped.Passed {
		fmt.Fprintf(stdout, "PASS (ratcheted): %.2f per 1000 lines is at or under %.2f\n", scoped.Rate, *failAbove)
	} else {
		fmt.Fprintf(stdout, "FAIL (ratcheted): %.2f per 1000 lines exceeds %.2f\n", scoped.Rate, *failAbove)
	}
	return scoped.ExitCode()
}

func writeJSONReport(report escape.Report, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return report.WriteJSON(f)
}
