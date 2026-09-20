package exclude

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMatches(t *testing.T) {
	cases := []struct {
		pattern, file string
		want          bool
	}{
		{"internal/gen/**", "internal/gen/a.go", true},
		{"internal/gen/**", "internal/gen/sub/a.go", true},
		{"internal/gen/**", "internal/genx/a.go", false},
		{"internal/gen", "internal/gen/a.go", true}, // directory takes its contents
		{"internal/gen/", "internal/gen/a.go", true},
		{"**/*_pb.go", "x/y/foo_pb.go", true},
		{"**/*_pb.go", "foo_pb.go", true},
		{"**/*_pb.go", "foo_pb.go.bak", false},
		{"*_pb.go", "deep/dir/foo_pb.go", true}, // bare name: any depth
		{"src/*.ts", "src/a.ts", true},
		{"src/*.ts", "src/sub/a.ts", false}, // * stays in one segment
		{"a?.go", "ab.go", true},
		{"a?.go", "a/.go", false},
		{"./src/a.ts", "src/a.ts", true},
		{"a.b", "aXb", false}, // regex metachars are literal
	}
	for _, c := range cases {
		s, err := Compile([]string{c.pattern})
		if err != nil {
			t.Fatal(err)
		}
		if got := s.Matches(c.file); got != c.want {
			t.Errorf("pattern %q file %q = %v, want %v", c.pattern, c.file, got, c.want)
		}
	}
}

func TestZeroSetAndBlanks(t *testing.T) {
	if (Set{}).Matches("a.go") {
		t.Error("zero Set must exclude nothing")
	}
	s, err := Compile([]string{"", "  "})
	if err != nil || s.Matches("a.go") {
		t.Errorf("blank patterns should be ignored (err=%v)", err)
	}
}

func TestReadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x")
	body := "# generated code\ninternal/gen/**\n\n**/*_pb.go  # protobuf output\n   \n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"internal/gen/**", "**/*_pb.go"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if _, err := ReadFile(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("expected error for a missing file")
	}
}
