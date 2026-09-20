package golang

import (
	"path/filepath"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/testmetric"
)

func scanFixture(t *testing.T) map[string]testmetric.Test {
	t.Helper()
	// A --dir with ".." components must still yield --dir-relative files.
	tests, err := Scanner{}.Scan(filepath.Join("..", "..", "..", "testdata", "test", "golang"))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	byName := map[string]testmetric.Test{}
	for _, tc := range tests {
		if tc.File != "sample_test.go" {
			t.Errorf("File = %q, want dir-relative %q", tc.File, "sample_test.go")
		}
		byName[tc.Name] = tc
	}
	return byName
}

func TestScanFindsOnlyRealTestFunctions(t *testing.T) {
	got := scanFixture(t)
	for _, notATest := range []string{"Testlowercase", "TestHelperNotATest", "BenchmarkNotATest"} {
		if _, ok := got[notATest]; ok {
			t.Errorf("%s should not be treated as a test", notATest)
		}
	}
	if len(got) != 16 {
		t.Errorf("found %d tests, want 16", len(got))
	}
}

func TestAssertionCounting(t *testing.T) {
	got := scanFixture(t)
	want := map[string]struct{ asserts, interactions int }{
		"TestGood":                            {2, 0},
		"TestNoAssertions":                    {0, 0},
		"TestSetupHelperOnlyIsNotAnAssertion": {0, 0},
		"TestDelegatesToHelper":               {1, 0},
		"TestAssertsViaUnnamedHelper":         {1, 0},
		"TestAssertsViaHelperOfHelper":        {1, 0},
		"TestSubtests":                        {1, 0},
		"TestOnlyMockAssertions":              {2, 2},
		"TestMockPlusBehavior":                {2, 1},
		"TestConditionalSkipIsFine":           {1, 0},
	}
	for name, w := range want {
		tc, ok := got[name]
		if !ok {
			t.Errorf("%s not found", name)
			continue
		}
		if tc.Assertions != w.asserts || tc.Interactions != w.interactions {
			t.Errorf("%s: assertions/interactions = %d/%d, want %d/%d",
				name, tc.Assertions, tc.Interactions, w.asserts, w.interactions)
		}
	}
}

func TestSkipOnlyWhenUnconditional(t *testing.T) {
	got := scanFixture(t)
	if !got["TestUnconditionalSkip"].Skipped {
		t.Error("an unconditional t.Skip should mark the test Skipped")
	}
	if got["TestSwitchSkipIsConditional"].Skipped {
		t.Error("a t.Skip inside a switch case is conditional too")
	}
	if got["TestSelectAndTypeSwitchDontCrash"].Assertions != 2 {
		t.Errorf("assertions inside type-switch/select = %d, want 2", got["TestSelectAndTypeSwitchDontCrash"].Assertions)
	}
	if got["TestConditionalSkipIsFine"].Skipped {
		t.Error("a t.Skip inside an if is an environment guard, not a disabled test")
	}
}

func TestTempCleanupDetection(t *testing.T) {
	got := scanFixture(t)
	if !got["TestTempNoCleanup"].UncleanedTemp {
		t.Error("MkdirTemp with no cleanup should be flagged")
	}
	for _, ok := range []string{"TestTempWithDefer", "TestTempWithCleanup", "TestGood"} {
		if got[ok].UncleanedTemp {
			t.Errorf("%s should not be flagged for temp cleanup", ok)
		}
	}
}
