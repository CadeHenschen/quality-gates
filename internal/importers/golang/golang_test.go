package golang

import (
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/importers"
)

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestImportResolvesLocalPackages(t *testing.T) {
	g, n, err := Importer{}.Import(importers.Options{Dir: "../../../testdata/arch/golang"})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if n != 4 {
		t.Errorf("files analyzed = %d, want 4 (domain, infra, cmd/a, cmd/b)", n)
	}

	domainEdges := g.Edges["domain/domain.go"]
	if !contains(domainEdges, "infra/infra.go") {
		t.Errorf("domain/domain.go edges = %v, want to include infra/infra.go", domainEdges)
	}

	bEdges := g.Edges["cmd/b/main.go"]
	if !contains(bEdges, "cmd/a/main.go") {
		t.Errorf("cmd/b/main.go edges = %v, want to include cmd/a/main.go", bEdges)
	}

	// infra imports nothing local — its own file's import list is empty.
	if edges := g.Edges["infra/infra.go"]; len(edges) != 0 {
		t.Errorf("infra/infra.go edges = %v, want none", edges)
	}
	if edges := g.Edges["cmd/a/main.go"]; len(edges) != 0 {
		t.Errorf("cmd/a/main.go edges = %v, want none", edges)
	}
}

func TestImportDropsExternalAndStdlibImports(t *testing.T) {
	// Every fixture file under testdata/arch/golang only imports the
	// standard library and this module's own testdata packages — a file
	// with an unresolvable import (stdlib, third-party) should simply
	// drop it rather than error, same as python/typescript's importers.
	g, _, err := Importer{}.Import(importers.Options{Dir: "../../../testdata/arch/golang"})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	for file, edges := range g.Edges {
		for _, e := range edges {
			if _, ok := g.Edges[e]; !ok {
				t.Errorf("%s has an edge to %q, which isn't itself a scanned file", file, e)
			}
		}
	}
}

func TestImportSubdirOfModule(t *testing.T) {
	// --dir doesn't have to be the module root — resolving each import's
	// full Go path still has to work when --dir is a subdirectory of the
	// enclosing module (this repo's own module root is three levels up).
	g, n, err := Importer{}.Import(importers.Options{Dir: "../../../testdata/arch/golang/domain"})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if n != 1 {
		t.Errorf("files analyzed = %d, want 1", n)
	}
	// infra isn't under this --dir, so it can't resolve to a scanned file
	// — the edge should simply be absent, not an error.
	if edges := g.Edges["domain.go"]; len(edges) != 0 {
		t.Errorf("domain.go edges = %v, want none (infra is outside --dir)", edges)
	}
}
