package crap

import "git.roost-r.com/cadeh/quality-gates/internal/reportio"

// ExitCode maps a Report's verdict to a process exit code, so callers don't
// duplicate the 0/1 convention at each call site.
func (r Report) ExitCode() int {
	return reportio.ExitCode(r.Passed)
}
