package crap

// ExitCode maps a Report's verdict to a process exit code, so callers don't
// duplicate the 0/1 convention at each call site.
func (r Report) ExitCode() int {
	if r.Passed {
		return 0
	}
	return 1
}
