package gomod

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindLocatesNearestGoMod(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/thing\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "internal", "foo")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	root, modPath, err := Find(sub)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	wantRoot, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	gotRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if gotRoot != wantRoot {
		t.Errorf("root = %q, want %q", gotRoot, wantRoot)
	}
	if modPath != "example.com/thing" {
		t.Errorf("modulePath = %q, want example.com/thing", modPath)
	}
}

func TestFindNoGoMod(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := Find(dir); err == nil {
		t.Error("expected an error when no go.mod exists above dir")
	}
}

func TestFindMalformedGoMod(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("go 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Find(dir); err == nil {
		t.Error("expected an error when go.mod has no module directive")
	}
}
