package reportio

import (
	"strings"
	"testing"
)

func TestAnalysisTextExplainsEvidenceAndNotApplicableScope(t *testing.T) {
	var out strings.Builder
	Analysis{Passed: true}.WriteText(&out)
	if !strings.Contains(out.String(), "not applicable") {
		t.Fatal(out.String())
	}
	out.Reset()
	r := Analysis{Required: 2, Analyzed: 1, Missing: []string{"src/b.ts"}, Passed: false, Reason: "missing file"}.WithUnresolved([]string{"src/a.ts: ./gone"})
	r.WriteText(&out)
	for _, want := range []string{"1/2", "missing: src/b.ts", "unresolved import: src/a.ts: ./gone", "FAIL:"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
}
