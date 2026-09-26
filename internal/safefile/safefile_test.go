package safefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFileAtRejectsSymlinkEscape(t *testing.T) {
	rootDir := t.TempDir()
	outsideDir := t.TempDir()
	secret := filepath.Join(outsideDir, "secret.go")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(rootDir, "escape.go")); err != nil {
		t.Fatal(err)
	}

	root, err := OpenRoot(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			t.Errorf("close root: %v", err)
		}
	}()

	if _, err := ReadFileAt(root, "escape.go"); err == nil {
		t.Fatal("ReadFileAt followed a symlink outside its root")
	}
}

func TestReadFilePathAllowsCallerSelectedParentButRejectsFinalSymlinkEscape(t *testing.T) {
	rootDir := t.TempDir()
	outsideDir := t.TempDir()
	secret := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(rootDir, "link.txt")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadFile(link); err == nil {
		t.Fatal("ReadFile followed a final symlink outside the caller-selected parent")
	}
	data, err := ReadFile(secret)
	if err != nil || string(data) != "secret" {
		t.Fatalf("ReadFile(%q) = %q, %v; want secret, nil", secret, data, err)
	}
}

func TestReadFileAtRejectsParentTraversal(t *testing.T) {
	root, err := OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			t.Errorf("close root: %v", err)
		}
	}()

	if _, err := ReadFileAt(root, "../outside"); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("ReadFileAt traversal error = %v, want outside-root error", err)
	}
}
