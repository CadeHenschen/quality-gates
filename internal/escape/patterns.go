// Package escape counts "escape hatches" — patterns where code suppresses
// type-checking, linting, or error handling rather than resolving it —
// and gates CI on the resulting rate per 1000 lines.
//
// Deliberately regex/line-based, not AST-based, for v1: every pattern here
// is a comment marker or a whole-line shape precise enough that matching
// inside a string literal in practice doesn't happen. That buys accuracy
// without needing a real parser per language — unlike crap-metric/
// dupe-metric, which do need one for complexity/tokenizing. A natural v2
// (counting bare `any`/`interface{}`/`Any` type usage) needs real AST
// awareness to avoid false-positiving on identifiers and string contents,
// and isn't implemented — see README.
//
// Known limitation, found by this tool flagging its own doc comments: a
// comment that *mentions* a marker as documentation, rather than using it
// as an actual suppression directive, still matches — regex can't tell
// "discussing the pattern" from "invoking it". See CLAUDE.md.
package escape

import "regexp"

// Pattern is one named, regex-matched escape hatch.
type Pattern struct {
	Name  string
	Regex *regexp.Regexp
}

// LanguagePatterns maps a language key to its file extensions, test-file
// suffixes to skip, and the patterns to look for.
type LanguagePatterns struct {
	Extensions   []string
	TestSuffixes []string
	Patterns     []Pattern
}

var languages = map[string]LanguagePatterns{
	"go": {
		Extensions:   []string{".go"},
		TestSuffixes: []string{"_test.go"},
		Patterns: []Pattern{
			{Name: "nolint", Regex: regexp.MustCompile(`//\s*nolint\b`)},
			{Name: "discarded-result", Regex: regexp.MustCompile(`^\s*_\s*=\s*\w+(\.\w+)*\([^)]*\)\s*$`)},
		},
	},
	"python": {
		Extensions:   []string{".py"},
		TestSuffixes: []string{"_test.py"},
		Patterns: []Pattern{
			{Name: "type-ignore", Regex: regexp.MustCompile(`#\s*type:\s*ignore\b`)},
			{Name: "noqa", Regex: regexp.MustCompile(`#\s*noqa\b`)},
			{Name: "bare-except", Regex: regexp.MustCompile(`^\s*except\s*:\s*$`)},
		},
	},
	"ts": {
		Extensions:   []string{".ts", ".tsx", ".js", ".jsx"},
		TestSuffixes: []string{".test.ts", ".test.tsx", ".spec.ts", ".spec.tsx"},
		Patterns: []Pattern{
			{Name: "ts-ignore", Regex: regexp.MustCompile(`//\s*@ts-ignore\b`)},
			{Name: "ts-expect-error", Regex: regexp.MustCompile(`//\s*@ts-expect-error\b`)},
			{Name: "eslint-disable", Regex: regexp.MustCompile(`//\s*eslint-disable`)},
		},
	},
	"swift": {
		Extensions:   []string{".swift"},
		TestSuffixes: []string{"Tests.swift", "Test.swift"},
		Patterns: []Pattern{
			{Name: "force-try", Regex: regexp.MustCompile(`\btry!`)},
			{Name: "force-cast", Regex: regexp.MustCompile(`\bas!`)},
			// Postfix force-unwrap: a '!' right after an identifier/)/],
			// not followed by '=' (so "!=" doesn't match) and not preceded
			// by another '!' (so "x!!" only counts once per '!', and the
			// unrelated prefix "!" boolean-NOT never matches at all, since
			// that '!' isn't preceded by an identifier character). Known
			// overlap, not fixed: RE2 (Go's regexp engine) has no
			// lookbehind, so this can't tell "x!" apart from the "!" in
			// "try!"/"as!" by the single preceding character alone — a
			// force-try or force-cast line also counts as force-unwrap.
			// Accepted rather than special-cased: all three names still
			// point at the same real "!", which is still useful signal,
			// and a narrower rule would need a real parser to do properly.
			{Name: "force-unwrap", Regex: regexp.MustCompile(`[\w)\]]!([^=!]|$)`)},
			// try? silently converts a thrown error to nil, discarding it —
			// same class of hatch as "bare-except" (Python) / "discarded-
			// result" (Go).
			{Name: "discarded-error", Regex: regexp.MustCompile(`\btry\?\s`)},
			{Name: "swiftlint-disable", Regex: regexp.MustCompile(`//\s*swiftlint:disable\b`)},
		},
	},
}

// Aliases so --lang accepts the same spellings crap-metric/dupe-metric do.
var aliases = map[string]string{
	"py": "python", "golang": "go",
	"typescript": "ts", "js": "ts", "javascript": "ts",
}

// Resolve normalizes a --lang value and returns its LanguagePatterns.
func Resolve(lang string) (LanguagePatterns, bool) {
	if canon, ok := aliases[lang]; ok {
		lang = canon
	}
	lp, ok := languages[lang]
	return lp, ok
}
