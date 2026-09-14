package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/importers/python"
	"git.roost-r.com/cadeh/quality-gates/internal/importers/typescript"
)

func TestImporterFor(t *testing.T) {
	cases := map[string]string{
		"python": "python.Importer", "py": "python.Importer",
		"ts": "typescript.Importer", "typescript": "typescript.Importer",
		"js": "typescript.Importer", "javascript": "typescript.Importer",
	}
	for lang, want := range cases {
		imp, err := importerFor(lang)
		if err != nil {
			t.Errorf("importerFor(%q): %v", lang, err)
			continue
		}
		var got string
		switch imp.(type) {
		case python.Importer:
			got = "python.Importer"
		case typescript.Importer:
			got = "typescript.Importer"
		}
		if got != want {
			t.Errorf("importerFor(%q) = %T, want %s", lang, imp, want)
		}
	}

	if _, err := importerFor(""); err == nil {
		t.Error("importerFor(\"\") should error (missing --lang)")
	}
	if _, err := importerFor("cobol"); err == nil {
		t.Error("importerFor(\"cobol\") should error (unknown language)")
	}
	if _, err := importerFor("go"); err == nil {
		t.Error("importerFor(\"go\") should error with an explanation, not silently succeed")
	}
	if _, err := importerFor("swift"); err == nil {
		t.Error("importerFor(\"swift\") should error with an explanation, not silently succeed")
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

func TestRunGoUnsupported(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"check", "--lang", "go", "--dir", "."}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "compiler already refuses") {
		t.Errorf("expected an explanatory error, got %q", stderr.String())
	}
}

func TestRunAgainstPythonTestdataPassAndFail(t *testing.T) {
	jsonPath := filepath.Join(t.TempDir(), "cycle-report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--lang", "python", "--dir", "../../testdata/cycle/python",
		"--fail-above", "10", "--json", jsonPath,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("reading --json output: %v", err)
	}
	var decoded struct {
		Cycles []struct{ Files []string } `json:"cycles"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decoding --json output: %v", err)
	}
	if len(decoded.Cycles) == 0 {
		t.Error("expected at least one cycle in the JSON report (the fixture has a deliberate one)")
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"check", "--lang", "python", "--dir", "../../testdata/cycle/python",
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

	unrelated := filepath.Join(dir, "unrelated.txt")
	if err := os.WriteFile(unrelated, []byte("some/other/file.py\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--lang", "python", "--dir", "../../testdata/cycle/python",
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
	if !strings.Contains(out, "ratchet scope: 0 cycle(s)") {
		t.Errorf("expected 'ratchet scope: 0 cycle(s)', got:\n%s", out)
	}
	if !strings.Contains(out, "PASS (ratcheted)") {
		t.Errorf("expected 'PASS (ratcheted)', got:\n%s", out)
	}

	touched := filepath.Join(dir, "touched.txt")
	if err := os.WriteFile(touched, []byte("some/repo/root/testdata/cycle/python/pkg/a.py\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"check", "--lang", "python", "--dir", "../../testdata/cycle/python",
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
		"check", "--lang", "python", "--dir", "../../testdata/cycle/python",
		"--only-files", "/nonexistent/changed-files.txt",
	}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (missing --only-files file)", code)
	}
	if !strings.Contains(stderr.String(), "reading --only-files") {
		t.Errorf("expected a 'reading --only-files' error, got %q", stderr.String())
	}
}
