package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/importers/golang"
	"git.roost-r.com/cadeh/quality-gates/internal/importers/python"
	"git.roost-r.com/cadeh/quality-gates/internal/importers/typescript"
)

func TestImporterFor(t *testing.T) {
	cases := map[string]string{
		"python": "python.Importer", "py": "python.Importer",
		"ts": "typescript.Importer", "typescript": "typescript.Importer",
		"js": "typescript.Importer", "javascript": "typescript.Importer",
		"go": "golang.Importer", "golang": "golang.Importer",
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
		case golang.Importer:
			got = "golang.Importer"
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

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"version"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("run([\"version\"]) exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "arch-metric") {
		t.Errorf("expected version output to mention arch-metric, got %q", stdout.String())
	}
}

func TestRunAgainstGoTestdataPassAndFail(t *testing.T) {
	jsonPath := filepath.Join(t.TempDir(), "arch-report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/arch/golang",
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
		Violations []struct {
			Rule string `json:"rule"`
		} `json:"violations"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decoding --json output: %v", err)
	}
	if len(decoded.Violations) != 2 {
		t.Fatalf("expected 2 violations in the JSON report (the fixture has two deliberate ones), got %d: %+v", len(decoded.Violations), decoded.Violations)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/arch/golang",
		"--fail-above", "0", "--json", jsonPath,
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "FAIL") {
		t.Errorf("expected FAIL in output, got:\n%s", stdout.String())
	}
}

func TestRunAgainstPythonTestdata(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--lang", "python", "--dir", "../../testdata/arch/python",
		"--fail-above", "0", "--json", filepath.Join(t.TempDir(), "report.json"),
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "domain must not depend on infra") {
		t.Errorf("expected the rule name in output, got:\n%s", stdout.String())
	}
}

func TestRunAgainstTypescriptTestdata(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--lang", "ts", "--dir", "../../testdata/arch/typescript",
		"--fail-above", "0", "--json", filepath.Join(t.TempDir(), "report.json"),
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr: %s)", code, stderr.String())
	}
}

func TestRunDefaultRulesFileLookup(t *testing.T) {
	// No --rules flag: falls back to <dir>/.arch-metric-rules.json, which
	// the golang testdata fixture carries.
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/arch/golang",
		"--fail-above", "0", "--json", filepath.Join(t.TempDir(), "report.json"),
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr: %s)", code, stderr.String())
	}
}

func TestRunMissingRulesFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--lang", "go", "--dir", t.TempDir(),
		"--json", filepath.Join(t.TempDir(), "report.json"),
	}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (no rules file present, and none given)", code)
	}
	if !strings.Contains(stderr.String(), "loading rules") {
		t.Errorf("expected a 'loading rules' error, got %q", stderr.String())
	}
}

func TestRunOnlyFilesRatchet(t *testing.T) {
	dir := t.TempDir()

	unrelated := filepath.Join(dir, "unrelated.txt")
	if err := os.WriteFile(unrelated, []byte("some/other/file.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/arch/golang",
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
	if !strings.Contains(out, "ratchet scope: 0 violation(s)") {
		t.Errorf("expected 'ratchet scope: 0 violation(s)', got:\n%s", out)
	}
	if !strings.Contains(out, "PASS (ratcheted)") {
		t.Errorf("expected 'PASS (ratcheted)', got:\n%s", out)
	}

	touched := filepath.Join(dir, "touched.txt")
	if err := os.WriteFile(touched, []byte("some/repo/root/testdata/arch/golang/domain/domain.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/arch/golang",
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
		"check", "--lang", "go", "--dir", "../../testdata/arch/golang",
		"--only-files", "/nonexistent/changed-files.txt",
	}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (missing --only-files file)", code)
	}
	if !strings.Contains(stderr.String(), "reading --only-files") {
		t.Errorf("expected a 'reading --only-files' error, got %q", stderr.String())
	}
}

func TestRunPolicyFileChangeUsesFullGate(t *testing.T) {
	dir := t.TempDir()
	changed := filepath.Join(dir, "changed.txt")
	if err := os.WriteFile(changed, []byte("some/repo/root/testdata/arch/golang/.arch-metric-rules.json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/arch/golang",
		"--fail-above", "0", "--only-files", changed,
		"--json", filepath.Join(dir, "report.json"),
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (a changed policy must gate every violation; stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "layer policy changed; evaluating every violation") {
		t.Errorf("expected full-policy ratchet explanation, got:\n%s", stdout.String())
	}
}

func TestRunStabilityAgainstFixturePassAndFail(t *testing.T) {
	jsonPath := filepath.Join(t.TempDir(), "arch-stability-report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"stability", "--lang", "go", "--dir", "../../testdata/arch/golang-stability",
		"--fail-above", "5", "--json", jsonPath,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("reading --json output: %v", err)
	}
	var decoded struct {
		Violations []struct {
			FromPkg string `json:"from_pkg"`
			ToPkg   string `json:"to_pkg"`
		} `json:"violations"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decoding --json output: %v", err)
	}
	if len(decoded.Violations) != 1 || decoded.Violations[0].FromPkg != "stable" || decoded.Violations[0].ToPkg != "unstable" {
		t.Fatalf("expected exactly one stable -> unstable violation, got %+v", decoded.Violations)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"stability", "--lang", "go", "--dir", "../../testdata/arch/golang-stability",
		"--fail-above", "0", "--json", jsonPath,
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "FAIL") {
		t.Errorf("expected FAIL in output, got:\n%s", stdout.String())
	}
}

func TestRunCheckExceptionDropsGrandfatheredViolation(t *testing.T) {
	rulesPath := filepath.Join(t.TempDir(), "rules.json")
	rulesJSON := `{
		"rules": [
			{"name": "domain must not depend on infra", "from": "domain", "deny": ["infra"]},
			{"name": "cmd binaries are independent processes", "from": "cmd/*", "deny": ["cmd/*"]}
		],
		"exceptions": [
			{"rule": "domain must not depend on infra", "from": "domain", "to": "infra", "reason": "test grandfather"}
		]
	}`
	if err := os.WriteFile(rulesPath, []byte(rulesJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	jsonPath := filepath.Join(t.TempDir(), "arch-report.json")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check", "--lang", "go", "--dir", "../../testdata/arch/golang",
		"--rules", rulesPath, "--fail-above", "0", "--json", jsonPath,
	}, &stdout, &stderr)
	// The cmd/* violation still fires — only the exempted domain -> infra
	// one should be gone, from both the table and the JSON report.
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (the un-exempted cmd/* violation should still gate), stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "domain must not depend on infra") {
		t.Errorf("exempted violation should not appear in the table, got:\n%s", stdout.String())
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("reading --json output: %v", err)
	}
	var decoded struct {
		Violations []struct {
			Rule string `json:"rule"`
		} `json:"violations"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decoding --json output: %v", err)
	}
	if len(decoded.Violations) != 1 || decoded.Violations[0].Rule != "cmd binaries are independent processes" {
		t.Fatalf("expected exactly the un-exempted cmd/* violation in the JSON report, got %+v", decoded.Violations)
	}
}

func TestRunDiffReportsNewAndFixed(t *testing.T) {
	oldPath := filepath.Join(t.TempDir(), "old.json")
	newPath := filepath.Join(t.TempDir(), "new.json")

	oldReport := `{"violations":[{"rule":"r","from_pkg":"a","to_pkg":"b","file":"a/gone.go","import":"b/y.go"}],"files_analyzed":1,"fail_above":0,"passed":false}`
	newReport := `{"violations":[{"rule":"r","from_pkg":"a","to_pkg":"b","file":"a/added.go","import":"b/y.go"}],"files_analyzed":1,"fail_above":0,"passed":false}`
	if err := os.WriteFile(oldPath, []byte(oldReport), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte(newReport), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"diff", "--old", oldPath, "--new", newPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (diff is informational only, stderr: %s)", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "new") || !strings.Contains(out, "a/added.go") {
		t.Errorf("expected the new violation in output, got:\n%s", out)
	}
	if !strings.Contains(out, "fixed") || !strings.Contains(out, "a/gone.go") {
		t.Errorf("expected the fixed violation in output, got:\n%s", out)
	}
}

func TestRunDiffJSON(t *testing.T) {
	oldPath := filepath.Join(t.TempDir(), "old.json")
	newPath := filepath.Join(t.TempDir(), "new.json")
	if err := os.WriteFile(oldPath, []byte(`{"violations":[],"files_analyzed":0,"fail_above":0,"passed":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte(`{"violations":[{"rule":"r","from_pkg":"a","to_pkg":"b","file":"a/x.go","import":"b/y.go"}],"files_analyzed":1,"fail_above":0,"passed":false}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"diff", "--old", oldPath, "--new", newPath, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	var decoded []struct {
		Change    string `json:"change"`
		Violation struct {
			File string `json:"file"`
		} `json:"violation"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("decoding --json diff output: %v", err)
	}
	if len(decoded) != 1 || decoded[0].Change != "new" || decoded[0].Violation.File != "a/x.go" {
		t.Errorf("decoded diff = %+v", decoded)
	}
}

func TestRunDiffMissingFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"diff", "--old", "x.json"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (missing --new)", code)
	}
	if !strings.Contains(stderr.String(), "requires both --old and --new") {
		t.Errorf("expected a 'requires both' error, got %q", stderr.String())
	}
}

func TestRunDiffMissingOldFile(t *testing.T) {
	newPath := filepath.Join(t.TempDir(), "new.json")
	if err := os.WriteFile(newPath, []byte(`{"violations":[],"files_analyzed":0,"fail_above":0,"passed":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"diff", "--old", "/nonexistent/old.json", "--new", newPath}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (missing --old file)", code)
	}
	if !strings.Contains(stderr.String(), "reading --old") {
		t.Errorf("expected a 'reading --old' error, got %q", stderr.String())
	}
}

func TestRunDiffMissingNewFile(t *testing.T) {
	oldPath := filepath.Join(t.TempDir(), "old.json")
	if err := os.WriteFile(oldPath, []byte(`{"violations":[],"files_analyzed":0,"fail_above":0,"passed":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"diff", "--old", oldPath, "--new", "/nonexistent/new.json"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (missing --new file)", code)
	}
	if !strings.Contains(stderr.String(), "reading --new") {
		t.Errorf("expected a 'reading --new' error, got %q", stderr.String())
	}
}

func TestRunStabilityBadLang(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"stability", "--lang", "cobol", "--dir", "."}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (unknown --lang)", code)
	}
}

func TestRunStabilityOnlyFilesRatchet(t *testing.T) {
	dir := t.TempDir()

	unrelated := filepath.Join(dir, "unrelated.txt")
	if err := os.WriteFile(unrelated, []byte("some/other/file.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"stability", "--lang", "go", "--dir", "../../testdata/arch/golang-stability",
		"--fail-above", "0", "--only-files", unrelated,
		"--json", filepath.Join(dir, "report.json"),
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (untouched files shouldn't be gated, stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "ratchet scope: 0 violation(s)") {
		t.Errorf("expected 'ratchet scope: 0 violation(s)', got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "PASS (ratcheted)") {
		t.Errorf("expected 'PASS (ratcheted)', got:\n%s", stdout.String())
	}

	touched := filepath.Join(dir, "touched.txt")
	if err := os.WriteFile(touched, []byte("some/repo/root/testdata/arch/golang-stability/stable/stable.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"stability", "--lang", "go", "--dir", "../../testdata/arch/golang-stability",
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

func TestRunStabilityOnlyFilesMissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"stability", "--lang", "go", "--dir", "../../testdata/arch/golang-stability",
		"--only-files", "/nonexistent/changed-files.txt",
	}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (missing --only-files file)", code)
	}
}

func TestRunStabilityBaselineGatesOnlyRegressions(t *testing.T) {
	dir := t.TempDir()
	baseline := filepath.Join(dir, "baseline.json")
	// The existing stable -> unstable violation is grandfathered by the
	// baseline, so the same full report passes the baseline gate.
	if err := os.WriteFile(baseline, []byte(`{"violations":[{"from_pkg":"stable","to_pkg":"unstable"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"stability", "--lang", "go", "--dir", "../../testdata/arch/golang-stability",
		"--fail-above", "0", "--baseline", baseline,
		"--json", filepath.Join(dir, "report.json"),
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (existing violation is in baseline; stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "baseline scope: 0 new stability violation(s)") {
		t.Errorf("expected baseline scope, got:\n%s", stdout.String())
	}
}

func TestRunStabilityRejectsBaselineAndOnlyFiles(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"stability", "--lang", "go", "--dir", "../../testdata/arch/golang-stability",
		"--baseline", "old.json", "--only-files", "changed.txt",
	}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "cannot be combined") {
		t.Errorf("exit code/stderr = %d/%q, want a mutual-exclusion error", code, stderr.String())
	}
}

func TestRunStabilityNoViolationsOnLayerFixture(t *testing.T) {
	// The layer-rules fixture's domain -> infra and cmd/b -> cmd/a edges
	// both point toward the more stable side already, so they shouldn't
	// trip the stability gate at all.
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"stability", "--lang", "go", "--dir", "../../testdata/arch/golang",
		"--fail-above", "0", "--json", filepath.Join(t.TempDir(), "report.json"),
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}
}
