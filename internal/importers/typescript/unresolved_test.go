package typescript

import (
	"os"
	"path/filepath"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/importers"
)

func TestUnresolvedRelativeImportIsReported(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.ts"), []byte(`import x from "./missing"; import y from "react";`), 0600); err != nil {
		t.Fatal(err)
	}
	g, _, err := (Importer{}).Import(importers.Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Unresolved) != 1 || g.Unresolved[0].File != "a.ts" || g.Unresolved[0].Specifier != "./missing" {
		t.Fatalf("unresolved = %+v", g.Unresolved)
	}
}

func TestCommentedImportDoesNotCreateUnresolvedIssue(t *testing.T) {
	dir := t.TempDir()
	source := "// import \"./absent\"\n/* import \"./gone\" */\nconst url = \"https://example.test/x\";\n"
	if err := os.WriteFile(filepath.Join(dir, "a.ts"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	g, _, err := (Importer{}).Import(importers.Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Unresolved) != 0 {
		t.Fatalf("commented imports: %+v", g.Unresolved)
	}
}
