package tokenizers

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/dupe"
)

func TestToFileTokens(t *testing.T) {
	raw := []RawFileTokens{
		{
			File: "a.py",
			Tokens: []struct {
				Text string `json:"text"`
				Line int    `json:"line"`
			}{
				{Text: "def", Line: 1},
				{Text: "f", Line: 1},
			},
		},
	}

	got := ToFileTokens(raw)
	want := []dupe.FileTokens{
		{File: "a.py", Tokens: []dupe.Token{{Text: "def", Line: 1}, {Text: "f", Line: 1}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ToFileTokens(%+v) = %+v, want %+v", raw, got, want)
	}
}

func TestToFileTokensEmpty(t *testing.T) {
	got := ToFileTokens(nil)
	if len(got) != 0 {
		t.Errorf("ToFileTokens(nil) = %+v, want empty", got)
	}
}

func TestRunEmbeddedScriptRealPython(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}

	script := []byte(`
import json, sys
print(json.dumps([{"file": sys.argv[1], "tokens": [{"text": "ok", "line": 1}]}]))
`)

	var raw []RawFileTokens
	if err := RunEmbeddedScript("python3", script, ".py", "some-dir", &raw); err != nil {
		t.Fatalf("RunEmbeddedScript: %v", err)
	}
	if len(raw) != 1 || raw[0].File != "some-dir" || raw[0].Tokens[0].Text != "ok" {
		t.Errorf("raw = %+v", raw)
	}
}

func TestRunEmbeddedScriptPropagatesStderr(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}

	script := []byte(`
import sys
print("a specific failure message", file=sys.stderr)
sys.exit(1)
`)

	var raw []RawFileTokens
	err := RunEmbeddedScript("python3", script, ".py", "dir", &raw)
	if err == nil {
		t.Fatal("expected an error from a script that exits non-zero")
	}
	if !strings.Contains(err.Error(), "a specific failure message") {
		t.Errorf("error = %v, want it to include the script's stderr", err)
	}
}

func TestRunEmbeddedScriptBadInterpreter(t *testing.T) {
	var raw []RawFileTokens
	err := RunEmbeddedScript("this-interpreter-does-not-exist", []byte("x"), ".x", "dir", &raw)
	if err == nil {
		t.Fatal("expected an error for a nonexistent interpreter")
	}
}
