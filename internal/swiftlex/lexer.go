// Package swiftlex is a hand-written, native lexer for Swift source — no
// subprocess, no Swift toolchain dependency. It's the "stdlib" internal/
// analyzers/swift and internal/tokenizers/swift both sit on, mirroring how
// Go's own analyzer/tokenizer both sit on go/parser and go/scanner — except
// here we're writing the standard library ourselves, since Go has none for
// Swift.
//
// Three documented v1 scope limitations, chosen deliberately rather than
// built around with a heuristic (same posture as escape-metric's
// "regex-only is a deliberate v1 scope" and dupe-metric's "boundary bleed
// is expected" — see CLAUDE.md):
//
//   - String interpolation ("\(expr)") contents are not tokenized
//     separately — the entire string literal, interpolation included, is
//     emitted as one opaque String token. Duplicated logic living only
//     inside a string interpolation won't be caught by dupe-metric. The
//     lexer still tracks interpolation nesting internally so the literal's
//     own boundaries are always found correctly, including when a nested
//     interpolated expression contains its own string with its own quotes
//     (e.g. `"prefix \(x == "a" ? 1 : 2) suffix"`).
//   - Regex literals ("/pattern/", Swift 5.7+) are not supported: '/' is
//     always lexed as an operator. A wrong guess at regex-vs-division
//     wouldn't just miscount one token — it would corrupt brace-depth
//     tracking for the rest of the file, which function-boundary detection
//     in internal/analyzers/swift depends on. That failure mode is worse
//     than under-supporting a literal form that's rare in practice.
//   - Ternary `cond ? a : b` is not distinguished from optional chaining or
//     an optional-type '?' — both are lexed as plain Operator tokens.
//     Telling them apart needs real parsing, not just lexing.
package swiftlex

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Kind classifies a Token.
type Kind int

const (
	Ident Kind = iota
	Keyword
	Number
	String
	Operator
	Punct
	Comment
)

// Token is one lexical token. Line is the token's starting line
// (1-indexed); for a multi-line string or block comment, Text spans
// multiple lines but Line is only where it began.
type Token struct {
	Text string
	Line int
	Kind Kind
}

// Tokenize lexes Swift source into a token stream.
func Tokenize(src []byte) []Token {
	l := &lexer{src: src, line: 1}
	l.run()
	return l.toks
}

// keywords are always classified as Keyword regardless of context. This
// includes both fully-reserved words and Swift's contextual keywords
// (get/set/willSet/didSet/some/any/mutating/...) — safe to always treat as
// keywords here since our callers match on exact token text, and a real
// identifier named e.g. "get" would need backtick-escaping to be legal
// Swift anyway (handled separately: backtick-quoted identifiers always
// lex as Ident, never Keyword, even if the quoted text matches this set).
var keywords = map[string]bool{
	"associatedtype": true, "class": true, "deinit": true, "enum": true,
	"extension": true, "fileprivate": true, "func": true, "import": true,
	"init": true, "inout": true, "internal": true, "let": true,
	"open": true, "operator": true, "private": true, "precedencegroup": true,
	"protocol": true, "public": true, "rethrows": true, "static": true,
	"struct": true, "subscript": true, "typealias": true, "var": true,
	"actor": true,

	"break": true, "case": true, "catch": true, "continue": true,
	"default": true, "defer": true, "do": true, "else": true,
	"fallthrough": true, "for": true, "guard": true, "if": true,
	"in": true, "repeat": true, "return": true, "throw": true,
	"switch": true, "where": true, "while": true,

	"as": true, "Any": true, "false": true, "is": true, "nil": true,
	"self": true, "Self": true, "super": true, "throws": true,
	"true": true, "try": true,

	"async": true, "await": true, "get": true, "set": true,
	"willSet": true, "didSet": true, "mutating": true, "nonmutating": true,
	"lazy": true, "final": true, "required": true, "convenience": true,
	"indirect": true, "weak": true, "unowned": true, "some": true,
	"any": true, "dynamic": true, "override": true, "optional": true,
	"unsafe": true,
}

