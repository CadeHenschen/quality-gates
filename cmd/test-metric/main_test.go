package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/mutation"
	"git.roost-r.com/cadeh/quality-gates/internal/testmetric"
	"git.roost-r.com/cadeh/quality-gates/internal/testscanners/golang"
	"git.roost-r.com/cadeh/quality-gates/internal/testscanners/python"
	"git.roost-r.com/cadeh/quality-gates/internal/testscanners/swift"
	"git.roost-r.com/cadeh/quality-gates/internal/testscanners/typescript"
)

func exec(args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestRunUsageAndVersion(t *testing.T) {
	if code, _, stderr := exec(); code != 2 || !strings.Contains(stderr, "usage:") {
		t.Errorf("no args: code=%d stderr=%q", code, stderr)
	}
	if code, _, stderr := exec("bogus"); code != 2 || !strings.Contains(stderr, "usage:") {
		t.Errorf("unknown subcommand: code=%d stderr=%q", code, stderr)
	}
	if code, stdout, _ := exec("version"); code != 0 || !strings.Contains(stdout, "test-metric") {
		t.Errorf("version: code=%d stdout=%q", code, stdout)
	}
}

func TestScannerForDispatch(t *testing.T) {
	want := map[string]any{
		"go": golang.Scanner{}, "golang": golang.Scanner{},
		"python": python.Scanner{}, "py": python.Scanner{},
		"ts": typescript.Scanner{}, "typescript": typescript.Scanner{},
		"js": typescript.Scanner{}, "javascript": typescript.Scanner{},
		"swift": swift.Scanner{},
	}
	for lang, w := range want {
		got, err := scannerFor(lang)
		if err != nil || got != w {
			t.Errorf("scannerFor(%q) = %#v, %v; want %#v", lang, got, err, w)
		}
	}
	if _, err := scannerFor("cobol"); err == nil || !strings.Contains(err.Error(), "unknown --lang") {
		t.Errorf("cobol error = %v", err)
	}
}

func TestDisabledChecks(t *testing.T) {
	got, err := disabledChecks(" skipped, focused ,", "")
	if err != nil || !got["skipped"] || !got["focused"] || !got["interaction-only"] || got["no-assertions"] {
		t.Errorf("--ignore with no --include: %v, %v (interaction-only is opt-in, so off)", got, err)
	}
	got, err = disabledChecks("", "interaction-only")
	if err != nil || got["interaction-only"] {
		t.Errorf("--include should enable an opt-in check: %v, %v", got, err)
	}
	got, err = disabledChecks("interaction-only", "interaction-only")
	if err != nil || !got["interaction-only"] {
		t.Errorf("--ignore should win over --include: %v, %v", got, err)
	}
	if _, err := disabledChecks("skipped,nope", ""); err == nil || !strings.Contains(err.Error(), "--ignore") || !strings.Contains(err.Error(), "nope") {
		t.Errorf("unknown --ignore check should error naming the flag and value, got %v", err)
	}
	if _, err := disabledChecks("", "nope"); err == nil || !strings.Contains(err.Error(), "--include") {
		t.Errorf("unknown --include check should error naming the flag, got %v", err)
	}
}

func TestCheckFlagErrors(t *testing.T) {
	cases := map[string][]string{
		"--lang is required":   {"check"},
		"unknown --lang":       {"check", "--lang", "cobol"},
		"unknown --ignore":     {"check", "--lang", "go", "--ignore", "nope"},
		"reading --only-files": {"check", "--lang", "go", "--only-files", "/no/such/file"},
	}
	for want, args := range cases {
		if code, _, stderr := exec(args...); code != 2 || !strings.Contains(stderr, want) {
			t.Errorf("%v: code=%d stderr=%q, want exit 2 mentioning %q", args, code, stderr, want)
		}
	}
	if code, _, _ := exec("check", "--bogus-flag"); code != 2 {
		t.Errorf("bad flag: code = %d, want 2", code)
	}
}

func TestCheckScanFailure(t *testing.T) {
	if code, _, stderr := exec("check", "--lang", "go", "--dir", "/no/such/dir", "--json", ""); code != 2 || !strings.Contains(stderr, "scan:") {
		t.Errorf("code=%d stderr=%q", code, stderr)
	}
}

func TestCheckGoFixtureFailsThenPasses(t *testing.T) {
	jsonPath := filepath.Join(t.TempDir(), "test-report.json")
	fixture := "../../testdata/test/golang"

	code, stdout, stderr := exec("check", "--lang", "go", "--dir", fixture, "--json", jsonPath)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr: %s)\n%s", code, stderr, stdout)
	}
	for _, want := range []string{"no-assertions", "skipped", "temp-no-cleanup", "FAIL"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "interaction-only") {
		t.Errorf("interaction-only is opt-in and must not fire by default:\n%s", stdout)
	}
	if _, withOptIn, _ := exec("check", "--lang", "go", "--dir", fixture, "--json", "", "--include", "interaction-only"); !strings.Contains(withOptIn, "interaction-only") {
		t.Errorf("--include interaction-only should enable it:\n%s", withOptIn)
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var decoded testmetric.Report
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Findings) == 0 || decoded.Tests != 16 || decoded.Passed {
		t.Errorf("decoded report = %+v", decoded)
	}

	if code, _, _ := exec("check", "--lang", "go", "--dir", fixture, "--json", "", "--fail-above", "100"); code != 0 {
		t.Errorf("--fail-above 100: exit code = %d, want 0", code)
	}
	ignoreAll := strings.Join(testmetric.Kinds, ",")
	if code, _, _ := exec("check", "--lang", "go", "--dir", fixture, "--json", "", "--ignore", ignoreAll); code != 0 {
		t.Errorf("--ignore everything: exit code = %d, want 0", code)
	}
}

