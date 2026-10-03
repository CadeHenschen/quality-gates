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
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{"app/src/pages/Foo.tsx": true}

	cases := []struct {
		itemFile string
		want     bool
	}{
		{"app/src/pages/Foo.tsx", true}, // exact match
		{"Foo.tsx", false},              // suffixes do not identify a unique file
		{"pages/Bar.tsx", false},
		{"src/pages/Foo.tsx.bak", false}, // similar but not a real path-boundary suffix
	}
	for _, c := range cases {
		got := MatchesInDir(c.itemFile, set, dir)
		if got != c.want {
			t.Errorf("Matches(%q) = %v, want %v", c.itemFile, got, c.want)
		}
	}
}

func TestMatchesInDirUsesExactRepositoryPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "app", "src")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	changed := map[string]bool{"app/src/pages/Foo.tsx": true}
	for _, tc := range []struct {
		item string
		want bool
	}{
		{"pages/Foo.tsx", true},
		{"pages/nested/../Foo.tsx", true},
		{"../src/pages/Foo.tsx", true},
		{"../../other/src/pages/Foo.tsx", false},
		{"pages/Foo.tsx.bak", false},
	} {
		if got := MatchesInDir(tc.item, changed, dir); got != tc.want {
			t.Errorf("MatchesInDir(%q) = %v, want %v", tc.item, got, tc.want)
		}
	}
}

func TestMatchesInDirWithoutGitUsesExactDirRelativePaths(t *testing.T) {
	dir := t.TempDir()
	changed := map[string]bool{"src/Foo.go": true}
	if !MatchesInDir("src/./Foo.go", changed, dir) {
		t.Error("expected normalized exact path to match without a Git root")
	}
	if MatchesInDir("other/src/Foo.go", changed, dir) {
		t.Error("unrelated same-suffix path matched without a Git root")
	}
}

func TestMatchesPathResolvesAbsolutePathInArchiveModuleFromOutside(t *testing.T) {
	moduleRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(moduleRoot, "go.mod"), []byte("module example.test/archive\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	policyPath := filepath.Join(moduleRoot, "config", "arch-rules.json")
	if err := os.MkdirAll(filepath.Dir(policyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policyPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	changed := map[string]bool{"config/arch-rules.json": true}
	if !MatchesPath(policyPath, changed) {
		t.Errorf("MatchesPath(%q) = false, want exact archive-module path match", policyPath)
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
