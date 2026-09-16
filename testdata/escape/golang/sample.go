package sample

import "fmt"

func Clean(x int) int {
	return x * 2
}

func WithNolint(x int) int {
	y := x / 0 //nolint
	return y
}

func WithDiscardedResult() {
	_ = fmt.Sprintf("hi")
}

func WithNosec(cmd string) {
	fmt.Println(cmd) // #nosec G204
}
