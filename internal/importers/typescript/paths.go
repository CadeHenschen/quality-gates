package typescript

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"git.roost-r.com/cadeh/quality-gates/internal/safefile"
)

type pathMappings struct {
	baseDir string
	entries []pathMapping
}

type pathMapping struct {
	pattern string
	targets []string
	prefix  string
	suffix  string
	wild    bool
}

func readPathMappings(root *os.Root) (pathMappings, error) {
	data, err := safefile.ReadFileAt(root, "tsconfig.json")
	if errors.Is(err, os.ErrNotExist) {
		return pathMappings{baseDir: "."}, nil
	}
	if err != nil {
		return pathMappings{}, fmt.Errorf("read tsconfig.json: %w", err)
	}
	var config struct {
		CompilerOptions *struct {
			BaseURL *string             `json:"baseUrl"`
			Paths   map[string][]string `json:"paths"`
		} `json:"compilerOptions"`
	}
	clean, err := stripJSONC(data)
	if err != nil {
		return pathMappings{}, fmt.Errorf("parse tsconfig.json: %w", err)
	}
	if err := json.Unmarshal(clean, &config); err != nil {
		return pathMappings{}, fmt.Errorf("parse tsconfig.json: %w", err)
	}
	if config.CompilerOptions == nil {
		return pathMappings{baseDir: "."}, nil
	}

	baseDir := "."
	if config.CompilerOptions.BaseURL != nil {
		baseDir = filepath.Clean(*config.CompilerOptions.BaseURL)
	}
	result := pathMappings{baseDir: filepath.ToSlash(baseDir)}
	for pattern, targets := range config.CompilerOptions.Paths {
		if len(targets) == 0 {
			return pathMappings{}, fmt.Errorf("parse tsconfig.json: paths entry %q has no targets", pattern)
		}
		first := strings.IndexByte(pattern, '*')
		wild := first >= 0
		if wild && strings.IndexByte(pattern[first+1:], '*') >= 0 {
			return pathMappings{}, fmt.Errorf("parse tsconfig.json: paths pattern %q has more than one wildcard", pattern)
		}
		mapping := pathMapping{pattern: pattern, targets: targets, wild: wild}
		if wild {
			mapping.prefix = pattern[:first]
			mapping.suffix = pattern[first+1:]
		}
		for _, target := range targets {
			if target == "" {
				return pathMappings{}, fmt.Errorf("parse tsconfig.json: paths entry %q has an empty target", pattern)
			}
		}
		result.entries = append(result.entries, mapping)
	}
	return result, nil
}

// resolve returns matched=false for an unmapped bare package import. A
// matched mapping with no scanned target returns matched=true and an empty
// target so callers can report the configured local alias as unresolved.
func (p pathMappings) resolve(spec string, existing map[string]bool) (target string, matched bool) {
	var best *pathMapping
	for i := range p.entries {
		entry := &p.entries[i]
		if !entry.matches(spec) {
			continue
		}
		if best == nil || entry.moreSpecificThan(best) {
			best = entry
		}
	}
	if best == nil {
		return "", false
	}

	wildcard := ""
	if best.wild {
		wildcard = strings.TrimSuffix(strings.TrimPrefix(spec, best.prefix), best.suffix)
	}
	for _, candidate := range best.targets {
		candidate = strings.Replace(candidate, "*", wildcard, 1)
		joined := filepath.ToSlash(filepath.Clean(filepath.Join(p.baseDir, filepath.FromSlash(candidate))))
		if target := resolveModule(joined, existing); target != "" {
			return target, true
		}
	}
	return "", true
}

func (m *pathMapping) matches(spec string) bool {
	if !m.wild {
		return spec == m.pattern
	}
	return strings.HasPrefix(spec, m.prefix) && strings.HasSuffix(spec, m.suffix) &&
		len(spec) >= len(m.prefix)+len(m.suffix)
}

func (m *pathMapping) moreSpecificThan(other *pathMapping) bool {
	if m.wild != other.wild {
		return !m.wild
	}
	if len(m.prefix) != len(other.prefix) {
		return len(m.prefix) > len(other.prefix)
	}
	return len(m.suffix) > len(other.suffix)
}

// stripJSONC removes comments and trailing commas outside strings, covering
// the JSON-with-comments syntax accepted by tsconfig.json.
func stripJSONC(src []byte) ([]byte, error) {
	clean, err := maskJSONCComments(src)
	if err != nil {
		return nil, err
	}
	return removeJSONCTrailingCommas(clean), nil
}

func maskJSONCComments(src []byte) ([]byte, error) {
	clean := append([]byte(nil), src...)
	var quote byte
	escaped := false
	lineComment, blockComment := false, false
	for i := 0; i < len(clean); i++ {
		if lineComment {
			if clean[i] == '\n' || clean[i] == '\r' {
				lineComment = false
			} else {
				clean[i] = ' '
			}
			continue
		}
		if blockComment {
			if clean[i] == '*' && i+1 < len(clean) && clean[i+1] == '/' {
				clean[i], clean[i+1] = ' ', ' '
				i++
				blockComment = false
			} else if clean[i] != '\n' && clean[i] != '\r' {
				clean[i] = ' '
			}
			continue
		}
		if quote != 0 {
			if escaped {
				escaped = false
			} else if clean[i] == '\\' {
				escaped = true
			} else if clean[i] == quote {
				quote = 0
			}
			continue
		}
		switch clean[i] {
		case '"':
			quote = clean[i]
		case '/':
			if i+1 < len(clean) && clean[i+1] == '/' {
				clean[i], clean[i+1] = ' ', ' '
				i++
				lineComment = true
			} else if i+1 < len(clean) && clean[i+1] == '*' {
				clean[i], clean[i+1] = ' ', ' '
				i++
				blockComment = true
			}
		}
	}
	if blockComment {
		return nil, fmt.Errorf("unterminated block comment")
	}
	return clean, nil
}

func removeJSONCTrailingCommas(clean []byte) []byte {
	var quote byte
	escaped := false
	for i := 0; i < len(clean); i++ {
		if quote != 0 {
			if escaped {
				escaped = false
			} else if clean[i] == '\\' {
				escaped = true
			} else if clean[i] == quote {
				quote = 0
			}
			continue
		}
		if clean[i] == '"' {
			quote = clean[i]
			continue
		}
		if clean[i] != ',' {
			continue
		}
		j := i + 1
		for j < len(clean) && (clean[j] == ' ' || clean[j] == '\t' || clean[j] == '\n' || clean[j] == '\r') {
			j++
		}
		if j < len(clean) && (clean[j] == '}' || clean[j] == ']') {
			clean[i] = ' '
		}
	}
	return clean
}
