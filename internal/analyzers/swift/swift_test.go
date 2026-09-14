package swift

import (
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/analyzers"
	"git.roost-r.com/cadeh/quality-gates/internal/crap"
)

func analyzeFixture(t *testing.T, coveragePath string) []crap.Function {
	t.Helper()
	fns, err := Analyzer{}.Analyze(analyzers.Options{
		Dir:          "../../../testdata/crap/swift",
		CoveragePath: coveragePath,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	return fns
}

func findFunc(t *testing.T, fns []crap.Function, name string) crap.Function {
	t.Helper()
	for _, fn := range fns {
		if fn.Name == name {
			return fn
		}
	}
	t.Fatalf("no function named %q in %+v", name, fns)
	return crap.Function{}
}

func TestAnalyzeSkipsTestFiles(t *testing.T) {
	fns := analyzeFixture(t, "")
	for _, fn := range fns {
		if fn.File == "CalculatorTests.swift" {
			t.Errorf("CalculatorTests.swift should be skipped (test-file suffix), got %+v", fn)
		}
		if fn.Name == "helperFunc" {
			t.Errorf("Tests/Helper.swift should be skipped (Tests/ dir), got %+v", fn)
		}
	}
}

func TestAnalyzeFileIsDirRelative(t *testing.T) {
	fns := analyzeFixture(t, "")
	for _, fn := range fns {
		if fn.File != "sample.swift" {
			t.Errorf("File = %q, want dir-relative %q", fn.File, "sample.swift")
		}
	}
}

func TestAnalyzeSimpleComplexity(t *testing.T) {
	fns := analyzeFixture(t, "")
	fn := findFunc(t, fns, "Calculator.classify")
	// starts at 1, +1 for "if", +1 for "&&" = 3.
	if fn.Complexity != 3 {
		t.Errorf("classify complexity = %d, want 3", fn.Complexity)
	}
	if fn.StartLine != 7 || fn.EndLine != 12 {
		t.Errorf("classify range = [%d,%d], want [7,12]", fn.StartLine, fn.EndLine)
	}
}

// The sharpest correctness case per the package doc comment: a nested
// local func must fold its branches into the enclosing function and must
// not be emitted as its own crap.Function.
func TestAnalyzeNestedLocalFuncFoldsIntoEnclosing(t *testing.T) {
	fns := analyzeFixture(t, "")
	for _, fn := range fns {
		if fn.Name == "double" || fn.Name == "Calculator.double" {
			t.Fatalf("nested local func must not be emitted as its own entry, got %+v", fn)
		}
	}
	fn := findFunc(t, fns, "Calculator.withNestedHelper")
	// starts at 1, +1 for the enclosing "if", +1 for the nested func's own
	// "if" (folded in) = 3.
	if fn.Complexity != 3 {
		t.Errorf("withNestedHelper complexity = %d, want 3 (enclosing if + nested func's folded-in if)", fn.Complexity)
	}
}

// Computed-property accessors must not appear as their own entries, and
// must not corrupt the line ranges of functions before/after them.
func TestAnalyzeComputedPropertyNotEmitted(t *testing.T) {
	fns := analyzeFixture(t, "")
	for _, fn := range fns {
		if fn.Name == "Calculator.doubled" || fn.Name == "Calculator.get" || fn.Name == "Calculator.set" {
			t.Errorf("computed property accessor should not be its own entry, got %+v", fn)
		}
	}
	fn := findFunc(t, fns, "Calculator.plain")
	if fn.StartLine != 46 || fn.EndLine != 48 {
		t.Errorf("plain range = [%d,%d], want [46,48] (unaffected by the preceding computed property)", fn.StartLine, fn.EndLine)
	}
}

func TestAnalyzeExtensionQualifiesName(t *testing.T) {
	fns := analyzeFixture(t, "")
	findFunc(t, fns, "Calculator.reset")
}

func TestAnalyzeCoverageFromLCOV(t *testing.T) {
	fns := analyzeFixture(t, "../../../testdata/crap/swift/coverage.lcov")
	fn := findFunc(t, fns, "Calculator.classify")
	// DA:8,5 DA:9,3 DA:11,0 fall within [7,12]: 3 measured lines, 2 covered,
	// line 11 uncovered.
	if fn.LinesTotal != 3 || fn.LinesCovered != 2 {
		t.Errorf("classify coverage = total %d covered %d, want total 3 covered 2", fn.LinesTotal, fn.LinesCovered)
	}
	if len(fn.UncoveredLines) != 1 || fn.UncoveredLines[0].Start != 11 || fn.UncoveredLines[0].End != 11 {
		t.Errorf("classify UncoveredLines = %+v, want a single [11,11] range", fn.UncoveredLines)
	}

	plain := findFunc(t, fns, "Calculator.plain")
	if plain.LinesTotal != 1 || plain.LinesCovered != 1 {
		t.Errorf("plain coverage = total %d covered %d, want total 1 covered 1", plain.LinesTotal, plain.LinesCovered)
	}
}

func TestAnalyzeEmptyDirNoError(t *testing.T) {
	fns, err := Analyzer{}.Analyze(analyzers.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(fns) != 0 {
		t.Errorf("got %d functions from an empty dir, want 0", len(fns))
	}
}
