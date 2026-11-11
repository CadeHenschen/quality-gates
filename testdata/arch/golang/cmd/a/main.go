// Package a is a buildable package under cmd/a for the import-graph fixture.
// The analyzer uses source paths, so this still exercises cmd/b -> cmd/a while
// allowing whole-repo static analysis to compile the fixture tree.
package a
