// Package exclude implements --exclude: glob patterns that take files out of
// a tool's analysis entirely. Unlike --only-files (which narrows only the
// gate and never the report), an excluded file is declared out of scope, so
// it is dropped before any report is built.
//
// Patterns are matched against a file's --dir-relative, slash-separated path:
//   - "*" matches within one path segment, "?" one non-slash character,
//     "**" any number of segments (including none)
//   - a pattern with no "/" matches the file's base name at any depth
//   - a pattern that matches a directory excludes everything under it
package exclude

import (
	"os"
	"regexp"
	"strings"
)

// DefaultFile is the conventional, committable exclusion file a CLI looks
// for in --dir when no explicit file is given.
const DefaultFile = ".crap-metric-exclude"

// ReadFile reads a pattern file: one glob per line, "#" starts a comment
// (whole-line or trailing, so each exclusion can be explained in place),
// blank lines ignored.
func ReadFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out, nil
}

// Set is a compiled list of exclude patterns. The zero value excludes nothing.
type Set struct {
	res []*regexp.Regexp
}

// Compile turns glob patterns into a Set. Blank patterns are ignored.
func Compile(patterns []string) (Set, error) {
	var s Set
	for _, p := range patterns {
		p = strings.TrimSpace(strings.TrimPrefix(p, "./"))
		if p == "" {
			continue
		}
		re, err := regexp.Compile(toRegexp(strings.TrimSuffix(p, "/")))
		if err != nil {
			return Set{}, err
		}
		s.res = append(s.res, re)
	}
	return s, nil
}

// Matches reports whether the --dir-relative file path is excluded.
func (s Set) Matches(file string) bool {
	file = strings.ReplaceAll(file, "\\", "/")
	for _, re := range s.res {
		if re.MatchString(file) {
			return true
		}
	}
	return false
}

func toRegexp(glob string) string {
	var b strings.Builder
	b.WriteString("^")
	if !strings.Contains(glob, "/") {
		b.WriteString("(?:.*/)?") // bare name: any depth
	}
	for i := 0; i < len(glob); i++ {
		switch c := glob[i]; c {
		case '*':
			if strings.HasPrefix(glob[i:], "**/") {
				b.WriteString("(?:.*/)?")
				i += 2
			} else if strings.HasPrefix(glob[i:], "**") {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("(?:/.*)?$") // a matched directory takes its contents
	return b.String()
}
