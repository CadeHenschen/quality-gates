package golang

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/tokenizers"
)

func TestTokenizeSkipsCommentsAndTestFiles(t *testing.T) {
	files, err := Tokenizer{}.Tokenize(tokenizers.Options{Dir: "../../../testdata/dupe/golang"})
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1 (sample.go — a _test.go fixture would leak in if excluded wrongly): %+v", len(files), files)
	}

	f := files[0]
	for _, tok := range f.Tokens {
		if len(tok.Text) > 1 && tok.Text[0] == '/' {
			t.Errorf("comment-shaped token leaked through: %q", tok.Text)
		}
	}
	// Sanity: the fixture's two identical function bodies mean "total" and
	// "range" should each appear at least twice.
	counts := map[string]int{}
	for _, tok := range f.Tokens {
		counts[tok.Text]++
	}
	if counts["total"] < 2 || counts["range"] < 2 {
		t.Errorf("expected repeated tokens from the fixture's duplicate functions, got counts: %+v", counts)
	}
}

func TestTokenizeSkipsTestdataDir(t *testing.T) {
	files, err := Tokenizer{}.Tokenize(tokenizers.Options{Dir: "../../.."})
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("got 0 files analyzing dupe-metric's own repo — something's wrong")
	}
	for _, f := range files {
		if containsTestdata(f.File) {
			t.Errorf("testdata file leaked into analysis: %s", f.File)
		}
	}
}

func containsTestdata(path string) bool {
	p := filepath.ToSlash(path)
	return strings.Contains(p, "/testdata/") || strings.HasPrefix(p, "testdata/")
}

func TestTokenizeEmptyDirNoError(t *testing.T) {
	dir := t.TempDir()
	files, err := Tokenizer{}.Tokenize(tokenizers.Options{Dir: dir})
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("got %d files from an empty dir, want 0", len(files))
	}
}

func TestTokenizeSkipsGoTestFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "real.go"), []byte("package p\nfunc F() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "real_test.go"), []byte("package p\nfunc TestF(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := Tokenizer{}.Tokenize(tokenizers.Options{Dir: dir})
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	if len(files) != 1 || files[0].File != "real.go" {
		t.Errorf("got %+v, want only real.go (relative to dir)", files)
	}
}
