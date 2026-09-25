package python

import (
	"os"
	"path/filepath"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/importers"
)

func TestUnresolvedRelativeModuleIsReported(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.py"), []byte("from .missing import value\nimport json\n"), 0600); err != nil {
		t.Fatal(err)
	}
	g, _, err := (Importer{}).Import(importers.Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Unresolved) != 1 || g.Unresolved[0].Specifier != ".missing" {
		t.Fatalf("unresolved = %+v", g.Unresolved)
	}
}
