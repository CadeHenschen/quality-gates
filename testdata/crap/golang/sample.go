// Package sample is a fixture used by internal/analyzers/golang's tests —
// not part of the crap-metric build itself ("testdata" directories are
// ignored by the go tool).
package sample

func Simple() int {
	return 1
}

func Branchy(x int) int {
	if x > 0 {
		return x
	}
	return -x
}

func Loopy(items []int) int {
	total := 0
	for _, v := range items {
		if v > 0 && v < 100 {
			total += v
		}
	}
	return total
}
