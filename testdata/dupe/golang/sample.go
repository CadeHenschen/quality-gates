// Package sample is a fixture with a deliberate duplicate block, used by
// internal/tokenizers/golang and internal/dupe's tests.
package sample

func ProcessOrder(items []int) int {
	total := 0
	for _, v := range items {
		if v > 0 && v < 1000 {
			total += v
		}
	}
	return total
}

func ProcessInvoice(items []int) int {
	total := 0
	for _, v := range items {
		if v > 0 && v < 1000 {
			total += v
		}
	}
	return total
}

func Unrelated() string {
	return "nothing shared here"
}
