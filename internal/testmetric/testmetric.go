// Package testmetric grades the *tests* themselves, not the code under
// test. Coverage (crap-metric) says a line ran; it can't say whether
// anything checked the result — a test that calls a function and asserts
// nothing scores identically to a good one. This package turns per-test
// facts extracted by a language scanner (internal/testscanners) into
// findings for the failure modes a static pass can see precisely:
//
//   - skipped / focused / expected-failure: the test isn't really running
//     (an unconditional skip, an `.only` that silently disables the rest
//     of a suite, an xfail);
//   - no-assertions / low-assertions: the test can't fail;
//   - interaction-only: every assertion verifies a mock was called rather
//     than what came back or what state resulted — it pins the
//     implementation, not the behavior;
//   - temp-no-cleanup: the test creates a temp directory/file and nothing
//     in it or its file removes it.
//
// Deliberately *not* here: judging whether assertions are meaningful or
// whether a test is "too coupled to implementation" beyond the mock-only
// case. Those need semantic understanding a scanner can't have; mutation
// testing (internal/mutation) is the honest answer to "do these tests
// catch bugs", and is a separate gate.
package testmetric

import (
	"fmt"
	"sort"
)

// Test is one test (or, when Suite is set, one suite/describe block) and
// the facts a scanner extracted about it. The JSON tags double as the wire
// format the embedded Python/Node scanner scripts emit directly.
type Test struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Name string `json:"name"`
	// Suite marks a describe-level block: it only carries Skipped/Focused
	// and is excluded from test and assertion counts.
	Suite bool `json:"suite,omitempty"`
	// Assertions counts every assertion the test makes, including
	// interaction (mock-verifying) ones.
	Assertions int `json:"assertions"`
	// Interactions counts the subset of Assertions that only verify a mock
	// or spy was called.
	Interactions  int  `json:"interactions"`
	Skipped       bool `json:"skipped,omitempty"`
	Focused       bool `json:"focused,omitempty"`
	ExpectedFail  bool `json:"expected_failure,omitempty"`
	UncleanedTemp bool `json:"uncleaned_temp,omitempty"`
}

// Check names, as accepted by Options.Ignore and printed in reports.
const (
	KindSkipped         = "skipped"
	KindFocused         = "focused"
	KindExpectedFailure = "expected-failure"
	KindNoAssertions    = "no-assertions"
	KindLowAssertions   = "low-assertions"
	KindInteractionOnly = "interaction-only"
	KindTempNoCleanup   = "temp-no-cleanup"
)

// Kinds lists every check, for validating --ignore.
var Kinds = []string{
	KindSkipped, KindFocused, KindExpectedFailure, KindNoAssertions,
	KindLowAssertions, KindInteractionOnly, KindTempNoCleanup,
}

// OptIn lists the checks that are off unless asked for. interaction-only
// can't tell a mock that is a *collaborator* (verifying it pins the
// implementation) from a callback prop that is the component's *output*
// (`expect(onChange).toHaveBeenCalledWith(...)` is the behavior): run over a
// real React app's tests it flagged 345 of 1774 tests, nearly all of them
// legitimate. It's meaningful where mocks are collaborators, so it's opt-in.
var OptIn = []string{KindInteractionOnly}

// Finding is one problem in one test.
type Finding struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Test   string `json:"test"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

// Options tunes which findings are produced.
type Options struct {
	// MinAssertions is the assertion count below which a test is flagged;
	// 0 counts as no-assertions, anything in (0, MinAssertions) as
	// low-assertions. Values below 1 are treated as 1.
	MinAssertions int
	// Ignore disables the named checks.
	Ignore map[string]bool
}

// Analyze applies the checks to tests and returns findings in a stable
// file/line order.
func Analyze(tests []Test, opts Options) []Finding {
	minAsserts := opts.MinAssertions
	if minAsserts < 1 {
		minAsserts = 1
	}

	var out []Finding
	add := func(t Test, kind, detail string) {
		if opts.Ignore[kind] {
			return
		}
		out = append(out, Finding{File: t.File, Line: t.Line, Test: t.Name, Kind: kind, Detail: detail})
	}

	for _, t := range tests {
		if t.Skipped {
			add(t, KindSkipped, "unconditionally skipped — it never runs")
		}
		if t.Focused {
			add(t, KindFocused, "focused (.only/fit/fdescribe) — every non-focused test in the run is silently disabled")
		}
		if t.ExpectedFail {
			add(t, KindExpectedFailure, "marked expected-failure — a known-broken test is being tolerated")
		}
		if t.Suite || t.Skipped {
			// A skipped test's body never runs, so its assertion count says
			// nothing about it; don't double-report.
			continue
		}
		switch {
		case t.Assertions == 0:
			add(t, KindNoAssertions, "makes no assertions — it can't fail on wrong behavior")
		case t.Assertions < minAsserts:
			add(t, KindLowAssertions, fmt.Sprintf("%d assertion(s), below the minimum of %d", t.Assertions, minAsserts))
		case t.Interactions == t.Assertions:
			add(t, KindInteractionOnly, "every assertion only verifies a mock/spy call — nothing checks a returned value or resulting state")
		}
		if t.UncleanedTemp {
			add(t, KindTempNoCleanup, "creates a temp directory/file with no cleanup in the test or its file")
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// Counts returns the number of real tests (suites excluded) and their
// total assertions.
func Counts(tests []Test) (numTests, assertions int) {
	for _, t := range tests {
		if t.Suite {
			continue
		}
		numTests++
		assertions += t.Assertions
	}
	return numTests, assertions
}
