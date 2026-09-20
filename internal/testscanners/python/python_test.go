package python

import (
	"os/exec"
	"path/filepath"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/testmetric"
)

func scanFixture(t *testing.T) map[string]testmetric.Test {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
	tests, err := Scanner{}.Scan(filepath.Join("..", "..", "..", "testdata", "test", "python"))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	byName := map[string]testmetric.Test{}
	for _, tc := range tests {
		if tc.File != "test_sample.py" {
			t.Errorf("File = %q, want dir-relative %q", tc.File, "test_sample.py")
		}
		byName[tc.Name] = tc
	}
	return byName
}

func TestScanFindsTestsNotHelpers(t *testing.T) {
	got := scanFixture(t)
	if _, ok := got["helper_not_a_test"]; ok {
		t.Error("a non-test_ function should not be scanned")
	}
	for _, name := range []string{"test_good", "TestThing.test_method", "TestSkippedClass.test_a", "TestCaseStyle.test_assert_style"} {
		if _, ok := got[name]; !ok {
			t.Errorf("%s not found", name)
		}
	}
}

func TestAssertionCounting(t *testing.T) {
	got := scanFixture(t)
	want := map[string]struct{ asserts, interactions int }{
		"test_good":                             {1, 0},
		"test_no_assertions":                    {0, 0},
		"test_setup_helper_is_not_an_assertion": {0, 0},
		"test_delegates_to_helper":              {1, 0},
		"test_raises":                           {1, 0},
		"test_only_mock_assertions":             {2, 2},
		"test_mock_plus_behavior":               {2, 1},
		"TestThing.test_method_no_asserts":      {0, 0},
		"TestCaseStyle.test_assert_style":       {2, 0},
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

func TestSkipAndXfailDetection(t *testing.T) {
	got := scanFixture(t)
	for _, name := range []string{"test_unconditional_skip", "test_skip_call", "TestSkippedClass.test_a"} {
		if !got[name].Skipped {
			t.Errorf("%s should be Skipped", name)
		}
	}
	for _, name := range []string{"test_conditional_skip_is_fine", "test_skip_call_in_guard_is_fine", "test_good"} {
		if got[name].Skipped {
			t.Errorf("%s should not be Skipped (conditional skips are environment guards)", name)
		}
	}
	if !got["test_expected_failure"].ExpectedFail {
		t.Error("@pytest.mark.xfail should mark ExpectedFail")
	}
}

func TestTempCleanupDetection(t *testing.T) {
	got := scanFixture(t)
	if !got["test_temp_no_cleanup"].UncleanedTemp {
		t.Error("mkdtemp with no cleanup should be flagged")
	}
	for _, ok := range []string{"test_temp_with_cleanup", "test_temp_with_finally", "test_good"} {
		if got[ok].UncleanedTemp {
			t.Errorf("%s should not be flagged for temp cleanup", ok)
		}
	}
}
