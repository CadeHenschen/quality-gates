package arch

import (
	"sort"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/cycle"
)

func TestEdgesFromFileGraph(t *testing.T) {
	g := cycle.Graph{Edges: map[string][]string{
		"pkg/domain/service.py": {"pkg/infra/db.py"},
		"pkg/infra/db.py":       nil,
		"standalone.py":         {"pkg/domain/service.py"},
	}}

	edges := EdgesFromFileGraph(g)
	sort.Slice(edges, func(i, j int) bool { return edges[i].File < edges[j].File })

	if len(edges) != 2 {
		t.Fatalf("got %d edge(s), want 2: %+v", len(edges), edges)
	}

	want := map[string]Edge{
		"pkg/domain/service.py": {File: "pkg/domain/service.py", FromPkg: "pkg/domain", Import: "pkg/infra/db.py", ToPkg: "pkg/infra"},
		"standalone.py":         {File: "standalone.py", FromPkg: ".", Import: "pkg/domain/service.py", ToPkg: "pkg/domain"},
	}
	for _, e := range edges {
		w, ok := want[e.File]
		if !ok {
			t.Errorf("unexpected edge for file %q: %+v", e.File, e)
			continue
		}
		if e != w {
			t.Errorf("edge for %q = %+v, want %+v", e.File, e, w)
		}
	}
}
