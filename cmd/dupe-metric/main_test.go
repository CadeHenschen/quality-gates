package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/tokenizers/golang"
	"git.roost-r.com/cadeh/quality-gates/internal/tokenizers/python"
	"git.roost-r.com/cadeh/quality-gates/internal/tokenizers/typescript"
)

func TestTokenizerFor(t *testing.T) {
	cases := map[string]string{
		"python": "python.Tokenizer", "py": "python.Tokenizer",
		"go": "golang.Tokenizer", "golang": "golang.Tokenizer",
		"ts": "typescript.Tokenizer", "typescript": "typescript.Tokenizer",
		"js": "typescript.Tokenizer", "javascript": "typescript.Tokenizer",
	}
	for lang, want := range cases {
		tk, err := tokenizerFor(lang)
		if err != nil {
			t.Errorf("tokenizerFor(%q): %v", lang, err)
			continue
		}
		var got string
		switch tk.(type) {
		case python.Tokenizer:
			got = "python.Tokenizer"
		case golang.Tokenizer:
			got = "golang.Tokenizer"
		case typescript.Tokenizer:
			got = "typescript.Tokenizer"
		}
		if got != want {
			t.Errorf("tokenizerFor(%q) = %T, want %s", lang, tk, want)
		}
	}

	if _, err := tokenizerFor(""); err == nil {
		t.Error("tokenizerFor(\"\") should error (missing --lang)")
	}
	if _, err := tokenizerFor("cobol"); err == nil {
		t.Error("tokenizerFor(\"cobol\") should error (unknown language)")
	}
}

func TestRunUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	if code != 2 {
		t.Errorf("run(nil) exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("expected usage message, got %q", stderr.String())
	}
}

func TestRunMissingLang(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"check", "--dir", "."}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "--lang is required") {
		t.Errorf("expected a --lang error, got %q", stderr.String())
	}
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

func TestRunAgainstGoTestdataPassAndFail(t *testing.T) {
	jsonPath := filepath.Join(t.TempDir(), "dupe-report.json")

	// High --fail-above: the fixture's known ~duplication should pass.
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/dupe/golang",
		"--fail-above", "1000", "--json", jsonPath,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("reading --json output: %v", err)
	}
	var decoded struct {
		Clones []struct{ FileA string } `json:"clones"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decoding --json output: %v", err)
	}
	if len(decoded.Clones) == 0 {
		t.Error("expected at least one clone in the JSON report (the fixture has a deliberate duplicate)")
	}

	// --fail-above 0: the fixture's known duplication fails the gate.
	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/dupe/golang",
		"--fail-above", "0", "--json", jsonPath,
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "FAIL") {
		t.Errorf("expected FAIL in output, got:\n%s", stdout.String())
	}
}

func TestRunMinTokensFlag(t *testing.T) {
	// With min-tokens set higher than the fixture's actual duplicate size,
	// nothing should be found.
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/dupe/golang",
		"--min-tokens", "10000", "--fail-above", "0",
		"--json", filepath.Join(t.TempDir(), "dupe-report.json"),
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (min-tokens too high to find anything, stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "PASS: no duplication found") {
		t.Errorf("expected no duplication found, got:\n%s", stdout.String())
	}
}

func TestRunOnlyFilesRatchet(t *testing.T) {
	dir := t.TempDir()

	// testdata/dupe/golang has a deliberate duplicate, so --fail-above 0 fails
	// unscoped. An --only-files list that doesn't mention it should still
	// let the *ratcheted* verdict pass, while the full report keeps
	// showing its own unscoped FAIL.
	unrelated := filepath.Join(dir, "unrelated.txt")
	if err := os.WriteFile(unrelated, []byte("some/other/file.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/dupe/golang",
		"--fail-above", "0", "--only-files", unrelated,
		"--json", filepath.Join(dir, "report.json"),
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (untouched files shouldn't be gated, stderr: %s)", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "FAIL: ") {
		t.Errorf("expected the full, unscoped report to still show its own FAIL, got:\n%s", out)
	}
	if !strings.Contains(out, "ratchet scope: 0 duplicate block(s)") {
		t.Errorf("expected 'ratchet scope: 0 duplicate block(s)', got:\n%s", out)
	}
	if !strings.Contains(out, "PASS (ratcheted)") {
		t.Errorf("expected 'PASS (ratcheted)', got:\n%s", out)
	}

	data, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatalf("reading --json output: %v", err)
	}
	var decoded struct {
		Clones []struct{ FileA string } `json:"clones"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decoding --json output: %v", err)
	}
	if len(decoded.Clones) == 0 {
		t.Error("expected the JSON report to still contain the clone, unfiltered by --only-files")
	}

	// A file list that *does* include the fixture: the ratcheted gate
	// should also fail.
	touched := filepath.Join(dir, "touched.txt")
	if err := os.WriteFile(touched, []byte("some/repo/root/testdata/dupe/golang/sample.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/dupe/golang",
		"--fail-above", "0", "--only-files", touched,
		"--json", filepath.Join(dir, "report2.json"),
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (touched file should still be gated, stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "FAIL (ratcheted)") {
		t.Errorf("expected 'FAIL (ratcheted)', got:\n%s", stdout.String())
	}
}

func TestRunOnlyFilesMissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/dupe/golang",
		"--only-files", "/nonexistent/changed-files.txt",
	}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (missing --only-files file)", code)
	}
	if !strings.Contains(stderr.String(), "reading --only-files") {
		t.Errorf("expected a 'reading --only-files' error, got %q", stderr.String())
	}
}
