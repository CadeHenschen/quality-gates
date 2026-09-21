package deadcode

import (
	"strings"
	"testing"
)

func TestIgnoreMatching(t *testing.T) {
	ig, err := ParseIgnore(strings.NewReader(`
# reflection target, called by name from a plugin registry
Registered

internal/gen/
**/*_gen.go
cmd/*/main.go
`))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		f    Finding
		want bool
	}{
		{Finding{File: "a.go", Name: "Registered"}, true},
		{Finding{File: "a.go", Name: "Thing.Registered"}, true},
		{Finding{File: "a.go", Name: "NotRegistered"}, false},
		{Finding{File: "internal/gen/x/y.go", Name: "f"}, true},
		{Finding{File: "internal/genx/y.go", Name: "f"}, false},
		{Finding{File: "deep/nested/foo_gen.go", Name: "f"}, true},
		{Finding{File: "foo_gen.go", Name: "f"}, true},
		{Finding{File: "cmd/tool/main.go", Name: "f"}, true},
		{Finding{File: "cmd/tool/sub/main.go", Name: "f"}, false},
	}
	for _, c := range cases {
		if got := ig.Matches(c.f); got != c.want {
			t.Errorf("Matches(%+v) = %v, want %v", c.f, got, c.want)
		}
	}
}

func TestIgnoreRejectsBadGlob(t *testing.T) {
	if _, err := ParseIgnore(strings.NewReader("[unclosed\n")); err == nil {
		t.Error("a malformed glob must be an error, not a silently dead rule")
	}
}

func TestIgnoreFilter(t *testing.T) {
	ig, _ := ParseIgnore(strings.NewReader("keep\n"))
	got := ig.Filter([]Finding{{File: "a", Name: "keep"}, {File: "a", Name: "drop"}})
	if len(got) != 1 || got[0].Name != "drop" {
		t.Errorf("got %+v", got)
	}
	var none Ignore
	if len(none.Filter(sample())) != 3 {
		t.Error("zero-value Ignore ignores nothing")
	}
}
