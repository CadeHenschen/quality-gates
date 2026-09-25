package mutation

import (
	"fmt"
	"io"
	"sort"

	"git.roost-r.com/cadeh/quality-gates/internal/reportio"
)

// Report is a mutation-score verdict over a set of mutants.
type Report struct {
	Mutants []Mutant `json:"mutants"`
	// Detected counts Killed + Timeout; Survived and NoCoverage are the
	// misses; Ignored mutants (no verdict) are in Mutants but in no count.
	Detected   int `json:"detected"`
	Survived   int `json:"survived"`
	NoCoverage int `json:"no_coverage"`
	// Score is Detected / (Detected + Survived + NoCoverage), as a
	// percentage; 100 when nothing has a verdict (nothing escaped).
	Score     float64 `json:"score"`
	FailBelow float64 `json:"fail_below"`
	Passed    bool    `json:"passed"`
	// CoveredOnly records that NoCoverage mutants were left out of the
	// score's denominator, so it measures assertion strength only — line
	// coverage is crap-metric's job.
	CoveredOnly bool `json:"covered_only"`
	// Graded is the score's denominator: how many mutants got a verdict
	// that counts toward it (so NoCoverage is out under CoveredOnly).
	Graded int `json:"graded"`
	// MinMutants and Insufficient record the --min-mutants waiver: with
	// fewer than MinMutants graded, the score is noise (one survivor among
	// three is 67%) and the gate passes rather than fail on chance.
	MinMutants           int                `json:"min_mutants,omitempty"`
	Insufficient         bool               `json:"insufficient,omitempty"`
	MinimumGraded        int                `json:"minimum_graded,omitempty"`
	InsufficientEvidence bool               `json:"insufficient_evidence,omitempty"`
	Analysis             *reportio.Analysis `json:"analysis,omitempty"`
}

// NewReport tallies mutants and applies the fail-below gate (a percentage).
// Because the numerator and denominator both come from the same mutant
// list, a ratcheted (--only-files) report scopes down naturally — unlike a
// per-line rate, there's no separate denominator to keep in step.
func NewReport(mutants []Mutant, failBelow float64, coveredOnly bool) Report {
	r := Report{Mutants: mutants, FailBelow: failBelow, CoveredOnly: coveredOnly}
	for _, m := range mutants {
		switch m.Status {
		case Killed, Timeout:
			r.Detected++
		case Survived:
			r.Survived++
		case NoCoverage:
			r.NoCoverage++
		}
	}
	denom := r.Detected + r.Survived
	if !coveredOnly {
		denom += r.NoCoverage
	}
	r.Graded = denom
	r.Score = 100
	if denom > 0 {
		r.Score = float64(r.Detected) / float64(denom) * 100
	}
	r.Passed = r.Score >= failBelow
	return r
}

// WithMinMutants applies the --min-mutants waiver: when fewer than n mutants
// were graded the score is too noisy to gate on, so the report passes and
// says why. n <= 0 disables it. Kept apart from NewReport so the score maths
// stays a single tally; the ratchet's scoped report calls it too.
func (r Report) WithMinMutants(n int) Report {
	r.MinMutants = n
	if n > 0 && r.Graded < n {
		r.Insufficient = true
		r.Passed = true
	}
	return r
}

// WithMinimumGraded requires enough scored mutants to make a score
// meaningful. Unlike the legacy --min-mutants waiver, this fails closed.
func (r Report) WithMinimumGraded(n int) Report {
	r.MinimumGraded = n
	if n > 0 && r.Graded < n {
		r.InsufficientEvidence = true
		r.Passed = false
	}
	return r
}

// WithEvidence attaches per-file analysis evidence to a ratcheted score.
func (r Report) WithEvidence(analysis reportio.Analysis) Report {
	r.Analysis = &analysis
	if !analysis.Passed {
		r.Passed = false
		r.InsufficientEvidence = true
	}
	return r
}

// ExitCode maps a Report's verdict to a process exit code.
func (r Report) ExitCode() int { return reportio.ExitCode(r.Passed) }

// WriteJSON writes the full report as JSON.
func (r Report) WriteJSON(w io.Writer) error { return reportio.WriteJSON(w, r) }

// WriteTable writes the score and the surviving mutants — the actionable
// part: each is a bug the tests would have let ship.
func (r Report) WriteTable(w io.Writer, top int) {
	fmt.Fprintf(w, "mutation score %.1f%%: %d detected, %d survived, %d not covered (%d mutants)\n\n",
		r.Score, r.Detected, r.Survived, r.NoCoverage, len(r.Mutants))

	var misses []Mutant
	for _, m := range r.Mutants {
		if m.Status == Survived || (m.Status == NoCoverage && !r.CoveredOnly) {
			misses = append(misses, m)
		}
	}
	sort.Slice(misses, func(i, j int) bool {
		if misses[i].File != misses[j].File {
			return misses[i].File < misses[j].File
		}
		return misses[i].Line < misses[j].Line
	})

	truncated := 0
	if top > 0 && len(misses) > top {
		truncated = len(misses) - top
		misses = misses[:top]
	}
	for _, m := range misses {
		fmt.Fprintf(w, "%s:%d  %-11s %s\n", m.File, m.Line, m.Status, m.Mutator)
	}
	if truncated > 0 {
		fmt.Fprintf(w, "... %d more not shown\n", truncated)
	}
	if len(misses) > 0 {
		fmt.Fprintln(w)
	}

	if r.Analysis != nil && !r.Analysis.Passed {
		fmt.Fprintf(w, "FAIL: %s (%v)\n", r.Analysis.Reason, r.Analysis.Missing)
	} else if r.InsufficientEvidence {
		fmt.Fprintf(w, "FAIL: insufficient analysis: %d graded mutants, need at least %d\n", r.Graded, r.MinimumGraded)
	} else if r.Insufficient {
		fmt.Fprintf(w, "PASS: only %d graded mutants (fewer than --min-mutants %d), so the %.1f%% score is not enforced\n", r.Graded, r.MinMutants, r.Score)
	} else if r.Passed {
		fmt.Fprintf(w, "PASS: %.1f%% is at or above %.1f%%\n", r.Score, r.FailBelow)
	} else {
		fmt.Fprintf(w, "FAIL: %.1f%% is below %.1f%%\n", r.Score, r.FailBelow)
	}
}
