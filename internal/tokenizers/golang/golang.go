// Package golang tokenizes Go source natively via go/scanner — no
// subprocess needed, mirroring crap-metric's Go analyzer.
package golang

import (
	"fmt"
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"git.roost-r.com/cadeh/quality-gates/internal/dupe"
	"git.roost-r.com/cadeh/quality-gates/internal/tokenizers"
)

type Tokenizer struct{}

func (Tokenizer) Tokenize(opts tokenizers.Options) ([]dupe.FileTokens, error) {
	var out []dupe.FileTokens
	err := filepath.WalkDir(opts.Dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "vendor" || d.Name() == "testdata" || (strings.HasPrefix(d.Name(), ".") && path != opts.Dir) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		ft, err := tokenizeFile(path, opts.Dir)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, ft)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func tokenizeFile(path, dir string) (dupe.FileTokens, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return dupe.FileTokens{}, err
	}

	fset := token.NewFileSet()
	file := fset.AddFile(path, fset.Base(), len(src))

	var s scanner.Scanner
	// No scanner.ScanComments: comments (e.g. shared license headers)
	// would otherwise dominate results with trivial matches.
	s.Init(file, src, nil, 0)

	var toks []dupe.Token
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		text := lit
		if text == "" {
			text = tok.String()
		}
		toks = append(toks, dupe.Token{Text: text, Line: fset.Position(pos).Line})
	}

	// Relative to dir, not the raw walked path — so e.g. `--dir
	// ../../src` reports "foo.go", not "../../src/foo.go" (which would
	// also break --only-files matching, whose changed-file list is
	// relative to the repo root, not to wherever --dir's own ".."
	// components happen to point).
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		rel = path
	}

	return dupe.FileTokens{File: rel, Tokens: toks}, nil
}
