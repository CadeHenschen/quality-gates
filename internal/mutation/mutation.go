// Package mutation grades a test suite by mutation score: how many
// deliberately-injected bugs (a `<` flipped to `<=`, a `+` to a `-`) the
// suite noticed. It is the honest answer to "do these tests assert
// behavior?" — a test can score full coverage and still let every mutant
// survive.
//
// Like crap-metric with coverage, this package does *not* run the mutation
// tool: mutating and re-running a suite is slow, language-specific, and
// already done well by tools the target repo runs in its own CI
// (go-gremlins for Go, Stryker for TS/JS, swift-mutation-testing for Swift).
// It parses their JSON reports into one normalized model and gates on the
// score, with the same
// --only-files ratchet as every other tool here. The generic format (see
// Parse) exists so a small converter over tools without compatible JSON can
// feed the same gate.
package mutation

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// Status is a mutant's normalized outcome across every source format.
type Status string

const (
	Killed     Status = "killed"      // a test failed: the bug was caught
	Crashed    Status = "crashed"     // the mutant caused the test process to crash
	Timeout    Status = "timeout"     // suite hung on the mutant: counted as caught
	Survived   Status = "survived"    // every test still passed: the bug was missed
	NoCoverage Status = "no_coverage" // no test ever executed the mutated line
	Unviable   Status = "unviable"    // the mutation could not be built/applied
	Ignored    Status = "ignored"     // source format reported no verdict without a more specific status
)

// Mutant is one injected bug and what the suite did about it.
type Mutant struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Mutator string `json:"mutator"`
	Status  Status `json:"status"`
}

// Options controls Parse.
type Options struct {
	// Dir, when set, relativizes absolute paths in the report to it, so
	// File is always --dir-relative (see CLAUDE.md on ratchet matching).
	Dir string
}

// Parse reads a mutation report and detects its format: a Stryker
// mutation-testing-elements JSON (`files` is an object; also used by
// swift-mutation-testing), a go-gremlins `--output` JSON (`files` is an
// array), or the generic `{"mutants":[{"file","line","mutator","status"}]}`
// shape where status is one of killed|crashed|timeout|survived|no_coverage|
// unviable|ignored.
func Parse(data []byte, opts Options) ([]Mutant, error) {
	var probe struct {
		Files   json.RawMessage `json:"files"`
		Mutants json.RawMessage `json:"mutants"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("parse mutation report: %w", err)
	}

	var mutants []Mutant
	var err error
	switch {
	case len(probe.Mutants) > 0:
		mutants, err = parseGeneric(probe.Mutants)
	case len(probe.Files) > 0 && probe.Files[0] == '{':
		mutants, err = parseStryker(probe.Files)
	case len(probe.Files) > 0 && probe.Files[0] == '[':
		mutants, err = parseGremlins(probe.Files)
	default:
		return nil, fmt.Errorf("unrecognized mutation report: want a Stryker or go-gremlins JSON report, or the generic {\"mutants\": [...]} format")
	}
	if err != nil {
		return nil, err
	}

	for i := range mutants {
		mutants[i].File = normalizePath(mutants[i].File, opts.Dir)
	}
	return mutants, nil
}

func normalizePath(file, dir string) string {
	if dir != "" && filepath.IsAbs(file) {
		if absDir, err := filepath.Abs(dir); err == nil {
			if rel, err := filepath.Rel(absDir, file); err == nil {
				file = rel
			}
		}
	}
	return strings.TrimPrefix(filepath.ToSlash(file), "./")
}

func parseGeneric(raw json.RawMessage) ([]Mutant, error) {
	var in []struct {
		File    string `json:"file"`
		Line    int    `json:"line"`
		Mutator string `json:"mutator"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("parse generic mutation report: %w", err)
	}
	out := make([]Mutant, 0, len(in))
	for _, m := range in {
		s := Status(strings.ToLower(m.Status))
		switch s {
		case Killed, Crashed, Timeout, Survived, NoCoverage, Unviable, Ignored:
		default:
			return nil, fmt.Errorf("generic mutation report: unknown status %q for %s:%d", m.Status, m.File, m.Line)
		}
		out = append(out, Mutant{File: m.File, Line: m.Line, Mutator: m.Mutator, Status: s})
	}
	return out, nil
}

func parseStryker(raw json.RawMessage) ([]Mutant, error) {
	var files map[string]struct {
		Mutants []struct {
			MutatorName string `json:"mutatorName"`
			Status      string `json:"status"`
			Location    struct {
				Start struct {
					Line int `json:"line"`
				} `json:"start"`
			} `json:"location"`
		} `json:"mutants"`
	}
	if err := json.Unmarshal(raw, &files); err != nil {
		return nil, fmt.Errorf("parse Stryker report: %w", err)
	}
	var out []Mutant
	for file, f := range files {
		for _, m := range f.Mutants {
			out = append(out, Mutant{File: file, Line: m.Location.Start.Line, Mutator: m.MutatorName, Status: strykerStatus(m.Status)})
		}
	}
	return out, nil
}

// strykerStatus maps Stryker and swift-mutation-testing status names.
// Unviable mutations have no verdict, while Crash is a detected mutation:
// the mutant caused an observable failure in the test process.
func strykerStatus(s string) Status {
	switch s {
	case "Killed":
		return Killed
	case "Crash":
		return Crashed
	case "Timeout":
		return Timeout
	case "Survived":
		return Survived
	case "NoCoverage":
		return NoCoverage
	case "Unviable":
		return Unviable
	}
	return Ignored
}

func parseGremlins(raw json.RawMessage) ([]Mutant, error) {
	var files []struct {
		FileName  string `json:"file_name"`
		Mutations []struct {
			Line   int    `json:"line"`
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"mutations"`
	}
	if err := json.Unmarshal(raw, &files); err != nil {
		return nil, fmt.Errorf("parse go-gremlins report: %w", err)
	}
	var out []Mutant
	for _, f := range files {
		for _, m := range f.Mutations {
			out = append(out, Mutant{File: f.FileName, Line: m.Line, Mutator: m.Type, Status: gremlinsStatus(m.Status)})
		}
	}
	return out, nil
}

// gremlinsStatus maps go-gremlins' status names; NOT VIABLE, SKIPPED, and
// RUNNABLE (a dry run) carry no verdict.
func gremlinsStatus(s string) Status {
	switch strings.ToUpper(s) {
	case "KILLED":
		return Killed
	case "TIMED OUT":
		return Timeout
	case "LIVED":
		return Survived
	case "NOT COVERED":
		return NoCoverage
	}
	return Ignored
}
