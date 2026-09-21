package util

func Used() string { return helper() }

func helper() string { return "ok" }

// Unused is exported but nothing reachable from main calls it.
func Unused() string { return "dead" }

type Thing struct{}

// Method is never called.
func (Thing) Method() {}
