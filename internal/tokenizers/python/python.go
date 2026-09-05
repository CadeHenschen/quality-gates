// Package python tokenizes Python source by shelling out to an embedded
// script (tokenize_files.py) that uses the standard library's `tokenize`
// module — no external tool or pip dependency needed, since tokenize is
// always available with any python3.
package python

import (
	_ "embed"

	"git.roost-r.com/cadeh/quality-gates/internal/dupe"
	"git.roost-r.com/cadeh/quality-gates/internal/tokenizers"
)

//go:embed tokenize_files.py
var tokenizeScript []byte

type Tokenizer struct{}

func (Tokenizer) Tokenize(opts tokenizers.Options) ([]dupe.FileTokens, error) {
	var raw []tokenizers.RawFileTokens
	if err := tokenizers.RunEmbeddedScript("python3", tokenizeScript, ".py", opts.Dir, &raw); err != nil {
		return nil, err
	}
	return tokenizers.ToFileTokens(raw), nil
}