const operatorChars = "/=-+!*%<>&|^~?."
const punctChars = "(){}[],;:@"

type frameKind int

const (
	frameString frameKind = iota
	frameInterp
	frameComment
)

// frame is one entry on the lexer's mode stack. Its meaning depends on
// kind: for frameString, hash/multiline describe the string's delimiter
// and startPos/startLine mark where its opening delimiter began (so the
// whole literal can be sliced out as one Token once its terminator is
// found); for frameInterp, depth is a single combined bracket-depth
// counter over '(' '[' '{' vs their closers (contents aren't tokenized, so
// which bracket kind opened it doesn't matter — only when it's back to 0);
// for frameComment, depth counts nested "/* */" (Swift, unlike C, allows
// nesting them).
type frame struct {
	kind      frameKind
	hash      int
	multiline bool
	depth     int
	startLine int
	startPos  int
	// topLevel is true only for a frameString begun directly from
	// scanTopLevel (frames empty at that point), never for one begun from
	// inside another string's interpolation (scanInInterp). Only a
	// top-level string frame emits a Token when it closes — a nested one's
	// span is just part of the outer string's own Text, per this
	// package's documented choice not to tokenize interpolation contents.
	topLevel bool
}

type lexer struct {
	src    []byte
	pos    int
	line   int
	frames []frame
	toks   []Token
}

func (l *lexer) run() {
	for l.pos < len(l.src) {
		if len(l.frames) > 0 {
			switch l.frames[len(l.frames)-1].kind {
			case frameComment:
				l.scanInBlockComment()
			case frameString:
				l.scanInString()
			case frameInterp:
				l.scanInInterp()
			}
			continue
		}
		l.scanTopLevel()
	}
}

func (l *lexer) peekByte(offset int) byte {
	i := l.pos + offset
	if i < 0 || i >= len(l.src) {
		return 0
	}
	return l.src[i]
}

func (l *lexer) scanTopLevel() {
	c := l.src[l.pos]
	switch {
	case c == '\n':
		l.line++
		l.pos++
	case c == ' ' || c == '\t' || c == '\r':
		l.pos++
	case c == '/' && l.peekByte(1) == '/':
		l.scanLineComment()
	case c == '/' && l.peekByte(1) == '*':
		l.startBlockComment()
	case c == '"' || c == '#':
		if !l.tryStartString() {
			l.emitPunct(1)
		}
	case c == '`':
		l.scanBacktickIdent()
	case l.identStartsHere():
		l.scanIdent()
	case c >= '0' && c <= '9':
		l.scanNumber()
	case isOperatorByte(c):
		l.scanOperatorRun()
	case isPunctByte(c):
		l.emitPunct(1)
	default:
		l.advanceRune()
	}
}

func (l *lexer) advanceRune() {
	_, size := utf8.DecodeRune(l.src[l.pos:])
	if size == 0 {
		size = 1
	}
	l.pos += size
}

func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || r > unicode.MaxASCII
}

func isIdentPart(r rune) bool {
	return isIdentStart(r) || unicode.IsDigit(r)
}

func (l *lexer) identStartsHere() bool {
	r, _ := utf8.DecodeRune(l.src[l.pos:])
	return isIdentStart(r)
}

func isOperatorByte(c byte) bool {
	return strings.IndexByte(operatorChars, c) >= 0
}

func isPunctByte(c byte) bool {
	return strings.IndexByte(punctChars, c) >= 0
}

func isASCIIDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func (l *lexer) emitPunct(n int) {
	end := l.pos + n
	if end > len(l.src) {
		end = len(l.src)
	}
	l.toks = append(l.toks, Token{Text: string(l.src[l.pos:end]), Line: l.line, Kind: Punct})
	l.pos = end
}

