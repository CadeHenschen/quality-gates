package swift

import (
	"path/filepath"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/testmetric"
)

func scanFixture(t *testing.T) map[string]testmetric.Test {
	t.Helper()
	// A --dir with ".." components must still yield --dir-relative files.
	tests, err := Scanner{}.Scan(filepath.Join("..", "..", "..", "testdata", "test", "swift"))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	byName := map[string]testmetric.Test{}
	for _, tc := range tests {
		if filepath.IsAbs(tc.File) || filepath.Dir(tc.File) != "." {
			t.Errorf("File = %q, want a bare --dir-relative name", tc.File)
		}
		byName[tc.Name] = tc
	}
	return byName
}

func TestScanFindsOnlyRealTests(t *testing.T) {
	got := scanFixture(t)
	for _, notATest := range []string{"testHasParametersIsNotATest", "helperNotATest", "notATestWithoutAttribute", "assertBalanced", "locate", "makeFixture", "inner"} {
		if _, ok := got[notATest]; ok {
			t.Errorf("%s should not be treated as a test", notATest)
		}
	}
	// 14 + 1 XCTest (incl. TeardownTests) and 7 Swift Testing.
	if len(got) != 22 {
		t.Errorf("found %d tests, want 22", len(got))
	}
}

func TestAssertionCounting(t *testing.T) {
	got := scanFixture(t)
	want := map[string]int{
		"testGood":                         2,
		"testNoAssertions":                 0, // its comment mentions XCTAssertEqual( but isn't code
		"testSetupHelperIsNotAnAssertion":  0,
		"testDelegatesToNamedHelper":       1,
		"testDelegatesToUnnamedHelper":     1,
		"testDelegatesToHelperOfHelper":    1,
		"testExpectationIsNotAnAssertion":  0,
		"testNestedLocalFuncFoldsIntoTest": 1,
		"good":                             1,
		"noAssertions":                     0,
		"requireCounts":                    1,
		"issueRecordCounts":                1,
	}
	for name, w := range want {
		tc, ok := got[name]
		if !ok {
			t.Errorf("%s not found", name)
			continue
		}
		if tc.Assertions != w {
			t.Errorf("%s: assertions = %d, want %d", name, tc.Assertions, w)
		}
	}
}

func TestSkipAndExpectedFailure(t *testing.T) {
	got := scanFixture(t)
	for _, name := range []string{"testUnconditionalSkip", "disabled"} {
		if !got[name].Skipped {
			t.Errorf("%s should be Skipped", name)
		}
	}
	for _, name := range []string{"testConditionalSkipIsFine", "testSkipIfIsFine", "conditionallyEnabledIsFine", "testGood"} {
		if got[name].Skipped {
			t.Errorf("%s should not be Skipped (guards are environment checks)", name)
		}
	}
	for _, name := range []string{"testExpectedFailure", "knownIssue"} {
		if !got[name].ExpectedFail {
			t.Errorf("%s should be ExpectedFail", name)
		}
	}
}

func TestTempCleanupDetection(t *testing.T) {
	got := scanFixture(t)
	if !got["testTempNoCleanup"].UncleanedTemp {
		t.Error("a temp directory created with no cleanup should be flagged")
	}
	for _, name := range []string{"testTempWithCleanup", "testTempCleanedByTearDown", "testGood"} {
		if got[name].UncleanedTemp {
			t.Errorf("%s should not be flagged for temp cleanup", name)
		}
	}
}
