package ratchet

import (
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
