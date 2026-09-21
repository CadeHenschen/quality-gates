// Command dead-metric gates on dead code — functions, exports and files
// that nothing reaches. It doesn't detect anything itself: it ingests the
// report of `deadcode -json` (Go), `knip --reporter json` (TS/JS) or
// `vulture` (Python), which the target repo's own CI runs, and gates on the count.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"git.roost-r.com/cadeh/quality-gates/internal/deadcode"
	"git.roost-r.com/cadeh/quality-gates/internal/ratchet"
)

const usage = `usage:
  dead-metric --report PATH [--format deadcode|knip|vulture] [--dir DIR] [--ignore FILE] [--fail-above N] [--top N] [--only-files PATH] [--json PATH]
  dead-metric version`

// version is overridden at build time via -ldflags "-X main.version=...";
// "dev" for a plain `go build`/`go run`.
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run implements the CLI without touching process state, so it's directly
// testable.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "version" {
		fmt.Fprintln(stdout, "dead-metric", version)
		return 0
	}
	if len(args) == 0 || args[0] == "" || args[0][0] != '-' {
		fmt.Fprintln(stderr, usage)
		return 2
	}

	fs := flag.NewFlagSet("dead-metric", flag.ContinueOnError)
	fs.SetOutput(stderr)
	reportPath := fs.String("report", "", "dead-code report to gate on: `deadcode -json` (Go), `knip --reporter json` (TS/JS), or `vulture` output (Python)")
	format := fs.String("format", "", "report format: deadcode, knip, or vulture (default: auto-detect; needed to accept an empty vulture report, since vulture prints nothing when clean)")
	dir := fs.String("dir", "", "relativize absolute paths in the report to this directory")
	ignorePath := fs.String("ignore", "", "allowlist file: symbol names, globs and dir/ prefixes for code that is live in fact (reflection, plugins, public API)")
	failAbove := fs.Int("fail-above", 0, "number of dead symbols above which the gate fails")
	top := fs.Int("top", 20, "number of findings to print (0 = all)")
	onlyFilesPath := fs.String("only-files", "", "path to a newline-separated changed-file list (e.g. `git diff --name-only`) — ratchets the gate to findings in these files, so pre-existing ones elsewhere don't block; omit to gate the whole report")
	jsonOut := fs.String("json", "dead-report.json", "path to write the full JSON report")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *reportPath == "" {
		fmt.Fprintln(stderr, "dead-metric: --report is required")
		return 2
	}
	onlyFiles, ok := ratchet.Load(*onlyFilesPath, "dead-metric", stderr)
	if !ok {
		return 2
	}
	ignore, err := loadIgnore(*ignorePath)
	if err != nil {
		fmt.Fprintln(stderr, "dead-metric: --ignore:", err)
		return 2
	}
	data, err := os.ReadFile(*reportPath)
	if err != nil {
		fmt.Fprintln(stderr, "dead-metric: reading --report:", err)
		return 2
	}
	found, err := deadcode.Parse(data, deadcode.Options{Dir: *dir, Format: *format})
	if err != nil {
		fmt.Fprintln(stderr, "dead-metric:", err)
		return 2
	}
	found = ignore.Filter(found)

	report := deadcode.NewReport(found, *failAbove)
	report.WriteTable(stdout, *top)

	if *jsonOut != "" {
		if err := writeJSON(report.WriteJSON, *jsonOut); err != nil {
			fmt.Fprintln(stderr, "dead-metric: write report:", err)
			return 2
		}
	}
	if onlyFiles == nil {
		return report.ExitCode()
	}

	// The full report above is what's printed and written; --only-files
	// narrows the gate only, by re-tallying just the touched files.
	scoped := deadcode.NewReport(filterFindings(found, onlyFiles), *failAbove)
	fmt.Fprintf(stdout, "\nratchet scope: %d dead symbol(s) in changed files\n", scoped.Count)
	verdict := "PASS"
	if !scoped.Passed {
		verdict = "FAIL"
	}
	fmt.Fprintf(stdout, "%s (ratcheted): %d dead symbol(s), gate is %d\n", verdict, scoped.Count, *failAbove)
	return scoped.ExitCode()
}

func loadIgnore(path string) (deadcode.Ignore, error) {
	if path == "" {
		return deadcode.Ignore{}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return deadcode.Ignore{}, err
	}
	defer f.Close()
	return deadcode.ParseIgnore(f)
}

// filterFindings narrows findings to those in files matching onlyFiles.
func filterFindings(found []deadcode.Finding, onlyFiles map[string]bool) []deadcode.Finding {
	var out []deadcode.Finding
	for _, f := range found {
		if ratchet.Matches(f.File, onlyFiles) {
			out = append(out, f)
		}
	}
	return out
}

func writeJSON(write func(io.Writer) error, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return write(f)
}
