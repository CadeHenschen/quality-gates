package cycle

import (
	"reflect"
	"testing"
)

func TestFindCyclesNoCycle(t *testing.T) {
	g := Graph{Edges: map[string][]string{
		"a.py": {"b.py"},
		"b.py": {"c.py"},
		"c.py": {},
	}}
	cycles := FindCycles(g)
	if len(cycles) != 0 {
		t.Errorf("got %d cycles, want 0: %+v", len(cycles), cycles)
	}
}

func TestFindCyclesSimpleTwoNodeCycle(t *testing.T) {
	g := Graph{Edges: map[string][]string{
		"a.py": {"b.py"},
		"b.py": {"a.py"},
	}}
	cycles := FindCycles(g)
	if len(cycles) != 1 {
		t.Fatalf("got %d cycles, want 1: %+v", len(cycles), cycles)
	}
	if !reflect.DeepEqual(cycles[0].Files, []string{"a.py", "b.py"}) {
		t.Errorf("Files = %v, want [a.py b.py]", cycles[0].Files)
	}
	if !reflect.DeepEqual(cycles[0].Chain, []string{"a.py", "b.py", "a.py"}) {
		t.Errorf("Chain = %v, want [a.py b.py a.py]", cycles[0].Chain)
	}
}

func TestFindCyclesThreeNodeCycle(t *testing.T) {
	g := Graph{Edges: map[string][]string{
		"a.py": {"b.py"},
		"b.py": {"c.py"},
		"c.py": {"a.py"},
	}}
	cycles := FindCycles(g)
	if len(cycles) != 1 {
		t.Fatalf("got %d cycles, want 1: %+v", len(cycles), cycles)
	}
	if len(cycles[0].Files) != 3 {
		t.Errorf("Files = %v, want 3 files", cycles[0].Files)
	}
	if len(cycles[0].Chain) != 4 || cycles[0].Chain[0] != cycles[0].Chain[3] {
		t.Errorf("Chain = %v, want a 4-element loop back to its own start", cycles[0].Chain)
	}
}

func TestFindCyclesSelfImport(t *testing.T) {
	g := Graph{Edges: map[string][]string{
		"a.py": {"a.py"},
	}}
	cycles := FindCycles(g)
	if len(cycles) != 1 || len(cycles[0].Files) != 1 || cycles[0].Files[0] != "a.py" {
		t.Errorf("got %+v, want one self-cycle on a.py", cycles)
	}
}

func TestFindCyclesEdgeTargetOnlyNodeStillVisited(t *testing.T) {
	// "c.py" only ever appears as an edge *target*, never as a key in
	// Edges — allNodes must still find and visit it.
	g := Graph{Edges: map[string][]string{
		"a.py": {"b.py"},
		"b.py": {"c.py", "a.py"},
	}}
	cycles := FindCycles(g)
	if len(cycles) != 1 || len(cycles[0].Files) != 2 {
		t.Errorf("got %+v, want one 2-file cycle (a.py, b.py); c.py has no path back so isn't part of it", cycles)
	}
}

func TestFindCyclesMultipleIndependentCycles(t *testing.T) {
	g := Graph{Edges: map[string][]string{
		"a.py": {"b.py"}, "b.py": {"a.py"}, // cycle 1
		"x.py": {"y.py", "z.py"}, "y.py": {"z.py"}, "z.py": {"x.py"}, // cycle 2 (3 files)
		"solo.py": {}, // not part of any cycle
	}}
	cycles := FindCycles(g)
	if len(cycles) != 2 {
		t.Fatalf("got %d cycles, want 2: %+v", len(cycles), cycles)
	}
	// Sorted largest-first: the 3-file cycle comes before the 2-file one.
	if len(cycles[0].Files) != 3 || len(cycles[1].Files) != 2 {
		t.Errorf("cycles = %+v, want sizes [3, 2]", cycles)
	}
}
