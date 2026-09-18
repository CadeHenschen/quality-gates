package embedscript

import (
	"os/exec"
	"strings"
	"testing"
)

// rawFileTokens mirrors the JSON shape the tests' inline scripts emit —
// just enough to exercise Run's unmarshaling, decoupled from any real
// consumer's wire type.
type rawFileTokens struct {
	File   string `json:"file"`
	Tokens []struct {
		Text string `json:"text"`
		Line int    `json:"line"`
	} `json:"tokens"`
}

func TestRunRealPython(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}

	script := []byte(`
import json, sys
print(json.dumps([{"file": sys.argv[1], "tokens": [{"text": "ok", "line": 1}]}]))
`)

	var raw []rawFileTokens
	if err := Run("python3", script, ".py", "some-dir", &raw); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(raw) != 1 || raw[0].File != "some-dir" || raw[0].Tokens[0].Text != "ok" {
		t.Errorf("raw = %+v", raw)
	}
}

func TestRunPropagatesStderr(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}

	script := []byte(`
import sys
print("a specific failure message", file=sys.stderr)
sys.exit(1)
`)

	var raw []rawFileTokens
	err := Run("python3", script, ".py", "dir", &raw)
	if err == nil {
		t.Fatal("expected an error from a script that exits non-zero")
	}
	if !strings.Contains(err.Error(), "a specific failure message") {
		t.Errorf("error = %v, want it to include the script's stderr", err)
	}
}

func TestRunBadInterpreter(t *testing.T) {
	var raw []rawFileTokens
	err := Run("this-interpreter-does-not-exist", []byte("x"), ".x", "dir", &raw)
	if err == nil {
		t.Fatal("expected an error for a nonexistent interpreter")
	}
}
