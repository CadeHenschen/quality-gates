// Package embedscript runs an embedded interpreter script against a
// caller-selected working directory and parses its JSON stdout. Shared by Python and
// TypeScript tokenizers (internal/tokenizers/{python,typescript}) and the
// TypeScript complexity analyzer (internal/analyzers/typescript) — all
// three independently implemented the same "run an embedded script and
// unmarshal JSON stdout" sequence until this package factored it out (the same class of gap
// dupe-metric's self-check caught in the tokenizers before — see
// CLAUDE.md — just one dupe-metric's shingle matcher didn't itself catch,
// since the two copies' surrounding code differed enough).
package embedscript

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

// Run sends scriptContent to a fixed interpreter on stdin from dir and
// unmarshals its JSON stdout into out. No caller-controlled value becomes
// an executable name or command-line argument.
func Run(interpreter string, scriptContent []byte, dir string, out any) error {
	var cmd *exec.Cmd
	switch interpreter {
	case "node":
		cmd = exec.Command("node")
	case "python3":
		cmd = exec.Command("python3")
	default:
		return fmt.Errorf("unsupported embedded-script interpreter %q", interpreter)
	}
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(scriptContent)
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