func (l *lexer) scanIdent() {
	start, startLine := l.pos, l.line
	for l.pos < len(l.src) {
		r, size := utf8.DecodeRune(l.src[l.pos:])
		if !isIdentPart(r) {
			break
		}
		l.pos += size
	}
	text := string(l.src[start:l.pos])
	kind := Ident
	if keywords[text] {
		kind = Keyword
	}
	l.toks = append(l.toks, Token{Text: text, Line: startLine, Kind: kind})
}

func (l *lexer) scanBacktickIdent() {
	start, startLine := l.pos, l.line
	l.pos++ // opening `
	for l.pos < len(l.src) && l.src[l.pos] != '`' && l.src[l.pos] != '\n' {
		l.pos++
	}
	if l.pos < len(l.src) && l.src[l.pos] == '`' {
		l.pos++
	}
	l.toks = append(l.toks, Token{Text: string(l.src[start:l.pos]), Line: startLine, Kind: Ident})
}

// scanNumber is deliberately permissive rather than spec-exact (e.g. it
// doesn't validate that octal/binary literals only contain their own
// digit range) — getting a number literal's own text slightly wrong
// doesn't corrupt brace-depth tracking or keyword matching, which is what
// the rest of this package is careful about. It does take care not to
// swallow the '.' that starts a range operator ("1...5"), the sign of an
// unrelated following expression ("1 +5"), or (the sharp edge here) treat
// a decimal exponent's 'e' as just another hex-ish digit, which would
// otherwise swallow "1e+5" as "1e" before the exponent's "+5" is even
// looked at.
func (l *lexer) scanNumber() {
	start, startLine := l.pos, l.line
	decDigit := func(c byte) bool { return isASCIIDigit(c) || c == '_' }
	hexDigit := func(c byte) bool {
		return decDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
	}

	digit := decDigit
	expChars := "eE"
	l.pos++ // the leading digit that got us into scanNumber
	if l.src[start] == '0' {
		switch l.peekByte(0) {
		case 'x', 'X':
			digit, expChars = hexDigit, "pP"
			l.pos++
		case 'o', 'O', 'b', 'B':
			l.pos++
		}
	}

	for l.pos < len(l.src) && digit(l.src[l.pos]) {
		l.pos++
	}
	if l.peekByte(0) == '.' && isASCIIDigit(l.peekByte(1)) {
		l.pos++
		for l.pos < len(l.src) && digit(l.src[l.pos]) {
			l.pos++
		}
	}
	if c := l.peekByte(0); strings.IndexByte(expChars, c) >= 0 {
		off := 1
		if s := l.peekByte(off); s == '+' || s == '-' {
			off++
		}
		if isASCIIDigit(l.peekByte(off)) {
			l.pos += off
			for l.pos < len(l.src) && digit(l.src[l.pos]) {
				l.pos++
			}
		}
	}
	l.toks = append(l.toks, Token{Text: string(l.src[start:l.pos]), Line: startLine, Kind: Number})
}

func (l *lexer) scanOperatorRun() {
	start, startLine := l.pos, l.line
	for l.pos < len(l.src) && isOperatorByte(l.src[l.pos]) {
		if l.src[l.pos] == '/' && (l.peekByte(1) == '/' || l.peekByte(1) == '*') {
			break
		}
		l.pos++
	}
	if l.pos == start {
		l.pos++ // safety net against an infinite loop; shouldn't be reachable
	}
	l.toks = append(l.toks, Token{Text: string(l.src[start:l.pos]), Line: startLine, Kind: Operator})
}

func (l *lexer) scanLineComment() {
	start, startLine := l.pos, l.line
	for l.pos < len(l.src) && l.src[l.pos] != '\n' {
		l.pos++
	}
	l.toks = append(l.toks, Token{Text: string(l.src[start:l.pos]), Line: startLine, Kind: Comment})
}

func (l *lexer) startBlockComment() {
	l.frames = append(l.frames, frame{kind: frameComment, depth: 1, startLine: l.line, startPos: l.pos})
	l.pos += 2
}

