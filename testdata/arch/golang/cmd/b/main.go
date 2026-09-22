// Package main is cmd/b — deliberately importing cmd/a, so the rules
// file's second rule ("cmd binaries are independent") has something real
// to catch. This wouldn't build as a real Go program (a package main
// isn't meant to be imported), but the importer only ever parses imports,
// never type-checks or builds a package graph, so that's not a fixture
// requirement here — see internal/importers/golang's package doc.
package main

import _ "git.roost-r.com/cadeh/quality-gates/testdata/arch/golang/cmd/a"

func main() {}
