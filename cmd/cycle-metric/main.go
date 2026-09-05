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
	"git.roost-r.com/cadeh/quality-gates/internal/importers"
	"git.roost-r.com/cadeh/quality-gates/internal/importers/python"
	"git.roost-r.com/cadeh/quality-gates/internal/importers/typescript"
	"git.roost-r.com/cadeh/quality-gates/internal/ratchet"
)

const usage = "usage: cycle-metric check --lang python|ts --dir DIR [--fail-above N] [--top N] [--only-files PATH] [--json PATH]"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run implements the CLI without touching process state, so it's directly
// testable.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || args[0] != "check" {
		fmt.Fprintln(stderr, usage)
		return 2
	}

	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lang := fs.String("lang", "", "language to analyze: python or ts (not go — see internal/cycle's package doc)")
	dir := fs.String("dir", ".", "source directory to analyze")
	failAbove := fs.Int("fail-above", 0, "number of cycles above which the gate fails")
	top := fs.Int("top", 20, "number of cycles to print (0 = all)")
	onlyFilesPath := fs.String("only-files", "", "path to a newline-separated changed-file list (e.g. `git diff --name-only`) — ratchets the gate to cycles touching these files, so pre-existing ones don't block; omit to check the whole --dir as before")
	jsonOut := fs.String("json", "cycle-report.json", "path to write the full JSON report")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	var onlyFiles map[string]bool
	if *onlyFilesPath != "" {
		loaded, err := ratchet.LoadFiles(*onlyFilesPath)
		if err != nil {
			fmt.Fprintln(stderr, "cycle-metric: reading --only-files:", err)
			return 2
		}
		onlyFiles = loaded
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
	report.WriteTable(stdout, *top)

	if *jsonOut != "" {
		if err := writeJSONReport(report, *jsonOut); err != nil {
			fmt.Fprintln(stderr, "cycle-metric: write report:", err)
			return 2
		}
	}

	if onlyFiles == nil {
		return report.ExitCode()
	}

	scopedCycles := filterForRatchet(cycles, onlyFiles)
	scoped := cycle.NewReport(filesAnalyzed, scopedCycles, *failAbove)
	fmt.Fprintf(stdout, "\nratchet scope: %d cycle(s) touching changed files\n", len(scoped.Cycles))
	if scoped.Passed {
		fmt.Fprintf(stdout, "PASS (ratcheted): %d cycle(s) is at or under %d\n", len(scoped.Cycles), *failAbove)
	} else {
		fmt.Fprintf(stdout, "FAIL (ratcheted): %d cycle(s) exceeds %d\n", len(scoped.Cycles), *failAbove)
	}
	return scoped.ExitCode()
}

func writeJSONReport(report cycle.Report, path string) error {
	f, err := os.Create(path)
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
	case "":
		return nil, fmt.Errorf("--lang is required")
	default:
		return nil, fmt.Errorf("unknown --lang %q (want python or ts)", lang)
	}
}
