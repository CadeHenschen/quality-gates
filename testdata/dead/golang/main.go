package main

import (
	"fmt"

	"example.com/deadfixture/util"
)

func main() {
	fmt.Println(util.Used())
}

// orphan is never called from main.
func orphan() int { return 1 }
