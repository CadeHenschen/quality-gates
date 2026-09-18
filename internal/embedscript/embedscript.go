// Package embedscript runs an embedded interpreter script against a
// directory and parses its JSON stdout. Shared by the Python and
// TypeScript tokenizers (internal/tokenizers/{python,typescript}) and the
// TypeScript complexity analyzer (internal/analyzers/typescript) — all
// three independently did the identical "write embedded script content to
// a temp file, run `interpreter tempfile dir`, unmarshal JSON stdout"
// sequence until this package factored it out (the same class of gap
// dupe-metric's self-check caught in the tokenizers before — see
// CLAUDE.md — just one dupe-metric's shingle matcher didn't itself catch,
// since the two copies' surrounding code differed enough).
package embedscript

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// Run writes scriptContent to a temp file (suffixed ext, so the interpreter
// recognizes the file type), runs `interpreter <tempfile> dir`, and
// unmarshals its JSON stdout into out.
func Run(interpreter string, scriptContent []byte, ext, dir string, out any) error {
	tmp, err := os.CreateTemp("", "quality-gates-script-*"+ext)
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(scriptContent); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	cmd := exec.Command(interpreter, tmp.Name(), dir)
	stdout, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("%s script failed: %w: %s", interpreter, err, exitErr.Stderr)
		}
		return fmt.Errorf("run %s (is it installed?): %w", interpreter, err)
	}

	if err := json.Unmarshal(stdout, out); err != nil {
		return fmt.Errorf("parse %s script output: %w", interpreter, err)
	}
	return nil
}
