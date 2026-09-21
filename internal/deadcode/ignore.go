package deadcode

import (
	"bufio"
	"fmt"
	"io"
	"path"
	"strings"
)

// Ignore is an allowlist of findings that are dead to the analyzer but live
// in fact: reflection targets, plugin entry points, a library's public API.
// Without it the false positives get the whole gate switched off.
//
// One entry per line; blank lines and `#` comments are skipped. An entry is:
//   - `dir/`     a path prefix (everything under it)
//   - a glob with `/` in it, matched against the whole file path; a leading
//     `**/` means "at any depth" (`*` never crosses a `/`)
//   - a glob with no `/` (`*_gen.go`), matched against the file's base name
//   - anything else, a symbol name; `Registered` also covers the method
//     `Thing.Registered`
type Ignore struct {
	rules []rule
}

type rule struct {
	prefix string // path prefix, ends in "/"
	glob   string // path or base-name glob
	base   bool   // glob matches the base name only
	symbol string // exact symbol name
}

// ParseIgnore reads an ignore list, rejecting malformed globs.
func ParseIgnore(r io.Reader) (Ignore, error) {
	var ig Ignore
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch {
		case strings.HasSuffix(line, "/") && !strings.ContainsAny(line, "*?["):
			ig.rules = append(ig.rules, rule{prefix: line})
		case strings.ContainsAny(line, "*?[") || strings.Contains(line, "/"):
			if _, err := path.Match(strings.TrimPrefix(line, "**/"), ""); err != nil {
				return Ignore{}, fmt.Errorf("ignore list line %d: bad glob %q: %w", n, line, err)
			}
			ig.rules = append(ig.rules, rule{glob: line, base: !strings.Contains(line, "/")})
		default:
			ig.rules = append(ig.rules, rule{symbol: line})
		}
	}
	return ig, sc.Err()
}

// Matches reports whether f is covered by an entry.
func (ig Ignore) Matches(f Finding) bool {
	for _, r := range ig.rules {
		switch {
		case r.prefix != "":
			if strings.HasPrefix(f.File, r.prefix) {
				return true
			}
		case r.base:
			if ok, _ := path.Match(r.glob, path.Base(f.File)); ok {
				return true
			}
		case r.glob != "":
			if globMatches(r.glob, f.File) {
				return true
			}
		default:
			if f.Name == r.symbol || strings.HasSuffix(f.Name, "."+r.symbol) {
				return true
			}
		}
	}
	return false
}

// globMatches matches file against glob; a leading `**/` lets the rest match
// at any directory depth, including none.
func globMatches(glob, file string) bool {
	rest, anyDepth := strings.CutPrefix(glob, "**/")
	if ok, _ := path.Match(rest, file); ok {
		return true
	}
	if !anyDepth {
		return false
	}
	for i, c := range file {
		if c == '/' {
			if ok, _ := path.Match(rest, file[i+1:]); ok {
				return true
			}
		}
	}
	return false
}

// Filter returns the findings not covered by any entry.
func (ig Ignore) Filter(findings []Finding) []Finding {
	var out []Finding
	for _, f := range findings {
		if !ig.Matches(f) {
			out = append(out, f)
		}
	}
	return out
}
