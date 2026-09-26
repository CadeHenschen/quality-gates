// Package typescript tokenizes TS/JS source by shelling out to an
// embedded Node script (scanner.js) that walks source via the *target
// repo's own* installed `typescript` package's scanner.
package typescript

import (
	_ "embed"

	"git.roost-r.com/cadeh/quality-gates/internal/dupe"
	"git.roost-r.com/cadeh/quality-gates/internal/embedscript"
	"git.roost-r.com/cadeh/quality-gates/internal/tokenizers"
)

//go:embed scanner.js
var scannerScript []byte

type Tokenizer struct{}

func (Tokenizer) Tokenize(opts tokenizers.Options) ([]dupe.FileTokens, error) {
	var raw []tokenizers.RawFileTokens
	if err := embedscript.Run("node", scannerScript, opts.Dir, &raw); err != nil {
		return nil, err
	}
	return tokenizers.ToFileTokens(raw), nil
}
