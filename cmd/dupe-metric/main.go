// Command dupe-metric finds duplicate code blocks via token-shingling and
// gates CI on the resulting duplication percentage.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"git.roost-r.com/cadeh/quality-gates/internal/dupe"
	"git.roost-r.com/cadeh/quality-gates/internal/ratchet"
	"git.roost-r.com/cadeh/quality-gates/internal/tokenizers"
	"git.roost-r.com/cadeh/quality-gates/internal/tokenizers/golang"
	"git.roost-r.com/cadeh/quality-gates/internal/tokenizers/python"
	swifttokenizer "git.roost-r.com/cadeh/quality-gates/internal/tokenizers/swift"
	"git.roost-r.com/cadeh/quality-gates/internal/tokenizers/typescript"
)

const usage = "usage: dupe-metric check --lang python|go|ts|swift --dir DIR [--min-tokens N] [--fail-above PCT] [--top N] [--only-files PATH] [--json PATH]"

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
	lang := fs.String("lang", "", "language to analyze: python, go, or ts")
	dir := fs.String("dir", ".", "source directory to analyze")
	minTokens := fs.Int("min-tokens", 40, "minimum matching token-block size to report as a duplicate")
	failAbove := fs.Float64("fail-above", 5, "duplication percentage above which the gate fails")
	top := fs.Int("top", 20, "number of duplicate blocks to print (0 = all)")
	onlyFilesPath := fs.String("only-files", "", "path to a newline-separated changed-file list (e.g. `git diff --name-only`) — ratchets the gate to duplicate blocks touching these files, so pre-existing duplication elsewhere doesn't block; omit to check the whole --dir as before")
	jsonOut := fs.String("json", "dupe-report.json", "path to write the full JSON report")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	onlyFiles, ok := ratchet.Load(*onlyFilesPath, "dupe-metric", stderr)
	if !ok {
		return 2
	}

	tokenizer, err := tokenizerFor(*lang)
	if err != nil {
		fmt.Fprintln(stderr, "dupe-metric:", err)
		return 2
	}

	files, err := tokenizer.Tokenize(tokenizers.Options{Dir: *dir})
	if err != nil {
		fmt.Fprintln(stderr, "dupe-metric: tokenize:", err)
		return 2
	}

	// The full report (every file, every clone found) is always what
	// gets printed and written to --json — --only-files narrows the
	// *gate* only, so nothing is hidden, just not blocking.
	clones := dupe.Find(files, *minTokens)
	report := dupe.NewReport(files, clones, *failAbove)
	report.WriteTable(stdout, *top)

	if *jsonOut != "" {
		if err := writeJSONReport(report, *jsonOut); err != nil {
			fmt.Fprintln(stderr, "dupe-metric: write report:", err)
			return 2
		}
	}

	if onlyFiles == nil {
		return report.ExitCode()
	}

	scopedFiles, scopedClones := filterForRatchet(files, clones, onlyFiles)
	scoped := dupe.NewReport(scopedFiles, scopedClones, *failAbove)
	fmt.Fprintf(stdout, "\nratchet scope: %d duplicate block(s) touching changed files, %.2f%% of %d changed-file lines\n",
		len(scoped.Clones), scoped.DuplicationPercent, scoped.TotalLines)
	if scoped.Passed {
		fmt.Fprintf(stdout, "PASS (ratcheted): %.2f%% is at or under %.2f%%\n", scoped.DuplicationPercent, *failAbove)
	} else {
		fmt.Fprintf(stdout, "FAIL (ratcheted): %.2f%% exceeds %.2f%%\n", scoped.DuplicationPercent, *failAbove)
	}
	return scoped.ExitCode()
}

func writeJSONReport(report dupe.Report, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return report.WriteJSON(f)
}

func tokenizerFor(lang string) (tokenizers.Tokenizer, error) {
	switch lang {
	case "python", "py":
		return python.Tokenizer{}, nil
	case "go", "golang":
		return golang.Tokenizer{}, nil
	case "ts", "typescript", "js", "javascript":
		return typescript.Tokenizer{}, nil
	case "swift":
		return swifttokenizer.Tokenizer{}, nil
	case "":
		return nil, fmt.Errorf("--lang is required")
	default:
		return nil, fmt.Errorf("unknown --lang %q (want python, go, ts, or swift)", lang)
	}
}
