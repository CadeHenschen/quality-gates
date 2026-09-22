package arch

import "testing"

func TestCompileRejectsMissingFields(t *testing.T) {
	cases := []struct {
		name  string
		rules []Rule
	}{
		{"missing name", []Rule{{From: "a", Deny: []string{"b"}}}},
		{"missing from", []Rule{{Name: "r", Deny: []string{"b"}}}},
		{"missing deny", []Rule{{Name: "r", From: "a"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Compile(c.rules, nil); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}

func TestRuleMatchingSemantics(t *testing.T) {
	rs, err := Compile([]Rule{
		{Name: "domain must not depend on infra", From: "internal/domain", Deny: []string{"internal/infra"}},
		{Name: "cmd binaries are independent", From: "cmd/*", Deny: []string{"cmd/*"}},
	}, nil)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	edges := []Edge{
		// domain -> infra directly: violates rule 1.
		{File: "internal/domain/service.go", FromPkg: "internal/domain", Import: "internal/infra/db.go", ToPkg: "internal/infra"},
		// domain -> a domain subpackage: not a layering question at all.
		{File: "internal/domain/service.go", FromPkg: "internal/domain", Import: "internal/domain/model/user.go", ToPkg: "internal/domain/model"},
		// infra -> domain: allowed direction, no rule forbids it.
		{File: "internal/infra/db.go", FromPkg: "internal/infra", Import: "internal/domain/service.go", ToPkg: "internal/domain"},
		// cmd/a -> cmd/b: violates rule 2.
		{File: "cmd/a/main.go", FromPkg: "cmd/a", Import: "cmd/b/main.go", ToPkg: "cmd/b"},
		// intra-package: never a violation even though FromPkg == ToPkg
		// would otherwise match a rule's own From/Deny.
		{File: "cmd/a/main.go", FromPkg: "cmd/a", Import: "cmd/a/helper.go", ToPkg: "cmd/a"},
	}

	violations := rs.Check(edges)
	if len(violations) != 2 {
		t.Fatalf("got %d violation(s), want 2: %+v", len(violations), violations)
	}
	if violations[0].Rule != "cmd binaries are independent" || violations[0].File != "cmd/a/main.go" {
		t.Errorf("violation 0 = %+v, want the cmd/a -> cmd/b violation (sorted first by rule name)", violations[0])
	}
	if violations[1].Rule != "domain must not depend on infra" || violations[1].File != "internal/domain/service.go" {
		t.Errorf("violation 1 = %+v, want the domain -> infra violation", violations[1])
	}
}

func TestRuleFromMatchesItselfAndSubdirs(t *testing.T) {
	rs, err := Compile([]Rule{
		{Name: "r", From: "internal/domain", Deny: []string{"internal/infra"}},
	}, nil)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	// A rule's "from" pattern with no wildcard should match the exact
	// directory and any subdirectory beneath it.
	edges := []Edge{
		{File: "internal/domain/x.go", FromPkg: "internal/domain", Import: "internal/infra/y.go", ToPkg: "internal/infra"},
		{File: "internal/domain/model/x.go", FromPkg: "internal/domain/model", Import: "internal/infra/y.go", ToPkg: "internal/infra"},
	}
	violations := rs.Check(edges)
	if len(violations) != 2 {
		t.Fatalf("got %d violation(s), want 2 (both internal/domain and internal/domain/model should match \"from\"): %+v", len(violations), violations)
	}
}
