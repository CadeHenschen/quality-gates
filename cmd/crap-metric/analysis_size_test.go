package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/crap"
	"git.roost-r.com/cadeh/quality-gates/internal/exclude"
)

func TestWriteCrapRatchetDescribesFunctionSizeFailure(t *testing.T) {
	scoped := crap.NewReport([]crap.Function{{
		File: "changed.go", Name: "long", StartLine: 1, EndLine: 100,
		Complexity: 1, LinesTotal: 1, LinesCovered: 1,
	}}, 30).WithSize(crap.SizeThresholds{MaxLines: 80})
	if scoped.Passed {
		t.Fatal("expected the function-size threshold to fail the ratcheted report")
	}
	var out bytes.Buffer
	writeCrapRatchet(&out, scoped, 30)
	if !strings.Contains(out.String(), "function-size threshold") {
		t.Errorf("ratchet summary omits function-size failure: %s", out.String())
	}
}

func TestCollectFileSizesIncludesFunctionlessVisitedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := "type One interface{}\n\n" + strings.Repeat("x", 40) + "\r\n"
	if err := os.WriteFile(filepath.Join(dir, "internal", "types.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	files, err := collectFileSizes(dir, []string{"internal/types.go", "internal/types.go"}, nil, exclude.Set{})
	if err != nil {
		t.Fatalf("collectFileSizes: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("file sizes = %+v, want one deduplicated fact", files)
	}
	if got := files[0]; got.File != "internal/types.go" || got.Lines != 3 || got.MaxLineLength != 40 || got.MaxLineLengthLine != 3 {
		t.Errorf("file size = %+v, want types.go, 3 lines, max line 40 at line 3", got)
	}
}

func TestCollectFileSizesHonorsExclusionsAndMeasuresFunctionFiles(t *testing.T) {
	dir := t.TempDir()
	for file, source := range map[string]string{"keep.go": "func f() {}\n", "ignored.go": strings.Repeat("x", 20)} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	excluded, err := exclude.Compile([]string{"ignored.go"})
	if err != nil {
		t.Fatal(err)
	}
	fns := []crap.Function{{File: "keep.go", FileLines: 99}}
	files, err := collectFileSizes(dir, []string{"keep.go", "ignored.go"}, fns, excluded)
	if err != nil {
		t.Fatalf("collectFileSizes: %v", err)
	}
	if len(files) != 1 || files[0].File != "keep.go" || files[0].Lines != 99 {
		t.Errorf("file sizes = %+v, want included keep.go with analyzer line count and no excluded file", files)
	}
	stampFunctionFileSizes(fns, files)
	if fns[0].FileMaxLineLength != len("func f() {}") || fns[0].FileMaxLineLengthLine != 1 {
		t.Errorf("function file-size fields = %+v, want measured max line length", fns[0])
	}
}

func TestCollectFileSizesRejectsMissingSource(t *testing.T) {
	if _, err := collectFileSizes(t.TempDir(), []string{"missing.go"}, nil, exclude.Set{}); err == nil {
		t.Fatal("collectFileSizes should return an error for an unreadable visited source")
	}
}
