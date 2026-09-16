package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"version"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("run([\"version\"]) exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "escape-metric") {
		t.Errorf("expected version output to mention escape-metric, got %q", stdout.String())
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
	jsonPath := filepath.Join(t.TempDir(), "escape-report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/escape/golang",
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
		Hatches []struct{ Pattern string } `json:"hatches"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decoding --json output: %v", err)
	}
	if len(decoded.Hatches) == 0 {
		t.Error("expected at least one hatch in the JSON report (the fixture has deliberate ones)")
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/escape/golang",
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

	// testdata/escape/golang/sample.go has deliberate hatches, so --fail-above 0
	// fails unscoped. An --only-files list that doesn't mention it should
	// still let the *ratcheted* verdict pass, while the full report keeps
	// showing its own unscoped FAIL.
	unrelated := filepath.Join(dir, "unrelated.txt")
	if err := os.WriteFile(unrelated, []byte("some/other/file.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/escape/golang",
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
	if !strings.Contains(out, "ratchet scope: 0 hatch(es)") {
		t.Errorf("expected 'ratchet scope: 0 hatch(es)', got:\n%s", out)
	}
	if !strings.Contains(out, "PASS (ratcheted)") {
		t.Errorf("expected 'PASS (ratcheted)', got:\n%s", out)
	}

	data, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatalf("reading --json output: %v", err)
	}
	var decoded struct {
		Hatches []struct{ Pattern string } `json:"hatches"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decoding --json output: %v", err)
	}
	if len(decoded.Hatches) == 0 {
		t.Error("expected the JSON report to still contain every hatch, unfiltered by --only-files")
	}

	// A file list that *does* include the fixture: the ratcheted gate
	// should also fail.
	touched := filepath.Join(dir, "touched.txt")
	if err := os.WriteFile(touched, []byte("some/repo/root/testdata/escape/golang/sample.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/escape/golang",
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
		"check", "--lang", "go", "--dir", "../../testdata/escape/golang",
		"--only-files", "/nonexistent/changed-files.txt",
	}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (missing --only-files file)", code)
	}
	if !strings.Contains(stderr.String(), "reading --only-files") {
		t.Errorf("expected a 'reading --only-files' error, got %q", stderr.String())
	}
}
