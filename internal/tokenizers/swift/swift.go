// Package swift tokenizes Swift source via internal/swiftlex — no
// subprocess, mirroring dupe-metric's Go tokenizer (which sits on
// go/scanner the same way this sits on swiftlex).
package swift

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"git.roost-r.com/cadeh/quality-gates/internal/dupe"
	"git.roost-r.com/cadeh/quality-gates/internal/safefile"
	"git.roost-r.com/cadeh/quality-gates/internal/swiftlex"
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
			name := d.Name()
			if name == ".build" || name == ".swiftpm" || name == "Pods" || name == "Tests" ||
				(strings.HasPrefix(name, ".") && path != opts.Dir) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".swift") || isTestFile(d.Name()) {
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

func isTestFile(name string) bool {
	return strings.HasSuffix(name, "Tests.swift") || strings.HasSuffix(name, "Test.swift")
}

func tokenizeFile(root *os.Root, openPath, reportPath string) (dupe.FileTokens, error) {
	src, err := safefile.ReadFileAt(root, openPath)
	if err != nil {
		return dupe.FileTokens{}, err
	}

	var toks []dupe.Token
	for _, t := range swiftlex.Tokenize(src) {
		if t.Kind == swiftlex.Comment {
			continue
		}
		toks = append(toks, dupe.Token{Text: t.Text, Line: t.Line})
	}

	return dupe.FileTokens{File: reportPath, Tokens: toks}, nil
}
