package swift

import (
	"path/filepath"
	"strings"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/tokenizers"
)

func TestTokenizeSkipsCommentsAndTestFiles(t *testing.T) {
	files, err := Tokenizer{}.Tokenize(tokenizers.Options{Dir: "../../../testdata/dupe/swift"})
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1 (sample.swift only)", len(files))
	}

	f := files[0]
	if f.File != "sample.swift" {
		t.Errorf("File = %q, want dir-relative %q", f.File, "sample.swift")
	}
	for _, tok := range f.Tokens {
		if strings.HasPrefix(tok.Text, "//") {
			t.Errorf("comment-shaped token leaked through: %q", tok.Text)
		}
	}
	// Sanity: the fixture's two identical function bodies mean "total" and
	// "items" should each appear at least twice.
	counts := map[string]int{}
	for _, tok := range f.Tokens {
		counts[tok.Text]++
	}
	if counts["total"] < 2 || counts["items"] < 2 {
		t.Errorf("expected repeated tokens from the fixture's duplicate functions, got counts: %+v", counts)
	}
}

func TestTokenizeSkipsTestsDirAndTestFiles(t *testing.T) {
	files, err := Tokenizer{}.Tokenize(tokenizers.Options{Dir: "../../../testdata/crap/swift"})
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	for _, f := range files {
		if f.File == "CalculatorTests.swift" {
			t.Errorf("CalculatorTests.swift should be skipped (test-file suffix)")
		}
		if strings.Contains(filepath.ToSlash(f.File), "Tests/") {
			t.Errorf("file under Tests/ leaked into tokenization: %s", f.File)
		}
	}
}

func TestTokenizeEmptyDirNoError(t *testing.T) {
	files, err := Tokenizer{}.Tokenize(tokenizers.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("got %d files from an empty dir, want 0", len(files))
	}
}
