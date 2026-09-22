// Package consumer2 is the second of two importers of "stable" — see
// consumer1's doc comment.
package consumer2

import "git.roost-r.com/cadeh/quality-gates/testdata/arch/golang-stability/stable"

func Do() {
	stable.Do()
}
