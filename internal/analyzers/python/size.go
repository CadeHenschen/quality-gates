package python

import (
	_ "embed"

	"git.roost-r.com/cadeh/quality-gates/internal/embedscript"
)

//go:embed size_facts.py
var sizeScript []byte

// sizeFact is one function's size facts, as emitted by size_facts.py.
type sizeFact struct {
	File            string `json:"file"`
	Lineno          int    `json:"lineno"`
	ParamCount      int    `json:"param_count"`
	MaxNestingDepth int    `json:"max_nesting_depth"`
	FileLines       int    `json:"file_lines"`
}

type sizeKey struct {
	file   string
	lineno int
}

// sizeFacts runs size_facts.py over dir and indexes its output by (file,
// lineno) — the function's "def" line, the same key radon's own Lineno
// uses — so Analyze can look a radonEntry's size facts up directly. dir is
// passed through unmodified, exactly like runRadon(dir), so both
// subprocesses key their "file" strings the same way and a lookup by the
// literal radon-reported path always hits.
func sizeFacts(dir string) (map[sizeKey]sizeFact, error) {
	var facts []sizeFact
	if err := embedscript.Run("python3", sizeScript, ".py", dir, &facts); err != nil {
		return nil, err
	}
	out := make(map[sizeKey]sizeFact, len(facts))
	for _, f := range facts {
		out[sizeKey{f.File, f.Lineno}] = f
	}
	return out, nil
}
