package python

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/tokenizers"
)

func skipIfNoPython3(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
}

func TestTokenizeRealFixture(t *testing.T) {
	skipIfNoPython3(t)

	files, err := Tokenizer{}.Tokenize(tokenizers.Options{Dir: "../../../testdata/dupe/python"})
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1: %+v", len(files), files)
	}

	f := files[0]
	if len(f.Tokens) == 0 {
		t.Fatal("got 0 tokens from sample.py")
	}

	counts := map[string]int{}
	for _, tok := range f.Tokens {
		counts[tok.Text]++
		if tok.Text == "" {
			t.Error("got an empty token text")
		}
	}
	// The fixture's two duplicate functions should each contribute a
	// "total" and a "range"-equivalent "for"/"in" pair.
	if counts["total"] < 2 || counts["for"] < 2 {
		t.Errorf("expected repeated tokens from the fixture's duplicate functions, got counts: %+v", counts)
	}
	// No comment or docstring-only content should appear as its own
	// token (the fixture's leading "# Fixture used by..." comment line).
	for _, tok := range f.Tokens {
		if tok.Text == "#" || tok.Line == 1 {
			t.Errorf("comment content leaked through as a token: %+v at line %d", tok.Text, tok.Line)
		}
	}
}

func TestTokenizeSkipsTestFiles(t *testing.T) {
	skipIfNoPython3(t)

	dir := t.TempDir()
	writeFile(t, dir, "real.py", "def f():\n    return 1\n")
	writeFile(t, dir, "test_real.py", "def test_f():\n    assert f() == 1\n")
	writeFile(t, dir, "other_test.py", "def test_other():\n    pass\n")

	files, err := Tokenizer{}.Tokenize(tokenizers.Options{Dir: dir})
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1 (only real.py): %+v", len(files), files)
	}
}

func TestTokenizeEmptyDirNoError(t *testing.T) {
	skipIfNoPython3(t)

	files, err := Tokenizer{}.Tokenize(tokenizers.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("got %d files from an empty dir, want 0", len(files))
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
