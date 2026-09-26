package python

import (
	_ "embed"
	"path/filepath"

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
// uses — so Analyze can look a radonEntry's size facts up directly.
func sizeFacts(dir string) (map[sizeKey]sizeFact, error) {
	var facts []sizeFact
	if err := embedscript.Run("python3", sizeScript, dir, &facts); err != nil {
		return nil, err
	}
	out := make(map[sizeKey]sizeFact, len(facts))
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	for _, f := range facts {
		absFile, err := filepath.Abs(f.File)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(absDir, absFile)
		if err != nil {
			return nil, err
		}
		out[sizeKey{filepath.Join(dir, rel), f.Lineno}] = f
	}
	return out, nil
}
