package typescript

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/tokenizers"
)

func skipIfNoNode(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not on PATH")
	}
}

func skipIfFixtureNotInstalled(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, "node_modules", "typescript")); err != nil {
		t.Skipf("%s has no installed typescript — run `npm install` there", dir)
	}
}

func TestTokenizeRealFixture(t *testing.T) {
	skipIfNoNode(t)
	dir := "../../../testdata/dupe/typescript"
	skipIfFixtureNotInstalled(t, dir)

	files, err := Tokenizer{}.Tokenize(tokenizers.Options{Dir: dir})
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1: %+v", len(files), files)
	}

	f := files[0]
	if len(f.Tokens) == 0 {
		t.Fatal("got 0 tokens from sample.ts")
	}

	counts := map[string]int{}
	for _, tok := range f.Tokens {
		counts[tok.Text]++
	}
	// The fixture's two duplicate functions should each contribute a
	// "total" and a "for" from their identical bodies.
	if counts["total"] < 2 || counts["for"] < 2 {
		t.Errorf("expected repeated tokens from the fixture's duplicate functions, got counts: %+v", counts)
	}
	// The leading "// Fixture used by..." comment should be skipped
	// (skipTrivia), so nothing should appear on line 1.
	for _, tok := range f.Tokens {
		if tok.Line == 1 {
			t.Errorf("comment content leaked through as a token: %+v", tok)
		}
	}
}

func TestTokenizeFallsBackToTypescript6ForTS7(t *testing.T) {
	skipIfNoNode(t)
	dir := "../../../testdata/dupe/typescript-ts7"
	if _, err := os.Stat(filepath.Join(dir, "node_modules", "@typescript", "typescript6")); err != nil {
		t.Skipf("%s has no installed fixtures — run `npm install` there", dir)
	}

	files, err := Tokenizer{}.Tokenize(tokenizers.Options{Dir: dir})
	if err != nil {
		t.Fatalf("Tokenize: %v (fallback to @typescript/typescript6 should have made this succeed even though typescript there is v7)", err)
	}
	if len(files) != 1 || len(files[0].Tokens) == 0 {
		t.Fatalf("got %+v, want one file with tokens", files)
	}
}
