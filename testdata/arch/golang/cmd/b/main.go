// Package main is cmd/b — it imports the buildable package in cmd/a so the
// rules file's second rule ("cmd binaries are independent") has an import edge
// to catch. The importer still derives the edge from the source path; keeping
// both packages buildable also lets whole-repo static analysis scan this tree.
package main

import _ "git.roost-r.com/cadeh/quality-gates/testdata/arch/golang/cmd/a"

func main() {}
