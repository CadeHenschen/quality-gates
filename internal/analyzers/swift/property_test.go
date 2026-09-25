package swift

import (
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/analyzers"
	"git.roost-r.com/cadeh/quality-gates/internal/crap"
	"git.roost-r.com/cadeh/quality-gates/internal/swiftlex"
)

// Regression coverage for the reported bug: a bare implicit-getter
// computed property (no "get"/"set" keyword at all, e.g.
// "var progress: Double { ... }") went completely unmeasured before —
// neither reported at 0% nor 100%, just absent — which is exactly what
// made a Swift package's crap-metric report look like "everything is
// 100% covered" even when a real, never-executed branch lived in a
// property. Uses its own fixture (testdata/crap/swift-property) so these
// line numbers don't ride on testdata/crap/swift/sample.swift's.
func analyzePropertyFixture(t *testing.T, coveragePath string) []crap.Function {
	t.Helper()
	fns, err := Analyzer{}.Analyze(analyzers.Options{
		Dir:          "../../../testdata/crap/swift-property",
		CoveragePath: coveragePath,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	return fns
}

func TestAnalyzeBareComputedPropertyEmitted(t *testing.T) {
	fns := analyzePropertyFixture(t, "")
	fn := findFunc(t, fns, "Snapshot.progress")
	// starts at 1, +1 for "&&" ("?:" is deliberately not counted) = 2.
	if fn.Complexity != 2 {
		t.Errorf("progress complexity = %d, want 2", fn.Complexity)
	}
	if fn.StartLine != 11 || fn.EndLine != 13 {
		t.Errorf("progress range = [%d,%d], want [11,13]", fn.StartLine, fn.EndLine)
	}
}

// A named setter parameter ("set(newScaled) { ... }") exercises the
// parenthesized-accessor-parameter path in scanPropertyAccessors, as
// opposed to bare "set" with the implicit "newValue".
func TestAnalyzeExplicitGetSetWithNamedSetterParam(t *testing.T) {
	fns := analyzePropertyFixture(t, "")
	get := findFunc(t, fns, "Snapshot.scaled.get")
	if get.Complexity != 1 {
		t.Errorf("scaled.get complexity = %d, want 1", get.Complexity)
	}
	set := findFunc(t, fns, "Snapshot.scaled.set")
	// starts at 1, +1 for "if" = 2.
	if set.Complexity != 2 {
		t.Errorf("scaled.set complexity = %d, want 2", set.Complexity)
	}
}

func TestAnalyzeStoredPropertyObserverEmitted(t *testing.T) {
	fns := analyzePropertyFixture(t, "")
	fn := findFunc(t, fns, "Snapshot.total.didSet")
	// starts at 1, +1 for "if" = 2.
	if fn.Complexity != 2 {
		t.Errorf("total.didSet complexity = %d, want 2", fn.Complexity)
	}
	for _, f := range fns {
		if f.Name == "Snapshot.total" {
			t.Errorf("a stored property's own declaration must not be its own entry, got %+v", f)
		}
	}
}

// A stored property's closure-literal default value is not an accessor
// block (no get/set/willSet/didSet keyword opens it) and must not be
// attributed to the property, nor corrupt the range of the function that
// follows it.
func TestAnalyzeStoredPropertyClosureDefaultNotEmitted(t *testing.T) {
	fns := analyzePropertyFixture(t, "")
	for _, f := range fns {
		if f.Name == "Snapshot.transform" {
			t.Errorf("closure-literal default value must not be its own entry, got %+v", f)
		}
	}
	fn := findFunc(t, fns, "Snapshot.plain")
	if fn.StartLine != 48 || fn.EndLine != 50 {
		t.Errorf("plain range = [%d,%d], want [48,50] (unaffected by the preceding stored property's closure default)", fn.StartLine, fn.EndLine)
	}
}

func TestAnalyzePropertyFixtureFunctionCount(t *testing.T) {
	fns := analyzePropertyFixture(t, "")
	// progress, scaled.get, scaled.set, total.didSet, plain.
	if len(fns) != 5 {
		t.Errorf("got %d functions, want 5; got %+v", len(fns), fns)
	}
}

// The real point of this whole fix: a genuinely uncovered line inside a
// computed property must actually surface as uncovered, not silently
// read as covered (or not read at all).
func TestAnalyzeComputedPropertyCoverageFromLCOV(t *testing.T) {
	fns := analyzePropertyFixture(t, "../../../testdata/crap/swift-property/coverage.lcov")

	progress := findFunc(t, fns, "Snapshot.progress")
	if progress.LinesTotal != 1 || progress.LinesCovered != 0 {
		t.Errorf("progress coverage = total %d covered %d, want total 1 covered 0", progress.LinesTotal, progress.LinesCovered)
	}
	if len(progress.UncoveredLines) != 1 || progress.UncoveredLines[0].Start != 12 {
		t.Errorf("progress UncoveredLines = %+v, want a single [12,12] range", progress.UncoveredLines)
	}

	didSet := findFunc(t, fns, "Snapshot.total.didSet")
	if didSet.LinesTotal != 1 || didSet.LinesCovered != 0 {
		t.Errorf("total.didSet coverage = total %d covered %d, want total 1 covered 0", didSet.LinesTotal, didSet.LinesCovered)
	}

	plain := findFunc(t, fns, "Snapshot.plain")
	if plain.LinesTotal != 1 || plain.LinesCovered != 1 {
		t.Errorf("plain coverage = total %d covered %d, want total 1 covered 1", plain.LinesTotal, plain.LinesCovered)
	}
}

// propertyName's fallback ("_") for a "var" not immediately followed by
// an identifier is a defensive path the outer walk never actually
// reaches in practice (see its doc comment) — exercised directly here
// rather than via a fixture that would need to contrive it.
func TestPropertyNameFallsBackWhenNoIdentFollows(t *testing.T) {
	toks := []swiftlex.Token{
		{Kind: swiftlex.Keyword, Text: "var", Line: 1},
		{Kind: swiftlex.Punct, Text: "(", Line: 1},
	}
	if got := propertyName(toks, 0); got != "_" {
		t.Errorf("propertyName = %q, want %q", got, "_")
	}
	if got := propertyName(toks, 1); got != "_" {
		t.Errorf("propertyName at end of stream = %q, want %q", got, "_")
	}
}
