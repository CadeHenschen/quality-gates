// Package sample is a fixture used by cmd/crap-metric's size-flag tests —
// not part of the crap-metric build itself ("testdata" directories are
// ignored by the go tool).
package sample

// ManyParams exists purely to cross --max-params with a small --max-params
// value in tests; its body is trivial on purpose.
func ManyParams(a, b, c, d, e int) int {
	return a + b + c + d + e
}
