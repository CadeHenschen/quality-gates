package deadcode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "dead", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func find(t *testing.T, fs []Finding, name string) Finding {
	t.Helper()
	for _, f := range fs {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("no finding named %q in %+v", name, fs)
	return Finding{}
}

// The Go fixture is real `deadcode -json` output over
// testdata/dead/golang: an unreachable function and an unreachable method.
func TestParseGoDeadcode(t *testing.T) {
	got, err := Parse(fixture(t, "deadcode.json"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 findings (orphan, Unused, Thing.Method), got %+v", got)
	}
	orphan := find(t, got, "orphan")
	if orphan.File != "main.go" || orphan.Line != 14 || orphan.Kind != KindFunction {
		t.Errorf("orphan = %+v", orphan)
	}
	method := find(t, got, "Thing.Method")
	if method.File != "util/util.go" || method.Line != 13 {
		t.Errorf("method = %+v", method)
	}
}

func TestParseGoDeadcodeSkipsGenerated(t *testing.T) {
	in := `[{"Name":"p","Path":"x/p","Funcs":[
		{"Name":"gen","Position":{"File":"a.pb.go","Line":3},"Generated":true},
		{"Name":"real","Position":{"File":"a.go","Line":9},"Generated":false}]}]`
	got, err := Parse([]byte(in), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "real" {
		t.Fatalf("generated code is not the author's to delete, want only real: %+v", got)
	}
}

// The knip fixture is real `knip --reporter json` output: an orphan file,
// two unused exports, an unused type and enum, and an unused dependency.
func TestParseKnip(t *testing.T) {
	got, err := Parse(fixture(t, "knip.json"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Kind{
		"src/orphan.ts": KindFile,
		"unusedFn":      KindExport,
		"unusedConst":   KindExport,
		"UnusedType":    KindType,
		"Color":         KindType,
	}
	if len(got) != len(want) {
		t.Fatalf("want %d findings (unused deps are not code), got %+v", len(want), got)
	}
	for name, kind := range want {
		if f := find(t, got, name); f.Kind != kind {
			t.Errorf("%s kind = %s, want %s", name, f.Kind, kind)
		}
	}
	if f := find(t, got, "unusedFn"); f.File != "src/lib.ts" || f.Line != 2 {
		t.Errorf("unusedFn = %+v", f)
	}
	if f := find(t, got, "src/orphan.ts"); f.File != "src/orphan.ts" || f.Line != 0 {
		t.Errorf("an unused file is reported at its own path, line 0: %+v", f)
	}
}

func TestParseKnipEnumAndNamespaceMembers(t *testing.T) {
	in := `{"issues":[{"file":"src/c.ts","exports":[],"files":[],
		"enumMembers":[{"name":"Blue","line":4}],
		"namespaceMembers":[{"name":"helper","line":9}]}]}`
	got, err := Parse([]byte(in), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || find(t, got, "Blue").Kind != KindMember || find(t, got, "helper").Kind != KindMember {
		t.Fatalf("got %+v", got)
	}
}

func TestParseRelativizesAbsolutePathsToDir(t *testing.T) {
	dir := t.TempDir()
	in := `[{"Name":"p","Path":"x","Funcs":[{"Name":"f","Position":{"File":"` + filepath.ToSlash(filepath.Join(dir, "pkg", "a.go")) + `","Line":1}}]}]`
	got, err := Parse([]byte(in), Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].File != "pkg/a.go" {
		t.Fatalf("File must be --dir-relative (ratchet matching): %+v", got)
	}
}

func TestParseRejectsUnrecognizedReports(t *testing.T) {
	for name, in := range map[string]string{
		"not json":       "hello",
		"unknown object": `{"foo":1}`,
		"empty":          ``,
		"blank":          "  \n",
	} {
		_, err := Parse([]byte(in), Options{})
		if err == nil || !strings.Contains(err.Error(), "dead") {
			t.Errorf("%s: want an error naming the supported formats, got %v", name, err)
		}
	}
}

func TestParseCleanReportsAreEmpty(t *testing.T) {
	for name, in := range map[string]string{
		"deadcode": `[]`,
		// Real `deadcode -json` output when nothing is dead: a JSON null.
		"deadcode null": "null\n",
		"knip":          `{"issues":[]}`,
	} {
		got, err := Parse([]byte(in), Options{})
		if err != nil || len(got) != 0 {
			t.Errorf("%s: a clean run is a valid report with no findings, got %v, %v", name, got, err)
		}
	}
}

// The vulture fixture is real `vulture .` output over testdata/dead/python.
func TestParseVulture(t *testing.T) {
	got, err := Parse(fixture(t, "vulture.txt"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 9 {
		t.Fatalf("want 9 findings, got %d: %+v", len(got), got)
	}
	want := map[string]Kind{
		"os":                           KindImport,
		"unused_function":              KindFunction,
		"method":                       KindFunction,
		"Dead":                         KindType,
		"unused_attr":                  KindMember,
		"unused_local":                 KindMember,
		"never_called":                 KindFunction,
		"y":                            KindMember,
		"unsatisfiable 'if' condition": KindUnreachable,
	}
	for name, kind := range want {
		if f := find(t, got, name); f.Kind != kind {
			t.Errorf("%s kind = %s, want %s", name, f.Kind, kind)
		}
	}
	if f := find(t, got, "never_called"); f.File != "pkg/util.py" || f.Line != 9 {
		t.Errorf("never_called = %+v", f)
	}
}

func TestParseVultureUnreachableCode(t *testing.T) {
	got, err := Parse([]byte("a.py:4: unreachable code after 'return' (100% confidence)\n"), Options{})
	if err != nil || len(got) != 1 || got[0].Kind != KindUnreachable || got[0].Line != 4 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

// vulture prints nothing when it finds nothing, which is indistinguishable
// from a step that crashed (CI must swallow its exit code 3). Auto-detect
// therefore refuses an empty report; --format vulture vouches for it.
func TestParseEmptyReportNeedsExplicitFormat(t *testing.T) {
	got, err := Parse(nil, Options{Format: FormatVulture})
	if err != nil || len(got) != 0 {
		t.Errorf("explicit vulture + empty is a clean run, got %v, %v", got, err)
	}
	if _, err := Parse(nil, Options{}); err == nil || !strings.Contains(err.Error(), "--format vulture") {
		t.Errorf("auto-detect on empty must point at --format vulture, got %v", err)
	}
}

func TestParseVultureRejectsUnparseableLines(t *testing.T) {
	_, err := Parse([]byte("a.py:1: unused function 'f' (60% confidence)\nnot a finding\n"), Options{Format: FormatVulture})
	if err == nil || !strings.Contains(err.Error(), "not a finding") {
		t.Errorf("a line we can't read must be an error naming it, not silently dropped: %v", err)
	}
}

func TestParseExplicitFormatOverridesDetection(t *testing.T) {
	if _, err := Parse([]byte("[]"), Options{Format: FormatVulture}); err == nil {
		t.Error("JSON forced through the vulture parser should fail")
	}
	if _, err := Parse([]byte("x"), Options{Format: "bogus"}); err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Errorf("unknown --format must be named in the error, got %v", err)
	}
	got, err := Parse([]byte("[]"), Options{Format: FormatDeadcode})
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v", got, err)
	}
}
