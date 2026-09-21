// Package deadcode gates on unused code: functions no entry point can reach,
// exports nothing imports, files nothing pulls in. Deleting code is the
// cheapest simplification there is, and the gate keeps new dead weight from
// accumulating.
//
// Like mutation and coverage, this package does not detect anything itself:
// reachability analysis is language-specific and already done well by tools
// the target repo runs in its own CI — `deadcode` (golang.org/x/tools) for
// Go, `knip` for TypeScript/JavaScript, `vulture` for Python, `periphery` for
// Swift. It parses their JSON reports into
// one normalized model and gates on the count, with the same --only-files
// ratchet as every other tool here.
package deadcode

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Kind classifies a finding across every source format.
type Kind string

const (
	KindFunction Kind = "function" // a function or method nothing reachable calls
	KindFile     Kind = "file"     // a whole file nothing imports
	KindExport   Kind = "export"   // an exported value nothing imports
	KindType     Kind = "type"     // an exported type or enum nothing imports
	KindMember   Kind = "member"   // an unused enum/namespace member, variable or attribute
	KindImport   Kind = "import"   // an import nothing uses
	// KindUnreachable is code that can never run (after a return, behind an
	// unsatisfiable condition); Name is the analyzer's own description.
	KindUnreachable Kind = "unreachable"
)

// Report formats, for Options.Format.
const (
	FormatDeadcode  = "deadcode"  // `deadcode -json` (Go)
	FormatKnip      = "knip"      // `knip --reporter json` (TS/JS)
	FormatVulture   = "vulture"   // `vulture` text output (Python)
	FormatPeriphery = "periphery" // `periphery scan --format json` (Swift)
)

// Finding is one piece of dead code.
type Finding struct {
	File string `json:"file"`
	Line int    `json:"line"` // 0 when the whole file is dead
	Name string `json:"name"`
	Kind Kind   `json:"kind"`
}

// Options controls Parse.
type Options struct {
	// Format skips auto-detection. Needed for one case: vulture prints
	// nothing when it finds nothing, so an empty report is only accepted
	// when the caller says it is vulture's.
	Format string
	// Dir, when set, relativizes absolute paths in the report to it, so
	// File is always --dir-relative (see CLAUDE.md on ratchet matching).
	Dir string
}

// Parse reads a dead-code report, in opts.Format or else auto-detected:
// `deadcode -json` (a JSON array of packages, or null), `knip --reporter
// json` (an object with an `issues` array), or `periphery` JSON
// (`file:line: unused function 'f' (60% confidence)`).
func Parse(data []byte, opts Options) ([]Finding, error) {
	format := opts.Format
	if format == "" {
		var err error
		if format, err = detectFormat(data); err != nil {
			return nil, err
		}
	}
	var found []Finding
	var err error
	switch format {
	case FormatDeadcode:
		found, err = parseGoDeadcode(data)
	case FormatKnip:
		found, err = parseKnip(data)
	case FormatVulture:
		found, err = parseVulture(data)
	case FormatPeriphery:
		found, err = parsePeriphery(data)
	default:
		return nil, fmt.Errorf("unknown --format %q (want %s, %s, %s, or %s)", format, FormatDeadcode, FormatKnip, FormatVulture, FormatPeriphery)
	}
	if err != nil {
		return nil, err
	}
	for i := range found {
		found[i].File = normalizePath(found[i].File, opts.Dir)
	}
	return found, nil
}

func detectFormat(data []byte) (string, error) {
	trimmed := strings.TrimSpace(string(data))
	switch {
	case trimmed == "":
		return "", fmt.Errorf("empty dead-code report: can't tell a clean run from a step that failed to run; for a clean vulture run (it prints nothing) pass --format %s", FormatVulture)
	case strings.HasPrefix(trimmed, "[") && isPeripheryArray(data):
		return FormatPeriphery, nil
	case trimmed == "null" || strings.HasPrefix(trimmed, "["):
		// `deadcode -json` prints a bare null, not [], when nothing is dead.
		return FormatDeadcode, nil
	case strings.HasPrefix(trimmed, "{") && strings.Contains(trimmed, `"issues"`):
		return FormatKnip, nil
	case vultureLine.MatchString(strings.SplitN(trimmed, "\n", 2)[0]):
		return FormatVulture, nil
	}
	return "", fmt.Errorf("unrecognized dead-code report: want `deadcode -json` (Go), `knip --reporter json` (TS/JS) `vulture` (Python) or `periphery` (Swift) output")
}

func normalizePath(file, dir string) string {
	if dir != "" && filepath.IsAbs(file) {
		if absDir, err := filepath.Abs(dir); err == nil {
			if rel, err := filepath.Rel(absDir, file); err == nil {
				file = rel
			}
		}
	}
	return strings.TrimPrefix(filepath.ToSlash(file), "./")
}

func parseGoDeadcode(data []byte) ([]Finding, error) {
	var pkgs []struct {
		Funcs []struct {
			Name     string
			Position struct {
				File string
				Line int
			}
			Generated bool
		}
	}
	if err := json.Unmarshal(data, &pkgs); err != nil {
		return nil, fmt.Errorf("parse deadcode report: %w", err)
	}
	var out []Finding
	for _, p := range pkgs {
		for _, f := range p.Funcs {
			if f.Generated {
				continue // not the author's to delete; regenerate instead
			}
			out = append(out, Finding{File: f.Position.File, Line: f.Position.Line, Name: f.Name, Kind: KindFunction})
		}
	}
	return out, nil
}

