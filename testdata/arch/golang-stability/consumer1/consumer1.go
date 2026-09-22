// Package consumer1 is one of two importers of "stable", lowering its
// instability (raising its afferent coupling) enough to make the
// stable -> unstable edge a real violation.
package consumer1

import "git.roost-r.com/cadeh/quality-gates/testdata/arch/golang-stability/stable"

func Do() {
	stable.Do()
}
