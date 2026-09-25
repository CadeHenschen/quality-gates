package evidence

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckFullAndRatchetedEvidence(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "web", "src")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.ts", "index.ts", "readme.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	full, err := Check(dir, "ts", false, []string{"a.ts"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if full.Passed || full.Required != 2 || len(full.Missing) != 1 || full.Missing[0] != "index.ts" {
		t.Fatalf("full = %+v", full)
	}
	docs, err := Check(dir, "ts", false, nil, map[string]bool{"readme.md": true})
	if err != nil {
		t.Fatal(err)
	}
	if !docs.Passed || docs.Required != 0 {
		t.Fatalf("docs = %+v", docs)
	}
	changed, err := Check(dir, "ts", false, []string{"a.ts"}, map[string]bool{"web/src/index.ts": true})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Passed || changed.Required != 1 || changed.Missing[0] != "index.ts" {
		t.Fatalf("changed = %+v", changed)
	}
}

func TestCheckEmptyFullScanFails(t *testing.T) {
	r, err := Check(t.TempDir(), "go", false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Passed || r.Reason == "" {
		t.Fatalf("empty = %+v", r)
	}
}

func TestEligibleLanguageConventions(t *testing.T) {
	cases := []struct {
		name, lang  string
		tests, want bool
	}{
		{"app.go", "go", false, true}, {"app_test.go", "go", false, false}, {"app_test.go", "go", true, true},
		{"app.py", "python", false, true}, {"test_app.py", "python", true, true}, {"app_test.py", "python", false, false},
		{"app.ts", "ts", false, true}, {"app.d.ts", "ts", false, false}, {"app.spec.jsx", "js", true, true},
		{"App.swift", "swift", false, true}, {"AppTests.swift", "swift", true, true}, {"AppTests.swift", "swift", false, false},
		{"app.rs", "rust", false, false},
	}
	for _, c := range cases {
		if got := languageFile(c.name, c.lang, c.tests); got != c.want {
			t.Errorf("%s/%s tests=%v: got %v want %v", c.name, c.lang, c.tests, got, c.want)
		}
	}
}