type knipSymbol struct {
	Name string `json:"name"`
	Line int    `json:"line"`
}

func parseKnip(data []byte) ([]Finding, error) {
	var report struct {
		Issues []struct {
			File             string       `json:"file"`
			Files            []knipSymbol `json:"files"`
			Exports          []knipSymbol `json:"exports"`
			Types            []knipSymbol `json:"types"`
			EnumMembers      []knipSymbol `json:"enumMembers"`
			NamespaceMembers []knipSymbol `json:"namespaceMembers"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("parse knip report: %w", err)
	}
	var out []Finding
	// Unused dependencies (knip's `dependencies`, `unlisted`, ...) are
	// deliberately not read: they are manifest hygiene, not dead code.
	for _, issue := range report.Issues {
		add := func(syms []knipSymbol, kind Kind) {
			for _, s := range syms {
				out = append(out, Finding{File: issue.File, Line: s.Line, Name: s.Name, Kind: kind})
			}
		}
		add(issue.Files, KindFile)
		add(issue.Exports, KindExport)
		add(issue.Types, KindType)
		add(issue.EnumMembers, KindMember)
		add(issue.NamespaceMembers, KindMember)
	}
	return out, nil
}

var (
	vultureLine   = regexp.MustCompile(`^(.+?):(\d+): (.+?)(?: \((\d+)% confidence\))?$`)
	vultureUnused = regexp.MustCompile(`^unused (\w+) '(.*)'$`)
)

// parseVulture reads vulture's text report. Confidence is dropped: vulture's
// own --min-confidence is the way to filter on it, before this sees it.
func parseVulture(data []byte) ([]Finding, error) {
	var out []Finding
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		m := vultureLine.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("parse vulture report: can't read line %q", line)
		}
		lineNo, _ := strconv.Atoi(m[2]) // \d+ by construction
		f := Finding{File: m[1], Line: lineNo}
		msg := m[3]
		if u := vultureUnused.FindStringSubmatch(msg); u != nil {
			f.Name, f.Kind = u[2], vultureKind(u[1])
		} else if strings.HasPrefix(msg, "unreachable") || strings.HasPrefix(msg, "unsatisfiable") {
			f.Name, f.Kind = msg, KindUnreachable
		} else {
			return nil, fmt.Errorf("parse vulture report: can't read line %q", line)
		}
		out = append(out, f)
	}
	return out, nil
}

func vultureKind(word string) Kind {
	switch word {
	case "function", "method":
		return KindFunction
	case "class":
		return KindType
	case "import":
		return KindImport
	}
	return KindMember // variable, attribute, property
}

// isPeripheryArray tells a periphery report from a deadcode one — both are
// JSON arrays — by its results carrying `hints`. Misreading one as the
// other would parse to zero findings, a silent pass.
func isPeripheryArray(data []byte) bool {
	var first []map[string]json.RawMessage
	if json.Unmarshal(data, &first) != nil || len(first) == 0 {
		return false
	}
	_, ok := first[0]["hints"]
	return ok
}

// parsePeriphery reads `periphery scan --format json`. Only results hinted
// `unused` are dead code. `assignOnlyProperty` is deliberately dropped: on a
// persisted model (Codable, SwiftData) a field written but never read in
// code is usually read by the coder or the database. The `redundant*` hints
// are API tidiness. Periphery scans the app scheme, not test targets, so
// code only tests use is reported — the strict reading of "dead".
func parsePeriphery(data []byte) ([]Finding, error) {
	var results []struct {
		Kind     string   `json:"kind"`
		Name     string   `json:"name"`
		Hints    []string `json:"hints"`
		Location string   `json:"location"`
	}
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, fmt.Errorf("parse periphery report: %w", err)
	}
	var out []Finding
	for _, r := range results {
		if !slices.Contains(r.Hints, "unused") {
			continue
		}
		file, line, err := splitPeripheryLocation(r.Location)
		if err != nil {
			return nil, err
		}
		out = append(out, Finding{File: file, Line: line, Name: r.Name, Kind: peripheryKind(r.Kind)})
	}
	return out, nil
}

// splitPeripheryLocation splits "path:line:col", from the right so a colon
// in the path survives.
func splitPeripheryLocation(loc string) (string, int, error) {
	parts := strings.Split(loc, ":")
	if len(parts) < 3 {
		return "", 0, fmt.Errorf("parse periphery report: can't read location %q (want path:line:col)", loc)
	}
	line, err := strconv.Atoi(parts[len(parts)-2])
	if err != nil {
		return "", 0, fmt.Errorf("parse periphery report: can't read location %q (want path:line:col)", loc)
	}
	return strings.Join(parts[:len(parts)-2], ":"), line, nil
}

func peripheryKind(kind string) Kind {
	switch base, _, _ := strings.Cut(kind, "."); base {
	case "function":
		return KindFunction
	case "class", "struct", "enum", "protocol", "typealias", "associatedtype", "extension":
		return KindType
	case "module":
		return KindImport
	}
	return KindMember // var.*, enumelement, ...
}
