// Package stable is depended on by two consumers below and depends on
// nothing else in this fixture — low instability by construction. Its
// import of "unstable" is the fixture's deliberate violation: a stable
// package depending on a less stable one.
package stable

import "git.roost-r.com/cadeh/quality-gates/testdata/arch/golang-stability/unstable"

func Do() {
	unstable.Do()
}
