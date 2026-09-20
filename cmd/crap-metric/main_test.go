package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/analyzers/golang"
	"git.roost-r.com/cadeh/quality-gates/internal/analyzers/python"
	swiftanalyzer "git.roost-r.com/cadeh/quality-gates/internal/analyzers/swift"
	"git.roost-r.com/cadeh/quality-gates/internal/analyzers/typescript"
	"git.roost-r.com/cadeh/quality-gates/internal/crap"
)

func TestAnalyzerFor(t *testing.T) {
	cases := map[string]string{
		"python":     "python.Analyzer",
		"py":         "python.Analyzer",
		"go":         "golang.Analyzer",
		"golang":     "golang.Analyzer",
		"ts":         "typescript.Analyzer",
		"typescript": "typescript.Analyzer",
		"js":         "typescript.Analyzer",
		"javascript": "typescript.Analyzer",
		"swift":      "swift.Analyzer",
	}
	for lang, want := range cases {
		a, err := analyzerFor(lang)
		if err != nil {
			t.Errorf("analyzerFor(%q): %v", lang, err)
			continue
		}
		var got string
		switch a.(type) {
		case python.Analyzer:
			got = "python.Analyzer"
		case golang.Analyzer:
			got = "golang.Analyzer"
		case typescript.Analyzer:
			got = "typescript.Analyzer"
		case swiftanalyzer.Analyzer:
			got = "swift.Analyzer"
		}
		if got != want {
			t.Errorf("analyzerFor(%q) = %T, want %s", lang, a, want)
		}
	}

	if _, err := analyzerFor(""); err == nil {
		t.Error("analyzerFor(\"\") should error (missing --lang)")
	}
	if _, err := analyzerFor("cobol"); err == nil {
		t.Error("analyzerFor(\"cobol\") should error (unknown language)")
	}
}

func TestRunUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	if code != 2 {
		t.Errorf("run(nil) exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("expected usage message on stderr, got %q", stderr.String())
	}
}

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"version"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("run([\"version\"]) exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "crap-metric") {
		t.Errorf("expected version output to mention crap-metric, got %q", stdout.String())
	}
}

func TestRunMissingLang(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"check", "--dir", "."}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "--lang is required") {
		t.Errorf("expected a --lang error on stderr, got %q", stderr.String())
	}
}

func TestRunAgainstGoTestdataPassAndFail(t *testing.T) {
	jsonPath := filepath.Join(t.TempDir(), "crap-report.json")

	// High --fail-above: the fixture's small functions should all pass.
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/crap/golang",
		"--fail-above", "1000", "--json", jsonPath,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "PASS") {
		t.Errorf("expected PASS in output, got:\n%s", stdout.String())
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("reading --json output: %v", err)
	}
	var decoded struct {
		Functions []struct{ Name string } `json:"functions"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decoding --json output: %v", err)
	}
	if len(decoded.Functions) == 0 {
		t.Error("expected at least one function in the JSON report")
	}

	// --fail-above 0: everything with complexity >= 1 fails the gate.
	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/crap/golang",
		"--fail-above", "0", "--json", jsonPath,
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "FAIL") {
		t.Errorf("expected FAIL in output, got:\n%s", stdout.String())
	}
}

func TestRunOnlyFilesRatchet(t *testing.T) {
	dir := t.TempDir()

	// A file list that doesn't mention the fixture at all: even with
	// --fail-above 0 (everything would otherwise fail unscoped), the
	// *ratcheted* gate should pass because nothing analyzed matches an
	// untouched file — but the full (unfiltered) report should still show
	// every function and its own unscoped FAIL, since --only-files
	// narrows the gate, not what's reported.
	unrelated := filepath.Join(dir, "unrelated.txt")
	if err := os.WriteFile(unrelated, []byte("some/other/file.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/crap/golang",
		"--fail-above", "0", "--only-files", unrelated,
		"--json", filepath.Join(dir, "report.json"),
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (untouched files shouldn't be gated, stderr: %s)", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "FAIL: at least one function exceeds CRAP") {
		t.Errorf("expected the full, unscoped report to still show its own FAIL, got:\n%s", out)
	}
	if !strings.Contains(out, "ratchet scope: 0 function(s)") {
		t.Errorf("expected a 'ratchet scope: 0 function(s)' line, got:\n%s", out)
	}
	if !strings.Contains(out, "PASS (ratcheted)") {
		t.Errorf("expected 'PASS (ratcheted)' once everything is filtered out of scope, got:\n%s", out)
	}

	data, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatalf("reading --json output: %v", err)
	}
	var decoded struct {
		Functions []struct{ Name string } `json:"functions"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decoding --json output: %v", err)
	}
	if len(decoded.Functions) == 0 {
		t.Error("expected the JSON report to still contain every function, unfiltered by --only-files")
	}

	// A file list that *does* include the fixture (as a repo-root-relative
	// path, the way `git diff --name-only` would report it): the
	// ratcheted gate should also fail, since the touched file's functions
	// are back in scope.
	touched := filepath.Join(dir, "touched.txt")
	if err := os.WriteFile(touched, []byte("some/repo/root/testdata/crap/golang/sample.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/crap/golang",
		"--fail-above", "0", "--only-files", touched,
		"--json", filepath.Join(dir, "report2.json"),
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (touched file should still be gated, stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "FAIL (ratcheted)") {
		t.Errorf("expected 'FAIL (ratcheted)' in output, got:\n%s", stdout.String())
	}
}

func TestRunOnlyFilesMissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/crap/golang",
		"--only-files", "/nonexistent/changed-files.txt",
	}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (missing --only-files file)", code)
	}
	if !strings.Contains(stderr.String(), "reading --only-files") {
		t.Errorf("expected a 'reading --only-files' error, got %q", stderr.String())
	}
}

