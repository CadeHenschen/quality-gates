package escape

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanGoFixture(t *testing.T) {
	result, err := Scan(Options{Dir: "../../testdata/escape/golang", Lang: "go"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if result.TotalLines == 0 {
		t.Fatal("got 0 lines analyzed")
	}
	if len(result.LinesByFile) == 0 {
		t.Fatal("got no per-file line counts")
	}

	byPattern := map[string]int{}
	for _, h := range result.Hatches {
		byPattern[h.Pattern]++
	}
	if byPattern["nolint"] != 1 {
		t.Errorf("nolint count = %d, want 1: %+v", byPattern["nolint"], result.Hatches)
	}
	if byPattern["discarded-result"] != 1 {
		t.Errorf("discarded-result count = %d, want 1: %+v", byPattern["discarded-result"], result.Hatches)
	}
	if byPattern["nosec"] != 1 {
		t.Errorf("nosec count = %d, want 1: %+v", byPattern["nosec"], result.Hatches)
	}
}

func TestScanPythonFixture(t *testing.T) {
	result, err := Scan(Options{Dir: "../../testdata/escape/python", Lang: "python"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	byPattern := map[string]int{}
	for _, h := range result.Hatches {
		byPattern[h.Pattern]++
	}
	for _, want := range []string{"type-ignore", "noqa", "bare-except", "pylint-disable"} {
		if byPattern[want] != 1 {
			t.Errorf("%s count = %d, want 1: %+v", want, byPattern[want], result.Hatches)
		}
	}
}

func TestScanTypescriptFixture(t *testing.T) {
	result, err := Scan(Options{Dir: "../../testdata/escape/typescript", Lang: "ts"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	byPattern := map[string]int{}
	for _, h := range result.Hatches {
		byPattern[h.Pattern]++
	}
	for _, want := range []string{"ts-ignore", "eslint-disable", "ts-nocheck"} {
		if byPattern[want] != 1 {
			t.Errorf("%s count = %d, want 1: %+v", want, byPattern[want], result.Hatches)
		}
	}
}

func TestScanSwiftFixture(t *testing.T) {
	result, err := Scan(Options{Dir: "../../testdata/escape/swift", Lang: "swift"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	byPattern := map[string]int{}
	for _, h := range result.Hatches {
		byPattern[h.Pattern]++
	}
	if byPattern["force-try"] != 1 {
		t.Errorf("force-try count = %d, want 1: %+v", byPattern["force-try"], result.Hatches)
	}
	if byPattern["force-cast"] != 1 {
		t.Errorf("force-cast count = %d, want 1: %+v", byPattern["force-cast"], result.Hatches)
	}
	if byPattern["discarded-error"] != 1 {
		t.Errorf("discarded-error count = %d, want 1: %+v", byPattern["discarded-error"], result.Hatches)
	}
	if byPattern["swiftlint-disable"] != 1 {
		t.Errorf("swiftlint-disable count = %d, want 1: %+v", byPattern["swiftlint-disable"], result.Hatches)
	}
	// force-unwrap fires on its own dedicated line plus the force-try and
	// force-cast lines too (documented overlap in patterns.go — RE2 has no
	// lookbehind to tell "x!" apart from "try!"/"as!"'s own '!').
	if byPattern["force-unwrap"] != 3 {
		t.Errorf("force-unwrap count = %d, want 3: %+v", byPattern["force-unwrap"], result.Hatches)
	}

	for _, h := range result.Hatches {
		if h.Pattern == "force-unwrap" && (strings.Contains(h.Text, "!=") || strings.Contains(h.Text, "return !flag")) {
			t.Errorf("false-positive force-unwrap match: %+v", h)
		}
	}
}

func TestScanUnknownLang(t *testing.T) {
	_, err := Scan(Options{Dir: ".", Lang: "cobol"})
	if err == nil {
		t.Error("expected an error for an unknown language")
	}
}

func TestScanReportsFileRelativeToDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "real.go", "package p\nfunc F() {\n\t_ = fmt.Sprintf(\"x\") //nolint\n}\n")

	result, err := Scan(Options{Dir: dir, Lang: "go"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Hatches) != 1 || result.Hatches[0].File != "real.go" {
		t.Errorf("got %+v, want File relative to --dir (\"real.go\")", result.Hatches)
	}
	if result.LinesByFile["real.go"] == 0 {
		t.Errorf("LinesByFile = %v, want an entry for real.go", result.LinesByFile)
	}
}

func TestScanSkipsTestFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "real.go", "package p\nfunc F() {\n\t_ = fmt.Sprintf(\"x\") //nolint\n}\n")
	writeFile(t, dir, "real_test.go", "package p\nfunc T() {\n\t_ = fmt.Sprintf(\"x\") //nolint\n}\n")

	result, err := Scan(Options{Dir: dir, Lang: "go"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, h := range result.Hatches {
		if h.File == "real_test.go" {
			t.Errorf("_test.go file should have been skipped, got hatch from it: %+v", h)
		}
	}
}

func TestScanSkipsVendorNodeModulesTestdata(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{"vendor", "node_modules", "testdata"} {
		full := filepath.Join(dir, sub)
		if err := os.MkdirAll(full, 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, full, "x.go", "package p\nfunc F() { _ = fmt.Sprintf(\"x\") }\n")
	}
	writeFile(t, dir, "real.go", "package p\nfunc F() {}\n")

	result, err := Scan(Options{Dir: dir, Lang: "go"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Hatches) != 0 {
		t.Errorf("expected 0 hatches (only real.go should be scanned, and it has none), got: %+v", result.Hatches)
	}
}

func TestScanNoFalsePositiveOnCleanCode(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "clean.go", "package p\n\nfunc Clean(x int) int {\n\treturn x * 2\n}\n")

	result, err := Scan(Options{Dir: dir, Lang: "go"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Hatches) != 0 {
		t.Errorf("expected 0 hatches on clean code, got: %+v", result.Hatches)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
