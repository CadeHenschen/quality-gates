// Package domain is this fixture's domain layer — deliberately reaching
// into infra below, so the rules file's first rule has something real to
// catch.
package domain

import "git.roost-r.com/cadeh/quality-gates/testdata/arch/golang/infra"

func UseInfra() {
	infra.Thing()
}
