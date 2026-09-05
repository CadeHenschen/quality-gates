// Package cycle finds import cycles via Tarjan's strongly-connected-
// components algorithm, and gates CI on the resulting count. Go is
// deliberately not supported: the Go compiler already refuses to build a
// package-import cycle, so a cycle detector for Go source would always
// report zero — not a useful check, just a slower way to learn what
// `go build` already told you. Python and TypeScript/JS have no such
// guarantee, which is where this tool's value actually is.
package cycle

import "sort"

// Graph is a directed graph of files: Edges[a] lists every file a
// directly imports that resolved to another file within the scanned
// directory (external/unresolvable imports are dropped by the importer,
// not represented here at all).
type Graph struct {
	Edges map[string][]string
}

// Cycle is one import cycle: Files is the full strongly-connected set
// (sorted, for deterministic output); Chain is one concrete path through
// it that returns to its own start (e.g. ["a.py", "b.py", "c.py",
// "a.py"]) — less complete than Files when the cycle involves more
// back-and-forth than a single loop shows, but far more actionable: it's
// a literal trail of imports you can follow to find the one edge to cut.
type Cycle struct {
	Files []string `json:"files"`
	Chain []string `json:"chain"`
}

// FindCycles runs Tarjan's SCC algorithm over g and returns every cycle:
// every strongly-connected component of size > 1, plus any single file
// that directly imports itself.
func FindCycles(g Graph) []Cycle {
	t := &tarjanState{
		graph:   g,
		index:   map[string]int{},
		lowlink: map[string]int{},
		onStack: map[string]bool{},
	}

	for _, v := range allNodes(g) {
		if _, ok := t.index[v]; !ok {
			t.strongconnect(v)
		}
	}

	var cycles []Cycle
	for _, scc := range t.sccs {
		if len(scc) > 1 {
			cycles = append(cycles, buildCycle(g, scc))
			continue
		}
		f := scc[0]
		for _, e := range g.Edges[f] {
			if e == f {
				cycles = append(cycles, buildCycle(g, scc))
				break
			}
		}
	}

	sort.Slice(cycles, func(i, j int) bool {
		if len(cycles[i].Files) != len(cycles[j].Files) {
			return len(cycles[i].Files) > len(cycles[j].Files)
		}
		return cycles[i].Files[0] < cycles[j].Files[0]
	})
	return cycles
}

func allNodes(g Graph) []string {
	seen := map[string]bool{}
	for from, tos := range g.Edges {
		seen[from] = true
		for _, to := range tos {
			seen[to] = true
		}
	}
	nodes := make([]string, 0, len(seen))
	for n := range seen {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)
	return nodes
}

func buildCycle(g Graph, scc []string) Cycle {
	sorted := append([]string(nil), scc...)
	sort.Strings(sorted)
	return Cycle{
		Files: sorted,
		Chain: reconstructChain(g, sorted),
	}
}

// reconstructChain does a DFS within the SCC, starting from its
// (deterministic, sorted-first) node, looking for any real path back to
// the start — not necessarily the shortest one, just a genuine one, so
// there's a concrete trail to show rather than only an unordered set of
// involved files.
func reconstructChain(g Graph, scc []string) []string {
	inSCC := map[string]bool{}
	for _, f := range scc {
		inSCC[f] = true
	}
	start := scc[0]

	visited := map[string]bool{start: true}
	var dfs func(node string, path []string) []string
	dfs = func(node string, path []string) []string {
		for _, e := range g.Edges[node] {
			if e == start && len(path) > 0 {
				return append(append([]string{}, path...), start)
			}
			if inSCC[e] && !visited[e] {
				visited[e] = true
				if result := dfs(e, append(path, e)); result != nil {
					return result
				}
			}
		}
		return nil
	}

	if chain := dfs(start, []string{start}); chain != nil {
		return chain
	}
	return scc // fallback: shouldn't happen for a genuine SCC, but stay safe
}

// tarjanState is the mutable state Tarjan's algorithm threads through its
// recursive strongconnect calls.
type tarjanState struct {
	graph   Graph
	index   map[string]int
	lowlink map[string]int
	onStack map[string]bool
	stack   []string
	counter int
	sccs    [][]string
}

func (t *tarjanState) strongconnect(v string) {
	t.index[v] = t.counter
	t.lowlink[v] = t.counter
	t.counter++
	t.stack = append(t.stack, v)
	t.onStack[v] = true

	for _, w := range t.graph.Edges[v] {
		if _, ok := t.index[w]; !ok {
			t.strongconnect(w)
			if t.lowlink[w] < t.lowlink[v] {
				t.lowlink[v] = t.lowlink[w]
			}
		} else if t.onStack[w] {
			if t.index[w] < t.lowlink[v] {
				t.lowlink[v] = t.index[w]
			}
		}
	}

	if t.lowlink[v] == t.index[v] {
		var scc []string
		for {
			n := len(t.stack) - 1
			w := t.stack[n]
			t.stack = t.stack[:n]
			t.onStack[w] = false
			scc = append(scc, w)
			if w == v {
				break
			}
		}
		t.sccs = append(t.sccs, scc)
	}
}
