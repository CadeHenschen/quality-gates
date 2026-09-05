package ratchet

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "changed.txt")
	content := "app/src/pages/Foo.tsx\n\napp/src/lib/bar.ts\n  \n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	set, err := LoadFiles(path)
	if err != nil {
		t.Fatalf("LoadFiles: %v", err)
	}
	if len(set) != 2 || !set["app/src/pages/Foo.tsx"] || !set["app/src/lib/bar.ts"] {
		t.Errorf("set = %v, want exactly the two non-blank lines", set)
	}
}

func TestLoadFilesMissingFile(t *testing.T) {
	if _, err := LoadFiles("/nonexistent/changed-files.txt"); err == nil {
		t.Error("expected an error for a missing file")
	}
}

func TestMatches(t *testing.T) {
	set := map[string]bool{"app/src/pages/Foo.tsx": true}

	cases := []struct {
		itemFile string
		want     bool
	}{
		{"pages/Foo.tsx", true},         // --dir-relative suffix of the changed (repo-root-relative) path
		{"app/src/pages/Foo.tsx", true}, // exact match
		{"Foo.tsx", true},               // still a valid path-boundary suffix
		{"pages/Bar.tsx", false},
		{"src/pages/Foo.tsx.bak", false}, // similar but not a real path-boundary suffix
	}
	for _, c := range cases {
		got := Matches(c.itemFile, set)
		if got != c.want {
			t.Errorf("Matches(%q) = %v, want %v", c.itemFile, got, c.want)
		}
	}
}

func TestLoadEmptyPathMeansNoRatchet(t *testing.T) {
	var stderr bytes.Buffer
	files, ok := Load("", "some-tool", &stderr)
	if !ok || files != nil {
		t.Errorf("Load(\"\") = (%v, %v), want (nil, true)", files, ok)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestLoadReadsRealFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "changed.txt")
	if err := os.WriteFile(path, []byte("a.go\nb.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	files, ok := Load(path, "some-tool", &stderr)
	if !ok || len(files) != 2 || !files["a.go"] || !files["b.go"] {
		t.Errorf("Load(%q) = (%v, %v), want the file's two entries", path, files, ok)
	}
}

func TestLoadMissingFileReportsToolPrefixedError(t *testing.T) {
	var stderr bytes.Buffer
	files, ok := Load("/nonexistent/changed-files.txt", "some-tool", &stderr)
	if ok || files != nil {
		t.Errorf("Load(missing) = (%v, %v), want (nil, false)", files, ok)
	}
	if got := stderr.String(); !bytes.Contains([]byte(got), []byte("some-tool: reading --only-files:")) {
		t.Errorf("stderr = %q, want it prefixed with the tool name", got)
	}
}
