package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/deadcode"
	"git.roost-r.com/cadeh/quality-gates/internal/reportio"
)

const goReport = "../../testdata/dead/deadcode.json" // 3 findings: main.go, util/util.go x2

func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func write(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestVersion(t *testing.T) {
	code, out, _ := runCLI(t, "version")
	if code != 0 || strings.TrimSpace(out) != "dead-metric dev" {
		t.Errorf("code %d out %q", code, out)
	}
}

func TestUsageErrors(t *testing.T) {
	jsonOut := filepath.Join(t.TempDir(), "r.json")
	cases := map[string][]string{
		"no args":          {},
		"unknown command":  {"bogus"},
		"missing --report": {"--json", jsonOut},
		"unreadable":       {"--report", "/nonexistent/report.json", "--json", jsonOut},
		"unrecognized":     {"--report", write(t, "r.json", "hello"), "--json", jsonOut},
		"bad only-files":   {"--report", goReport, "--only-files", "/nonexistent", "--json", jsonOut},
		"bad ignore file":  {"--report", goReport, "--ignore", "/nonexistent", "--json", jsonOut},
		"bad ignore glob":  {"--report", goReport, "--ignore", write(t, "ig", "[x\n"), "--json", jsonOut},
	}
	for name, args := range cases {
		if code, _, stderr := runCLI(t, args...); code != 2 || stderr == "" {
			t.Errorf("%s: want exit 2 with a message, got %d %q", name, code, stderr)
		}
	}
}

func TestGateAndJSON(t *testing.T) {
	jsonOut := filepath.Join(t.TempDir(), "dead.json")
	code, out, _ := runCLI(t, "--report", goReport, "--json", jsonOut)
	if code != 1 || !strings.Contains(out, "FAIL: 3 dead symbol(s), gate is 0") {
		t.Fatalf("code %d out:\n%s", code, out)
	}
	f, err := os.Open(jsonOut)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rep, err := reportio.ReadReport[deadcode.Report](f)
	if err != nil || rep.Count != 3 || rep.Passed {
		t.Errorf("json report: %+v, %v", rep, err)
	}

	if code, _, _ := runCLI(t, "--report", goReport, "--fail-above", "3", "--json", ""); code != 0 {
		t.Errorf("--fail-above 3 with 3 findings should pass, got %d", code)
	}
}

func TestIgnoreListRemovesFindingsFromReportAndGate(t *testing.T) {
	ig := write(t, "ignore", "# plugin entry\nUnused\nThing.Method\norphan\n")
	code, out, _ := runCLI(t, "--report", goReport, "--ignore", ig, "--json", "")
	if code != 0 || !strings.Contains(out, "PASS: 0 dead symbol(s)") {
		t.Errorf("code %d out:\n%s", code, out)
	}
}

// The ratchet narrows the gate, never the report (see CLAUDE.md).
func TestOnlyFilesNarrowsGateNotReport(t *testing.T) {
	untouched := write(t, "changed.txt", "README.md\n")
	code, out, _ := runCLI(t, "--report", goReport, "--only-files", untouched, "--json", "")
	if code != 0 {
		t.Errorf("pre-existing dead code outside changed files must not fail the gate, got %d", code)
	}
	for _, want := range []string{"main.go:14", "util/util.go:8", "ratchet scope: 0 dead symbol(s) in changed files", "PASS (ratcheted)"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in full report/verdict:\n%s", want, out)
		}
	}

	touched := write(t, "changed.txt", "sub/dir/util/util.go\n")
	code, out, _ = runCLI(t, "--report", goReport, "--only-files", touched, "--json", "")
	if code != 1 || !strings.Contains(out, "ratchet scope: 2 dead symbol(s)") || !strings.Contains(out, "FAIL (ratcheted): 2 dead symbol(s), gate is 0") {
		t.Errorf("touching a file with dead code must fail, got %d:\n%s", code, out)
	}
}

func TestVultureReportThroughCLI(t *testing.T) {
	code, out, _ := runCLI(t, "--report", "../../testdata/dead/vulture.txt", "--json", "")
	if code != 1 || !strings.Contains(out, "9 dead symbol(s) found") || !strings.Contains(out, "pkg/util.py:9") {
		t.Errorf("code %d out:\n%s", code, out)
	}
	// 60%-confidence guesses are the noisy part; gating on the rest works
	// via vulture's own --min-confidence, so a raised --fail-above is the
	// only knob dead-metric itself needs.
	if code, _, _ := runCLI(t, "--report", "../../testdata/dead/vulture.txt", "--fail-above", "9", "--json", ""); code != 0 {
		t.Errorf("--fail-above 9 should pass, got %d", code)
	}
}

func TestFormatFlag(t *testing.T) {
	empty := write(t, "vulture.txt", "")
	if code, _, stderr := runCLI(t, "--report", empty, "--json", ""); code != 2 || !strings.Contains(stderr, "--format vulture") {
		t.Errorf("an empty report must not silently pass: %d %q", code, stderr)
	}
	code, out, _ := runCLI(t, "--report", empty, "--format", "vulture", "--json", "")
	if code != 0 || !strings.Contains(out, "PASS: 0 dead symbol(s)") {
		t.Errorf("empty + --format vulture is a clean run: %d\n%s", code, out)
	}
	if code, _, stderr := runCLI(t, "--report", goReport, "--format", "bogus", "--json", ""); code != 2 || !strings.Contains(stderr, "bogus") {
		t.Errorf("unknown format: %d %q", code, stderr)
	}
}