func TestRunCheckVerboseShowsUncoveredLines(t *testing.T) {
	// A synthetic cover profile marking Branchy's `if` body (lines 11-13)
	// as uncovered, keyed by sample.go's real module import path.
	profile := "mode: set\n" +
		"git.roost-r.com/cadeh/quality-gates/testdata/crap/golang/sample.go:11.2,13.3 1 0\n"
	covPath := filepath.Join(t.TempDir(), "cover.out")
	if err := os.WriteFile(covPath, []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/crap/golang",
		"--coverage", covPath, "--fail-above", "1000", "--verbose",
		"--json", filepath.Join(t.TempDir(), "crap-report.json"),
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "uncovered: 11-13") {
		t.Errorf("expected an 'uncovered: 11-13' line for Branchy, got:\n%s", stdout.String())
	}
}

func TestRunDiff(t *testing.T) {
	dir := t.TempDir()
	oldPath := writeReportFixture(t, dir, "old.json", crap.NewReport([]crap.Function{
		{File: "a.go", Name: "stable", Complexity: 1, LinesTotal: 10, LinesCovered: 10},
		{File: "a.go", Name: "getsWorse", Complexity: 2, LinesTotal: 10, LinesCovered: 10},
	}, 30))
	newPath := writeReportFixture(t, dir, "new.json", crap.NewReport([]crap.Function{
		{File: "a.go", Name: "stable", Complexity: 1, LinesTotal: 10, LinesCovered: 10},
		{File: "a.go", Name: "getsWorse", Complexity: 2, LinesTotal: 10, LinesCovered: 0},
		{File: "a.go", Name: "brandNew", Complexity: 3, LinesTotal: 10, LinesCovered: 0},
	}, 30))

	var stdout, stderr bytes.Buffer
	code := run([]string{"diff", "--old", oldPath, "--new", newPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "getsWorse") {
		t.Errorf("expected 'getsWorse' regression in diff output, got:\n%s", out)
	}
	if !strings.Contains(out, "brandNew") {
		t.Errorf("expected 'brandNew' as a new function in diff output, got:\n%s", out)
	}
	if strings.Contains(out, "stable") {
		t.Errorf("unchanged function 'stable' shouldn't appear in diff output, got:\n%s", out)
	}
}

func TestRunDiffJSON(t *testing.T) {
	dir := t.TempDir()
	oldPath := writeReportFixture(t, dir, "old.json", crap.NewReport(nil, 30))
	newPath := writeReportFixture(t, dir, "new.json", crap.NewReport([]crap.Function{
		{File: "a.go", Name: "f", Complexity: 3, LinesTotal: 10, LinesCovered: 0},
	}, 30))

	var stdout, stderr bytes.Buffer
	code := run([]string{"diff", "--old", oldPath, "--new", newPath, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}

	var deltas []crap.Delta
	if err := json.Unmarshal(stdout.Bytes(), &deltas); err != nil {
		t.Fatalf("decode: %v (raw: %s)", err, stdout.String())
	}
	if len(deltas) != 1 || deltas[0].Name != "f" || deltas[0].Change != "new" {
		t.Errorf("deltas = %+v, want one 'new' delta named f", deltas)
	}
}

func TestRunDiffBadPaths(t *testing.T) {
	validPath := writeReportFixture(t, t.TempDir(), "report.json", crap.NewReport(nil, 30))

	var stdout, stderr bytes.Buffer
	code := run([]string{"diff", "--old", "/nonexistent/old.json", "--new", validPath}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (missing --old file)", code)
	}
	if !strings.Contains(stderr.String(), "reading --old") {
		t.Errorf("expected a 'reading --old' error, got %q", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"diff", "--old", validPath, "--new", "/nonexistent/new.json"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (missing --new file)", code)
	}
	if !strings.Contains(stderr.String(), "reading --new") {
		t.Errorf("expected a 'reading --new' error, got %q", stderr.String())
	}
}

func TestRunDiffMissingFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"diff", "--old", "x.json"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (missing --new)", code)
	}
	if !strings.Contains(stderr.String(), "requires both --old and --new") {
		t.Errorf("expected a missing-flag error, got %q", stderr.String())
	}
}

func writeReportFixture(t *testing.T, dir, name string, report crap.Report) string {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := report.WriteJSON(f); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunUnknownLang(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"check", "--lang", "cobol", "--dir", "."}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown --lang") {
		t.Errorf("expected an unknown-language error, got %q", stderr.String())
	}
}

func TestRunExcludeFileDefaultAndExplicit(t *testing.T) {
	// A copy of the fixture whose --dir holds the default exclude file.
	src, err := os.ReadFile("../../testdata/crap/golang/sample.go")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/x\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	args := func(extra ...string) []string {
		return append([]string{"check", "--lang", "go", "--dir", dir, "--fail-above", "0", "--json", filepath.Join(dir, "r.json")}, extra...)
	}

	var stdout, stderr bytes.Buffer
	if code := run(args(), &stdout, &stderr); code != 1 {
		t.Fatalf("no exclude file: exit %d, want 1 (%s)", code, stderr.String())
	}

	if err := os.WriteFile(filepath.Join(dir, ".crap-metric-exclude"), []byte("# fixture\nsample.go # why\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if code := run(args(), &stdout, &stderr); code != 0 {
		t.Fatalf("default exclude file: exit %d, want 0 (%s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "exclude file: ") {
		t.Errorf("expected the exclude file to be announced, got:\n%s", stdout.String())
	}

	// An explicit path that doesn't exist is an error, unlike the default.
	stderr.Reset()
	if code := run(args("--exclude-file", filepath.Join(dir, "missing")), &stdout, &stderr); code != 2 {
		t.Errorf("missing explicit file: exit %d, want 2", code)
	}
}
