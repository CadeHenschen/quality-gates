// Package golang tokenizes Go source natively via go/scanner — no
// subprocess needed, mirroring crap-metric's Go analyzer.
package golang

import (
	"errors"
	"fmt"
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"git.roost-r.com/cadeh/quality-gates/internal/dupe"
	"git.roost-r.com/cadeh/quality-gates/internal/safefile"
	"git.roost-r.com/cadeh/quality-gates/internal/tokenizers"
)

type Tokenizer struct{}

func (Tokenizer) Tokenize(opts tokenizers.Options) (out []dupe.FileTokens, retErr error) {
	root, err := safefile.OpenRoot(opts.Dir)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	err = filepath.WalkDir(opts.Dir, func(path string, d os.DirEntry, err error) error {
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

		rel, err := filepath.Rel(opts.Dir, path)
		if err != nil {
			return err
		}
		ft, err := tokenizeFile(root, filepath.ToSlash(rel), rel)
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

func tokenizeFile(root *os.Root, openPath, reportPath string) (dupe.FileTokens, error) {
	src, err := safefile.ReadFileAt(root, openPath)
	if err != nil {
		return dupe.FileTokens{}, err
	}

	fset := token.NewFileSet()
	file := fset.AddFile(reportPath, fset.Base(), len(src))

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

	return dupe.FileTokens{File: reportPath, Tokens: toks}, nil
}