func (l *lexer) scanInBlockComment() {
	top := &l.frames[len(l.frames)-1]
	switch {
	case l.peekByte(0) == '\n':
		l.line++
		l.pos++
	case l.peekByte(0) == '/' && l.peekByte(1) == '*':
		top.depth++
		l.pos += 2
	case l.peekByte(0) == '*' && l.peekByte(1) == '/':
		top.depth--
		l.pos += 2
		if top.depth == 0 {
			l.toks = append(l.toks, Token{Text: string(l.src[top.startPos:l.pos]), Line: top.startLine, Kind: Comment})
			l.frames = l.frames[:len(l.frames)-1]
		}
	default:
		l.advanceRune()
	}
}

// tryStartString attempts to begin a string literal at l.pos, which must
// be '"' or '#'. On success it pushes a frameString and returns true. On
// failure (a run of '#' not followed by a quote — e.g. "#available") it
// consumes nothing and returns false, so the caller falls back to
// emitting the '#' as Punct.
func (l *lexer) tryStartString() bool {
	start, startLine := l.pos, l.line
	hash := 0
	for l.peekByte(hash) == '#' {
		hash++
	}
	if l.peekByte(hash) != '"' {
		return false
	}
	multiline := l.peekByte(hash+1) == '"' && l.peekByte(hash+2) == '"'
	delimLen := hash + 1
	if multiline {
		delimLen = hash + 3
	}
	l.pos = start + delimLen
	l.frames = append(l.frames, frame{kind: frameString, hash: hash, multiline: multiline, startLine: startLine, startPos: start, topLevel: len(l.frames) == 0})
	return true
}

// matchInterpStart checks for '\' + N '#'s (matching hash) + '(' at l.pos,
// where l.src[l.pos] == '\\'. On success it consumes the whole sequence
// and returns true.
func (l *lexer) matchInterpStart(hash int) bool {
	off := 1
	for i := 0; i < hash; i++ {
		if l.peekByte(off) != '#' {
			return false
		}
		off++
	}
	if l.peekByte(off) != '(' {
		return false
	}
	l.pos += off + 1
	return true
}

// matchTerminator checks for this frame's closing delimiter at l.pos
// (a matching quote count plus N trailing '#'s) and consumes it on match.
func (l *lexer) matchTerminator(top *frame) bool {
	quoteLen := 1
	if top.multiline {
		if l.peekByte(0) != '"' || l.peekByte(1) != '"' || l.peekByte(2) != '"' {
			return false
		}
		quoteLen = 3
	} else if l.peekByte(0) != '"' {
		return false
	}
	off := quoteLen
	for i := 0; i < top.hash; i++ {
		if l.peekByte(off) != '#' {
			return false
		}
		off++
	}
	l.pos += off
	return true
}

func (l *lexer) scanInString() {
	top := &l.frames[len(l.frames)-1]
	switch {
	case l.src[l.pos] == '\n':
		l.line++
		l.pos++
	case l.src[l.pos] == '\\' && l.matchInterpStart(top.hash):
		l.frames = append(l.frames, frame{kind: frameInterp, depth: 1, startLine: l.line})
	case l.src[l.pos] == '\\':
		l.pos++ // backslash
		if l.pos < len(l.src) {
			l.advanceRune()
		}
	case l.matchTerminator(top):
		if top.topLevel {
			l.toks = append(l.toks, Token{Text: string(l.src[top.startPos:l.pos]), Line: top.startLine, Kind: String})
		}
		l.frames = l.frames[:len(l.frames)-1]
	default:
		l.advanceRune()
	}
}

func (l *lexer) scanInInterp() {
	top := &l.frames[len(l.frames)-1]
	switch c := l.src[l.pos]; c {
	case '\n':
		l.line++
		l.pos++
	case '"', '#':
		if !l.tryStartString() {
			l.pos++
		}
	case '(', '[', '{':
		top.depth++
		l.pos++
	case ')', ']', '}':
		top.depth--
		l.pos++
		if top.depth == 0 {
			l.frames = l.frames[:len(l.frames)-1]
		}
	default:
		l.advanceRune()
	}
}
