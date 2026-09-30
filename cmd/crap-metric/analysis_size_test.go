package main

import (
	"os"
	"path/filepath"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/crap"
)

func TestStampFileMaxLineLengths(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "internal", "sample.go"), []byte("short\nλ界🙂-line\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fns := []crap.Function{
		{File: "internal/sample.go", Name: "First"},
		{File: "internal/sample.go", Name: "Second"},
	}
	if err := stampFileMaxLineLengths(dir, fns); err != nil {
		t.Fatalf("stampFileMaxLineLengths: %v", err)
	}
	for _, fn := range fns {
		if fn.FileMaxLineLength != 8 || fn.FileMaxLineLengthLine != 2 {
			t.Errorf("%s got max line (%d, %d), want (8, 2)", fn.Name, fn.FileMaxLineLength, fn.FileMaxLineLengthLine)
		}
	}
}

func TestStampFileMaxLineLengthsRejectsMissingSource(t *testing.T) {
	if err := stampFileMaxLineLengths(t.TempDir(), []crap.Function{{File: "missing.go"}}); err == nil {
		t.Fatal("stampFileMaxLineLengths should return an error for an unreadable source")
	}
}
