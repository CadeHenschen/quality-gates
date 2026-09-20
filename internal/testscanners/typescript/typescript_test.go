package typescript

import (
	"os/exec"
	"path/filepath"
	"testing"

	"git.roost-r.com/cadeh/quality-gates/internal/testmetric"
)

func scanFixture(t *testing.T) []testmetric.Test {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not on PATH")
	}
	tests, err := Scanner{}.Scan(filepath.Join("..", "..", "..", "testdata", "test", "typescript"))
	if err != nil {
		t.Fatalf("Scan: %v (run `npm install` in testdata/test/typescript if typescript is missing)", err)
	}
	return tests
}

func byName(tests []testmetric.Test) map[string]testmetric.Test {
	m := map[string]testmetric.Test{}
	for _, tc := range tests {
		m[tc.Name] = tc
	}
	return m
}

func TestFilesAreDirRelative(t *testing.T) {
	for _, tc := range scanFixture(t) {
		if tc.File != "sample.test.ts" && tc.File != "hooked.spec.js" {
			t.Errorf("File = %q, want a --dir-relative name", tc.File)
		}
	}
}

func TestAssertionCounting(t *testing.T) {
	got := byName(scanFixture(t))
	want := map[string]struct{ asserts, interactions int }{
		"asserts a value":       {1, 0},
		"has no assertions":     {0, 0},
		"delegates to a helper": {1, 0},
		"uses node assert":      {2, 0},
		"only verifies mocks":   {2, 2},
		"mock plus behavior":    {2, 1},
		"table %i":              {1, 0},
		"declares expect.assertions but asserts nothing else": {0, 0},
		"top-level test": {1, 0},
	}
	for name, w := range want {
		tc, ok := got[name]
		if !ok {
			t.Errorf("%q not found", name)
			continue
		}
		if tc.Assertions != w.asserts || tc.Interactions != w.interactions {
			t.Errorf("%q: assertions/interactions = %d/%d, want %d/%d",
				name, tc.Assertions, tc.Interactions, w.asserts, w.interactions)
		}
	}
}

func TestSkipAndFocusDetection(t *testing.T) {
	got := byName(scanFixture(t))
	for _, name := range []string{"is skipped", "is x-skipped", "write this later", "skipped suite"} {
		if !got[name].Skipped {
			t.Errorf("%q should be Skipped", name)
		}
	}
	if got["conditional skip is fine"].Skipped {
		t.Error("test.skipIf is conditional and must not count as Skipped")
	}
	for _, name := range []string{"is focused", "focused suite"} {
		if !got[name].Focused {
			t.Errorf("%q should be Focused", name)
		}
	}
	for _, name := range []string{"skipped suite", "focused suite"} {
		if !got[name].Suite {
			t.Errorf("%q should be a Suite", name)
		}
	}
	if _, ok := got["good suite"]; ok {
		t.Error("an ordinary describe should not be recorded at all")
	}
}

func TestTempCleanupDetection(t *testing.T) {
	got := byName(scanFixture(t))
	if !got["leaks a temp dir"].UncleanedTemp {
		t.Error("mkdtempSync with no cleanup should be flagged")
	}
	for _, name := range []string{"cleans a temp dir", "temp dir cleaned by a file-level hook"} {
		if got[name].UncleanedTemp {
			t.Errorf("%q should not be flagged for temp cleanup", name)
		}
	}
}
