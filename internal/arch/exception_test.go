package arch

import "testing"

func TestCompileRejectsMalformedException(t *testing.T) {
	rule := []Rule{{Name: "r", From: "a", Deny: []string{"b"}}}
	cases := []struct {
		name  string
		excpt []Exception
	}{
		{"missing rule", []Exception{{From: "a", To: "b"}}},
		{"missing from", []Exception{{Rule: "r", To: "b"}}},
		{"missing to", []Exception{{Rule: "r", From: "a"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Compile(rule, c.excpt); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}

func TestExceptionDropsMatchingViolationFromBothReportAndGate(t *testing.T) {
	rs, err := Compile(
		[]Rule{{Name: "domain must not depend on infra", From: "internal/domain", Deny: []string{"internal/infra"}}},
		[]Exception{{Rule: "domain must not depend on infra", From: "internal/domain/legacy", To: "internal/infra", Reason: "migrating off in Q1"}},
	)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	edges := []Edge{
		// Exempted: matches the exception's rule+from+to exactly.
		{File: "internal/domain/legacy/x.go", FromPkg: "internal/domain/legacy", Import: "internal/infra/y.go", ToPkg: "internal/infra"},
		// Not exempted: same rule, but a different "from" package.
		{File: "internal/domain/service.go", FromPkg: "internal/domain", Import: "internal/infra/y.go", ToPkg: "internal/infra"},
	}

	violations := rs.Check(edges)
	if len(violations) != 1 {
		t.Fatalf("got %d violation(s), want 1 (the exempted edge should be dropped entirely): %+v", len(violations), violations)
	}
	if violations[0].File != "internal/domain/service.go" {
		t.Errorf("surviving violation = %+v, want the non-exempted internal/domain/service.go one", violations[0])
	}
}

func TestExceptionIsScopedToItsOwnRule(t *testing.T) {
	rs, err := Compile(
		[]Rule{
			{Name: "rule A", From: "a", Deny: []string{"b"}},
			{Name: "rule B", From: "a", Deny: []string{"b"}},
		},
		[]Exception{{Rule: "rule A", From: "a", To: "b"}},
	)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	edges := []Edge{{File: "a/x.go", FromPkg: "a", Import: "b/y.go", ToPkg: "b"}}
	violations := rs.Check(edges)
	if len(violations) != 1 || violations[0].Rule != "rule B" {
		t.Fatalf("expected exactly the \"rule B\" violation to survive (the exception only names \"rule A\"), got %+v", violations)
	}
}

func TestExceptionPatternsUseGlobs(t *testing.T) {
	rs, err := Compile(
		[]Rule{{Name: "r", From: "internal", Deny: []string{"cmd"}}},
		[]Exception{{Rule: "r", From: "internal/legacy/*", To: "cmd"}},
	)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	edges := []Edge{
		{File: "internal/legacy/foo/x.go", FromPkg: "internal/legacy/foo", Import: "cmd/y.go", ToPkg: "cmd"},
		{File: "internal/other/x.go", FromPkg: "internal/other", Import: "cmd/y.go", ToPkg: "cmd"},
	}
	violations := rs.Check(edges)
	if len(violations) != 1 || violations[0].FromPkg != "internal/other" {
		t.Fatalf("expected only the internal/other violation to survive, got %+v", violations)
	}
}