func TestCheckRatchetScopesTheGateNotTheReport(t *testing.T) {
	fixture := "../../testdata/test/golang"
	dir := t.TempDir()
	untouched := filepath.Join(dir, "changed.txt")
	if err := os.WriteFile(untouched, []byte("some/other/file_test.go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	touched := filepath.Join(dir, "touched.txt")
	if err := os.WriteFile(touched, []byte("pkg/sample_test.go\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, stdout, _ := exec("check", "--lang", "go", "--dir", fixture, "--json", "", "--only-files", untouched)
	if code != 0 || !strings.Contains(stdout, "PASS (ratcheted)") {
		t.Errorf("untouched files: code=%d\n%s", code, stdout)
	}
	if !strings.Contains(stdout, "no-assertions") {
		t.Errorf("the full report must still list pre-existing findings:\n%s", stdout)
	}

	code, stdout, _ = exec("check", "--lang", "go", "--dir", fixture, "--json", "", "--only-files", touched)
	if code != 1 || !strings.Contains(stdout, "FAIL (ratcheted)") {
		t.Errorf("touched file: code=%d\n%s", code, stdout)
	}
}

func TestMutationFlagErrors(t *testing.T) {
	cases := map[string][]string{
		"--report is required": {"mutation"},
		"reading --report":     {"mutation", "--report", "/no/such/file"},
		"reading --only-files": {"mutation", "--report", "x", "--only-files", "/no/such/file"},
	}
	for want, args := range cases {
		if code, _, stderr := exec(args...); code != 2 || !strings.Contains(stderr, want) {
			t.Errorf("%v: code=%d stderr=%q, want exit 2 mentioning %q", args, code, stderr, want)
		}
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte(`{"x":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := exec("mutation", "--report", bad); code != 2 || !strings.Contains(stderr, "unrecognized") {
		t.Errorf("unrecognized report: code=%d stderr=%q", code, stderr)
	}
	if code, _, _ := exec("mutation", "--bogus-flag"); code != 2 {
		t.Errorf("bad flag: code = %d, want 2", code)
	}
}

func TestMutationGate(t *testing.T) {
	report := "../../testdata/test/mutation/gremlins.json" // 2 detected, 1 survived, 1 not covered: 50%
	jsonPath := filepath.Join(t.TempDir(), "mutation-report.json")

	code, stdout, stderr := exec("mutation", "--report", report, "--json", jsonPath)
	if code != 1 || !strings.Contains(stdout, "FAIL: 50.0% is below 60.0%") {
		t.Fatalf("default gate: code=%d stderr=%s\n%s", code, stderr, stdout)
	}
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var decoded mutation.Report
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Detected != 2 || decoded.Survived != 1 || decoded.NoCoverage != 1 || decoded.Passed {
		t.Errorf("decoded report = %+v", decoded)
	}

	if code, _, _ := exec("mutation", "--report", report, "--json", "", "--fail-below", "50"); code != 0 {
		t.Errorf("--fail-below 50: code = %d, want 0", code)
	}
	if code, _, _ := exec("mutation", "--report", report, "--json", "", "--covered-only"); code != 0 {
		t.Errorf("--covered-only (66.7%%): code = %d, want 0", code)
	}
}

func TestMutationMinMutants(t *testing.T) {
	report := "../../testdata/test/mutation/gremlins.json" // 4 graded mutants, 50%: fails the default gate

	code, stdout, _ := exec("mutation", "--report", report, "--json", "", "--min-mutants", "5")
	if code != 0 || !strings.Contains(stdout, "PASS: only 4 graded mutants (fewer than --min-mutants 5)") {
		t.Errorf("--min-mutants 5: code=%d\n%s", code, stdout)
	}
	if code, _, _ := exec("mutation", "--report", report, "--json", "", "--min-mutants", "4"); code != 1 {
		t.Errorf("--min-mutants 4: code = %d, want 1 (enough mutants to enforce)", code)
	}

	// The ratcheted scope gets the same waiver: calc.go alone has 3 graded
	// mutants at 33%, which fails at min 3 and is waived at min 4.
	touched := filepath.Join(t.TempDir(), "calc.txt")
	if err := os.WriteFile(touched, []byte("internal/calc/calc.go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := exec("mutation", "--report", report, "--json", "", "--fail-below", "10", "--only-files", touched, "--min-mutants", "4"); code != 0 {
		t.Errorf("ratchet + waiver: code = %d, want 0", code)
	}
	code, stdout, _ = exec("mutation", "--report", report, "--json", "", "--only-files", touched, "--min-mutants", "4")
	if code != 0 || !strings.Contains(stdout, "PASS (ratcheted)") {
		t.Errorf("ratchet waives scope with too few mutants: code=%d\n%s", code, stdout)
	}
	code, _, _ = exec("mutation", "--report", report, "--json", "", "--only-files", touched, "--min-mutants", "3")
	if code != 1 {
		t.Errorf("ratchet at min 3 of 3: code = %d, want 1", code)
	}
}

func TestMutationMinimumGradedAndDocsOnlyRatchet(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "calc.go"), []byte("package calc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(report, []byte(`{"mutants":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, _ := exec("mutation", "--report", report, "--minimum-graded", "1", "--json", "")
	if code != 1 || !strings.Contains(out, "insufficient analysis") {
		t.Fatalf("full: code=%d out=%s", code, out)
	}
	changed := filepath.Join(dir, "changed.txt")
	if err := os.WriteFile(changed, []byte("README.md\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, _ = exec("mutation", "--report", report, "--minimum-graded", "1", "--lang", "go", "--dir", dir, "--only-files", changed, "--json", "")
	if code != 0 || !strings.Contains(out, "not applicable") {
		t.Fatalf("docs: code=%d out=%s", code, out)
	}
	if err := os.WriteFile(changed, []byte("calc.go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, _ = exec("mutation", "--report", report, "--minimum-graded", "1", "--lang", "go", "--dir", dir, "--only-files", changed, "--json", "")
	if code != 1 || !strings.Contains(out, "missing: calc.go") {
		t.Fatalf("source: code=%d out=%s", code, out)
	}
}

func TestMutationRatchetScopesTheGateNotTheReport(t *testing.T) {
	report := "../../testdata/test/mutation/gremlins.json"
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	// util.go's only verdict is a caught mutant: the scope passes even
	// though the whole-report score fails.
	code, stdout, _ := exec("mutation", "--report", report, "--json", "", "--only-files", write("util.txt", "internal/util/util.go\n"))
	if code != 0 || !strings.Contains(stdout, "PASS (ratcheted)") || !strings.Contains(stdout, "FAIL: 50.0%") {
		t.Errorf("util.go scope: code=%d\n%s", code, stdout)
	}
	// calc.go: 1 of 3 caught.
	code, stdout, _ = exec("mutation", "--report", report, "--json", "", "--only-files", write("calc.txt", "internal/calc/calc.go\n"))
	if code != 1 || !strings.Contains(stdout, "FAIL (ratcheted): 33.3%") {
		t.Errorf("calc.go scope: code=%d\n%s", code, stdout)
	}
}

func TestCheckSwiftFixture(t *testing.T) {
	code, stdout, stderr := exec("check", "--lang", "swift", "--dir", "../../testdata/test/swift", "--json", "")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr: %s)\n%s", code, stderr, stdout)
	}
	for _, want := range []string{"no-assertions", "skipped", "expected-failure", "temp-no-cleanup", "SampleTests.swift", "ModernTests.swift"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output missing %q:\n%s", want, stdout)
		}
	}
}
