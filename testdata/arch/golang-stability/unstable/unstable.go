// Package unstable is depended on by "stable" below but depends on "leaf"
// itself, and nothing else depends on it — high instability by
// construction, so "stable" importing it is a deliberate Stable
// Dependencies Principle violation for the fixture to catch.
package unstable

import "git.roost-r.com/cadeh/quality-gates/testdata/arch/golang-stability/leaf"

func Do() {
	leaf.Do()
}
