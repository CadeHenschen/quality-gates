package repopath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootFindsGitRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: elsewhere"), 0o600); err != nil {
		t.Fatal(err)
	}
	scanDir := filepath.Join(root, "nested", "src")
	if err := os.MkdirAll(scanDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Root(scanDir); got != root {
		t.Errorf("Root(%q) = %q, want Git root %q", scanDir, got, root)
	}
}

func TestRootUsesContainingModuleWhenGitMetadataIsAbsent(t *testing.T) {
	moduleRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(moduleRoot, "go.mod"), []byte("module example.test/app\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	scanDir := filepath.Join(moduleRoot, "testdata", "fixture")
	if err := os.MkdirAll(scanDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(moduleRoot)
	if got := Root(scanDir); got != moduleRoot {
		t.Errorf("Root(%q) = %q, want module root %q", scanDir, got, moduleRoot)
	}
}

func TestRootFindsOutermostModuleFromScanDirectory(t *testing.T) {
	moduleRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(moduleRoot, "go.mod"), []byte("module example.test/app\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	nestedRoot := filepath.Join(moduleRoot, "nested")
	if err := os.MkdirAll(filepath.Join(nestedRoot, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedRoot, "go.mod"), []byte("module example.test/app/nested\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	scanDir := filepath.Join(nestedRoot, "src")
	for _, cwd := range []string{t.TempDir(), nestedRoot} {
		t.Chdir(cwd)
		if got := Root(scanDir); got != moduleRoot {
			t.Errorf("Root(%q) from cwd %q = %q, want outer archive module root %q", scanDir, cwd, got, moduleRoot)
		}
	}
}

func TestRootFallsBackToScanDirectoryOutsideGitAndModule(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	scanDir := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(scanDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Root(scanDir); got != scanDir {
		t.Errorf("Root(%q) = %q, want scan directory %q", scanDir, got, scanDir)
	}
}

func TestRelativeNormalizesRepoPaths(t *testing.T) {
	root := t.TempDir()
	got, err := Relative(root, filepath.Join(root, "src", "..", "app.go"))
	if err != nil {
		t.Fatalf("Relative: %v", err)
	}
	if got != "app.go" {
		t.Errorf("Relative = %q, want app.go", got)
	}
}
