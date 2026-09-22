package arch

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestStabilitiesComputesAfferentEfferentAndInstability(t *testing.T) {
	// domain <- infra (infra depends on domain)
	// domain is depended-on only (Ca=1, Ce=0 -> instability 0, maximally stable)
	// infra depends on domain only (Ca=0, Ce=1 -> instability 1, maximally unstable)
	edges := []Edge{
		{File: "infra/x.go", FromPkg: "infra", Import: "domain/y.go", ToPkg: "domain"},
	}

	s := Stabilities(edges)
	if len(s) != 2 {
		t.Fatalf("got %d package(s), want 2: %+v", len(s), s)
	}
	domain, ok := s["domain"]
	if !ok {
		t.Fatalf("missing \"domain\" in %+v", s)
	}
	if domain.Afferent != 1 || domain.Efferent != 0 || domain.Instability != 0 {
		t.Errorf("domain = %+v, want Afferent=1 Efferent=0 Instability=0", domain)
	}

	infra, ok := s["infra"]
	if !ok {
		t.Fatalf("missing \"infra\" in %+v", s)
	}
	if infra.Afferent != 0 || infra.Efferent != 1 || infra.Instability != 1 {
		t.Errorf("infra = %+v, want Afferent=0 Efferent=1 Instability=1", infra)
	}
}

func TestStabilitiesIgnoresIntraPackageEdges(t *testing.T) {
	edges := []Edge{
		{File: "domain/a.go", FromPkg: "domain", Import: "domain/b.go", ToPkg: "domain"},
	}
	s := Stabilities(edges)
	if len(s) != 0 {
		t.Errorf("intra-package edge should contribute no coupling, got %+v", s)
	}
}

func TestStabilitiesDedupesFanOutToOnePackagePair(t *testing.T) {
	// Go's importer fans a single package import out to every file in the
	// target package — that must count as ONE unit of coupling between
	// the two packages, not one per fanned-out file.
	edges := []Edge{
		{File: "infra/x.go", FromPkg: "infra", Import: "domain/a.go", ToPkg: "domain"},
		{File: "infra/x.go", FromPkg: "infra", Import: "domain/b.go", ToPkg: "domain"},
		{File: "infra/x.go", FromPkg: "infra", Import: "domain/c.go", ToPkg: "domain"},
	}
	s := Stabilities(edges)
	if s["infra"].Efferent != 1 {
		t.Errorf("infra.Efferent = %d, want 1 (one package dependency, however many files fanned out)", s["infra"].Efferent)
	}
	if s["domain"].Afferent != 1 {
		t.Errorf("domain.Afferent = %d, want 1", s["domain"].Afferent)
	}
}

func TestCheckStabilityFlagsStableDependingOnUnstable(t *testing.T) {
	// domain is depended on by both infra and ui, and depends on nothing:
	// maximally stable (instability 0).
	// infra depends only on domain: maximally unstable (instability 1).
	// domain -> infra would violate the Stable Dependencies Principle:
	// domain (stable) depending on infra (unstable) — but here it's the
	// other way around (infra -> domain), so no violation from that edge.
	// Add a deliberate reversal: domain -> infra.
	edges := []Edge{
		{File: "infra/x.go", FromPkg: "infra", Import: "domain/y.go", ToPkg: "domain"},
		{File: "ui/x.go", FromPkg: "ui", Import: "domain/y.go", ToPkg: "domain"},
		{File: "domain/z.go", FromPkg: "domain", Import: "infra/x.go", ToPkg: "infra"},
	}

	violations := CheckStability(edges)
	if len(violations) != 1 {
		t.Fatalf("got %d violation(s), want 1: %+v", len(violations), violations)
	}
	v := violations[0]
	if v.FromPkg != "domain" || v.ToPkg != "infra" {
		t.Errorf("violation = %+v, want domain -> infra", v)
	}
	if v.FromInstability >= v.ToInstability {
		t.Errorf("violation FromInstability (%v) should be less than ToInstability (%v)", v.FromInstability, v.ToInstability)
	}
}

func TestCheckStabilityNoViolationWhenDependingTowardStability(t *testing.T) {
	// infra (unstable) depending on domain (stable) is exactly the
	// direction Martin's principle wants — never a violation.
	edges := []Edge{
		{File: "infra/x.go", FromPkg: "infra", Import: "domain/y.go", ToPkg: "domain"},
		{File: "ui/x.go", FromPkg: "ui", Import: "domain/y.go", ToPkg: "domain"},
	}
	if v := CheckStability(edges); len(v) != 0 {
		t.Errorf("expected no violations, got %+v", v)
	}
}

func TestNewStabilityReportGate(t *testing.T) {
	violations := []StabilityViolation{{FromPkg: "a", ToPkg: "b", FromInstability: 0, ToInstability: 1}}

	r := NewStabilityReport(2, violations, 1)
	if !r.Passed || r.ExitCode() != 0 {
		t.Errorf("1 violation with --fail-above 1 should pass, got %+v", r)
	}

	r = NewStabilityReport(2, violations, 0)
	if r.Passed || r.ExitCode() != 1 {
		t.Errorf("1 violation with --fail-above 0 should fail, got %+v", r)
	}
}

func TestStabilityReportWriteTablePass(t *testing.T) {
	var buf bytes.Buffer
	NewStabilityReport(3, nil, 0).WriteTable(&buf, 20)
	if !strings.Contains(buf.String(), "PASS: no stable-dependency violations found") {
		t.Errorf("expected a clean-pass message, got:\n%s", buf.String())
	}
}

func TestStabilityReportWriteTableFailAndTruncation(t *testing.T) {
	var violations []StabilityViolation
	for i := 0; i < 5; i++ {
		violations = append(violations, StabilityViolation{FromPkg: "a", ToPkg: "b", FromInstability: 0, ToInstability: 1})
	}

	var buf bytes.Buffer
	NewStabilityReport(3, violations, 0).WriteTable(&buf, 2)
	out := buf.String()
	if !strings.Contains(out, "FAIL: 5 violation(s) exceeds 0") {
		t.Errorf("expected a FAIL summary, got:\n%s", out)
	}
	if !strings.Contains(out, "3 more violation(s) not shown") {
		t.Errorf("expected truncation note, got:\n%s", out)
	}
}

func TestStabilityReportWriteJSONRoundTrip(t *testing.T) {
	r := NewStabilityReport(4, []StabilityViolation{{FromPkg: "a", ToPkg: "b", FromInstability: 0.25, ToInstability: 0.75}}, 0)

	var buf bytes.Buffer
	if err := r.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var decoded StabilityReport
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.FilesAnalyzed != 4 || len(decoded.Violations) != 1 || decoded.Passed {
		t.Errorf("round-tripped report = %+v, want FilesAnalyzed=4, 1 violation, Passed=false", decoded)
	}
}
