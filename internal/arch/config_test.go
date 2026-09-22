package arch

import (
	"os"
	"path/filepath"
	"testing"
)

func writeRulesFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadRulesValid(t *testing.T) {
	path := writeRulesFile(t, `{
		"rules": [
			{"name": "domain must not depend on infra", "from": "internal/domain", "deny": ["internal/infra"]}
		]
	}`)

	rules, exceptions, err := LoadRules(path)
	if err != nil {
		t.Fatalf("LoadRules: %v", err)
	}
	if len(rules) != 1 || rules[0].Name != "domain must not depend on infra" {
		t.Errorf("rules = %+v", rules)
	}
	if len(exceptions) != 0 {
		t.Errorf("exceptions = %+v, want none (the file declares none)", exceptions)
	}
	if _, err := Compile(rules, exceptions); err != nil {
		t.Errorf("Compile of loaded rules: %v", err)
	}
}

func TestLoadRulesWithExceptions(t *testing.T) {
	path := writeRulesFile(t, `{
		"rules": [
			{"name": "domain must not depend on infra", "from": "internal/domain", "deny": ["internal/infra"]}
		],
		"exceptions": [
			{"rule": "domain must not depend on infra", "from": "internal/domain/legacy", "to": "internal/infra", "reason": "migrating off in Q1"}
		]
	}`)

	rules, exceptions, err := LoadRules(path)
	if err != nil {
		t.Fatalf("LoadRules: %v", err)
	}
	if len(exceptions) != 1 || exceptions[0].From != "internal/domain/legacy" || exceptions[0].Reason != "migrating off in Q1" {
		t.Errorf("exceptions = %+v", exceptions)
	}
	if _, err := Compile(rules, exceptions); err != nil {
		t.Errorf("Compile of loaded rules+exceptions: %v", err)
	}
}

func TestLoadRulesMissingFile(t *testing.T) {
	if _, _, err := LoadRules(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Error("expected an error for a missing rules file")
	}
}

func TestLoadRulesEmptyList(t *testing.T) {
	path := writeRulesFile(t, `{"rules": []}`)
	if _, _, err := LoadRules(path); err == nil {
		t.Error("expected an error for a rules file with no rules declared")
	}
}

func TestLoadRulesMalformedJSON(t *testing.T) {
	path := writeRulesFile(t, `not json`)
	if _, _, err := LoadRules(path); err == nil {
		t.Error("expected an error for malformed JSON")
	}
}
