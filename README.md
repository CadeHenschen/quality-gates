# quality-gates

Seven CI quality gate CLIs, one Go module: **crap-metric**, **dupe-metric**,
**escape-metric**, **cycle-metric**, **arch-metric**, **test-metric**, and
**dead-metric**. Each answers a different question about a change, with a
deliberately different, appropriately-scoped detection method — but they
share enough (the `--only-files` ratchet mechanism, the CI/release
plumbing, the install pattern) that running them as four separate repos
meant four copies of that shared logic drifting independently. Migrated
into one module for exactly that reason — see [CLAUDE.md](CLAUDE.md) for
the incidents that motivated it.

## The seven gates

| Binary | Question | Method | Default gate |
|---|---|---|---|
| `crap-metric` | Is this function complex *and* undertested?<br>Is it also too big to read? | Parses each language for cyclomatic complexity + coverage, combines via `complexity² × (1−coverage)³ + complexity`; the same parse also gates on size/shape (length, params, nesting, file length) | `--fail-above 30`; `--max-lines 80 --max-params 6 --max-nesting 5 --max-file-lines 600` |
| `dupe-metric` | Is this code duplicated? | Tokenizes source, finds exact-match blocks via greedy leftmost-longest shingling | `--fail-above 5` (%) |
| `escape-metric` | Did this code opt out of type-checking/linting/error handling? | Regex-matches suppression comments and a few high-confidence whole-line patterns | `--fail-above 1` (per 1000 lines) |
| `cycle-metric` | Are these modules structurally tangled? | Regex-extracts imports, resolves to files, runs Tarjan's SCC | `--fail-above 0` (cycles) |
| `arch-metric` | Does this code violate a declared layer boundary? | `check`: resolves imports to packages, checks each edge against a small "X may not import Y" rules file. `stability`: gates on Martin's Stable Dependencies Principle from the same graph | `check --fail-above 0` (violations); `stability --fail-above 0` (violations) |
| `dead-metric` | Is there code nothing uses? | Ingests `deadcode` (Go) / `knip` (TS/JS) / `vulture` (Python) / `periphery` (Swift) reports and gates on the count | `--fail-above 0` (dead symbols) |
| `test-metric` | Can these tests actually fail? | `check`: parses test files for tests with no assertions, skips, `.only`, mock-only assertions, leaked temp dirs. `mutation`: gates on a mutation tool's score | `check --fail-above 0` (findings); `mutation --fail-below 60` (%) |

The first four support `python` and `ts`/`typescript`/`js`; `crap-metric`,
`dupe-metric`, and `escape-metric` also support `go` and `swift`.
`arch-metric` supports `python`, `ts`/`js`, and **`go`** — the one place a
Go importer exists in this module at all, for the opposite reason
`cycle-metric` excludes Go: the compiler already refuses to build an
import cycle, but it has no opinion whatsoever on layering, so `arch-metric`
is exactly where a Go import graph earns its keep. `test-metric check`
supports all four (`go`, `python`, `ts`/`js`, `swift`).

`cycle-metric` deliberately excludes both Go and Swift: Go's compiler
already refuses to build a package-import cycle, so a detector for it
would always report zero; Swift files within one module never import each
other at all (no per-file import graph exists the way Python/TS have
one), and cycles between separate modules would need `Package.swift`-level
resolution this tool doesn't implement — see `--lang swift`'s own error
message and CLAUDE.md's Swift entry for the full reasoning. `arch-metric`
excludes only Swift, for the same per-file-import-graph reason. See each
package's doc comment (`internal/crap`, `internal/dupe`, `internal/escape`,
`internal/cycle`, `internal/arch`) for the full reasoning and known
characteristics of its method.

## Ratcheting: `--only-files`

Every `check` command (and `test-metric mutation`) accepts `--only-files PATH`, pointing at a
newline-separated changed-file list (e.g. `git diff --name-only`). This
computes a **second**, scoped gate verdict from only the items touching
those files — the full, unfiltered report is always still printed and
written to `--json`; `--only-files` narrows the *gate*, never what gets
reported.

This exists because a pure absolute threshold has a real cost: the first
time crap-metric's gate went live on a real app repo, it failed *every*
PR — not just ones touching the flagged files — until about a dozen
pre-existing hotspots were fixed. `--only-files` fixes that: existing
debt in files nobody touched no longer blocks the build; new or modified
code is still held to the full bar.

`internal/ratchet` implements the shared half (load the changed-file
list, match a `--dir`-relative path against it by suffix, since the list
is repo-root-relative and a tool doesn't know the repo root). Each CLI
keeps its own tool-specific filtering on top — a `crap.Function` filters
directly, a `dupe.Clone` counts as touched if *either* side matches, an
`escape.Hatch`'s scoped rate needs its own touched-files-only line-count
denominator (using the whole-repo total would dilute a small change's
hatches into a rate too tiny to ever trip `--fail-above`), a
`cycle.Cycle` counts as touched if *any* file in it matches, an
`arch.Violation` (and, identically, an `arch.StabilityViolation`) counts
as touched if *either* the importing or the imported file matches (same
reasoning as `dupe.Clone`), `test-metric
check` re-analyzes just the touched files' tests, and `test-metric
mutation` re-scores just the touched files' mutants (numerator and
denominator both come from the mutant list, so there's no separate
denominator to scope).

**Known limitation**: the suffix match has no path-boundary awareness
beyond "preceded by `/`" — it can't tell two files with the same leaf
name apart if one is a real path-boundary suffix of the other. A changed
file `web/src/index.ts` will match an item's `File: "index.ts"` even if
that item actually came from a completely unrelated `admin/index.ts`.
This is unlikely to matter for a distinctive filename, but a repo with
many same-named leaves (`index.ts`, `__init__.py`, `utils.go` across
several packages) should expect occasional false-positive ratchet
inclusion — an untouched hotspot gating the build because some other,
unrelated file with the same name was actually the one that changed. Not
currently worth the complexity of resolving both sides to full
repo-relative paths for what's, in practice, a rare and only-ever-extra
(never missed) inclusion.

In CI, the changed-file list itself comes from `ci-workflows`'
`compute-changed-files` action (a `git diff` against the PR base or the
previous push's `before` SHA) — see that repo's README for what it
needs from the caller (git installed before checkout, `fetch-depth: 0`).

## Usage

```
crap-metric   check --lang <python|go|ts|swift> --dir <dir> [--coverage PATH] [--fail-above N] [--max-lines N] [--max-params N] [--max-nesting N] [--max-file-lines N] [--verbose] [--require-analysis] [--only-files PATH] [--exclude GLOB]... [--exclude-file PATH] [--json PATH]
crap-metric   diff  --old PATH --new PATH [--top N] [--json]
dupe-metric   check --lang <python|go|ts|swift> --dir <dir> [--min-tokens N] [--fail-above PCT] [--require-analysis] [--only-files PATH] [--json PATH]
escape-metric check --lang <python|go|ts|swift> --dir <dir> [--fail-above RATE] [--require-analysis] [--only-files PATH] [--json PATH]
cycle-metric  check --lang <python|ts> --dir <dir> [--fail-above N] [--require-analysis] [--only-files PATH] [--json PATH]
arch-metric   check --lang <python|ts|go> --dir <dir> [--rules PATH] [--fail-above N] [--require-analysis] [--only-files PATH] [--json PATH]
arch-metric   stability --lang <python|ts|go> --dir <dir> [--fail-above N] [--require-analysis] [--only-files PATH] [--json PATH]
arch-metric   diff  --old PATH --new PATH [--top N] [--json]
test-metric   check --lang <go|python|ts|swift> --dir <dir> [--fail-above N] [--min-assertions N] [--ignore KIND,...] [--include KIND,...] [--require-analysis] [--only-files PATH] [--json PATH]
test-metric   mutation --report PATH [--dir <dir>] [--lang <go|python|ts|swift>] [--fail-below PCT] [--covered-only] [--min-mutants N] [--minimum-graded N] [--only-files PATH] [--json PATH]
<any tool>    version
```

Every `check` also takes `--top N` (rows to print, default 20, `0` = all).
`version` prints the build's short commit SHA (`dev` for a plain
`go build`/`go run`, since it's baked in via `-ldflags` — see Status).

### Requiring analysis evidence

The source-scanning `check` commands and `arch-metric stability` accept
`--require-analysis`. It is opt-in for direct CLI users. With no
`--only-files`, it fails when the requested language has no eligible files
under `--dir`, or when an eligible file was not visited by the scanner.
`test-metric check` requires test files to contain discovered tests. A file
with no functions can still count as visited by `crap-metric`.

With `--only-files`, eligibility is restricted to changed files under
`--dir`. A documentation-only change has no eligible files and passes as
not applicable; an eligible changed file missed by the analyzer fails.
The changed-file list is matched against exact repo-root-relative paths
when `--dir` is inside a Git checkout. Outside a Git checkout it is
interpreted relative to `--dir`. Reports keep the full unfiltered findings
and add an `analysis` object with required, analyzed, and missing counts.
The scoped verdict is printed separately, as with the existing ratchet.

`cycle-metric` and `arch-metric` also report unresolved relative local
imports as `unresolved_imports`; strict analysis fails when one is in
scope. Bare package imports are external. TypeScript path aliases are not
resolved yet and remain outside this check. Python `from . import name`
can name either a module or a symbol, so an unresolved name in that form
is not treated as a missing module.

### crap-metric: per-language coverage input

Each language needs its own coverage artifact generated *first*, using
tooling already present in that language's own CI job — crap-metric
doesn't run tests itself:

- **Python**: `coverage run -m pytest && coverage json` → pass the
  resulting `coverage.json`. Needs `radon` on `PATH` for complexity.
- **Go**: `go test -coverprofile=cover.out ./...` → pass `cover.out`.
  Complexity is computed natively (`go/ast`).
- **TypeScript/JS**: `vitest run --coverage` (or `jest --coverage`) →
  pass the resulting `coverage/coverage-final.json`. Complexity comes
  from an embedded Node script using the *target repo's own* installed
  `typescript` package (classic compiler API), for `.ts`/`.tsx` **and**
  plain `.js`/`.jsx` — TypeScript's parser reads JS natively, so a
  JS-only repo needs `typescript` (or the TS7 fallback below) as a
  devDependency too, purely to get that parser; nothing else is
  required, no `tsconfig.json` or actual TS usage. TypeScript 7 drops
  that API from its main entry point — the script falls back to
  Microsoft's official `@typescript/typescript6` compat package if
  present. See CLAUDE.md for the full TS7 story.
- **Swift**: `swift test --enable-code-coverage` then
  `llvm-cov export -format=lcov <test binary> -instr-profile <profdata>`
  → pass the resulting `.lcov` file. Complexity is computed natively via
  `internal/swiftlex`, a hand-written lexer (no subprocess, no Swift
  toolchain dependency for the analysis itself) — see CLAUDE.md for its
  design and documented v1 scope limitations.

#### Files missing from the coverage report ("unmeasured")

When `--coverage` is given but the report has no entry at all for a
function's file — typically a class/module no test ever loads — coverage is
*unknown*, not perfect. Such functions are flagged `unmeasured: true` in the
JSON, shown with `none` in the COVERAGE column, scored as **0% covered**
(so an untested, complex function trips `--fail-above` like any other), and
the table ends with a `WARNING:` listing the affected files. A file the
report *does* cover, whose function simply has no coverable lines, is still
treated as fully covered. With no `--coverage` at all nothing is flagged.
Code that genuinely shouldn't be measured (generated files, vendored code)
is declared with `--exclude`.

#### `--exclude GLOB` (repeatable)

Drops matching files from analysis entirely: unlike `--only-files`, which
narrows only the gate, excluded functions are absent from the printed
report and the `--json` output too (the table notes `excluded: N
function(s)`). Globs match the `--dir`-relative slash path: `*` stays
within a path segment, `**` spans segments, a pattern with no `/` matches
that name at any depth, and a pattern naming a directory excludes
everything under it. Example: `--exclude 'internal/gen/**' --exclude
'**/*_pb.go'`.

**Committed exclusion file (preferred).** If `<dir>/.crap-metric-exclude`
exists, crap-metric reads it automatically — no CI flags needed. One glob
per line; `#` starts a comment (whole-line or trailing), so each exclusion
can carry its reason in review:

```
# protobuf output, regenerated by `make proto`
**/*_pb.go
internal/gen/**   # codegen, not hand-edited
```

Patterns are relative to `--dir`. `--exclude-file PATH` points elsewhere
(and must exist, unlike the default), and `--exclude` flags add to
whatever the file lists. The file used is printed at the top of the run.

### crap-metric: size/shape gate

CRAP's cyclomatic complexity misses a real readability problem: a flat
40-case switch scores *high* on cyclomatic complexity but reads fine,
while a 6-deep nested `if` scores *low* but is unreadable. Rather than a
fifth tool, the same per-language parse that already walks each function
for complexity also records **function length** (derived from
start/end line, no extra field), **parameter count**, the function's own
**deepest control-flow nesting depth**, and its **file's physical line
count** — all as extra fields on the same `crap.Function` every
language analyzer already produces, gated independently of the CRAP
score itself via `--max-lines`/`--max-params`/`--max-nesting`/
`--max-file-lines` (each `<= 0` disables that one check). A function can
fail on size alone with a low CRAP score, or vice versa — the two gates
share the same report but are otherwise orthogonal (see `WriteTable`'s
separate `PASS`/`FAIL` line for each).

Defaults are deliberately generous — egregious cases only, not a style
gate — and were picked by running the gate against this repo's own real
source rather than guessed: 80 lines and 6 params come from the sizes
named in this repo's own design notes; 5 nesting levels and 600 file
lines were chosen from this repo's own observed maximums at the time
(nesting 4, 484 file lines) plus headroom, the same way dupe-metric's
18% self-check threshold and vulture's `--min-confidence 80` were
derived from a real run rather than picked in the abstract (see
CLAUDE.md). Nesting depth deliberately does **not** penalize a chained
`else if`/`elif` — like a many-case switch, it reads flat, so it's
walked as one level, not one-per-link; see each analyzer's `size.go` for
the language-specific details (Go's is the reference implementation;
Python/TypeScript/Swift mirror its algorithm) and CLAUDE.md's Swift entry
for that language's specific approximations (switch has no per-case
braces, so its whole body is one level rather than one per case; a
nested local func's own nesting is opaque to its enclosing function, the
same under-counting-is-safe choice the Go analyzer makes for closures).

### crap-metric: trend diffing

`crap-metric diff --old <report.json> --new <report.json>` compares two
JSON reports (matched by file+name) and prints every function that's
new, removed, or changed score, worst-regression-first. Informational
only — it always exits `0`; the gate stays `check`'s job.

### dupe-metric / escape-metric: how each language is analyzed

No external tool needed for either language in **escape-metric** (pure
Go regex over raw text) or for **Go**/**Swift** in dupe-metric (native
`go/scanner` for Go, `internal/swiftlex` for Swift). **Python** tokenizing
uses an embedded script against the standard library's `tokenize` module
(no pip package needed). **TypeScript/JS** tokenizing uses an embedded
Node script against the target repo's own `typescript` package's scanner
— same TS7 fallback as crap-metric.

Both tools skip comments and test files (`*_test.go`, `test_*.py`/
`*_test.py`, `*.test.ts`/`*.spec.ts`, `*.test.js`/`*.spec.js` and their
`.tsx`/`.jsx` variants) — duplicate/suppressed test boilerplate is
common and isn't the kind of finding these gates are for.

### cycle-metric: import resolution

Pure regex extraction, no parser: Python's `import`/`from` (absolute and
relative) and TS/JS's `import`/`export`/`require` specifiers (relative
only — bare/aliased imports are treated as external). Both languages are
scanned **with test files included**, unlike the other three tools — a
cycle is a structural property of the whole import graph, and excluding
tests could hide a real tangle. Every cycle's report includes both the
full set of files involved and a reconstructed concrete `Chain` — a
literal path back to its own start, not just an unordered set.

### arch-metric: declared layer rules

Go's compiler stops import cycles; it says nothing about *layering* — a
leaf package reaching back into one that's supposed to depend on it is
perfectly buildable, just architecturally backwards. arch-metric enforces
the Dependency Inversion Principle directly: a small, checked-in rules
file declares boundaries, and every resolved import is checked against
them structurally. A failure is binary (the import either exists or it
doesn't), never a judgment call.

It reuses cycle-metric's own machinery rather than reinventing it:
python's and typescript's importers are used completely unchanged (their
file-level import graph is simply read as `(file, package)` pairs, since
a package is just a file's own directory), and `--only-files` ratchets
the same way every other tool's does. The one new piece is
`internal/importers/golang` — a Go importer arch-metric needed and
cycle-metric deliberately never built, for the opposite reason:
`cycle-metric --lang go` would always report zero (see above), but a
completely acyclic Go program can absolutely violate a layer boundary,
so this is exactly where a Go import graph earns its keep. It resolves
each file's imports via `go/parser`'s import-only mode plus the enclosing
module's own `go.mod`, fans a package import out to every other scanned
file in that package's directory (Go's import unit is the package, not
the file, unlike Python/TS), and — like cycle-metric — includes test
files, since a layer boundary is a structural property of the whole
graph.

**Rules file** (`--rules PATH`, default `<dir>/.arch-metric-rules.json`):

```json
{
  "rules": [
    {
      "name": "domain must not depend on infra",
      "from": "internal/domain",
      "deny": ["internal/infra"]
    },
    {
      "name": "cmd binaries are independent processes",
      "from": "cmd/*",
      "deny": ["cmd/*"]
    }
  ]
}
```

Each rule's `from` and `deny` patterns reuse `internal/exclude`'s glob
syntax unchanged (the same one `--exclude` and dead-metric's `--ignore`
already use), matched against a `--dir`-relative package (directory)
path: a pattern with no wildcard matches that directory *and everything
under it* (`"internal/domain"` covers `internal/domain/model` too), `*`
stays within one path segment (`"cmd/*"` matches each `cmd/X` — and
everything under it — but never `cmd` itself), `**` spans segments. An
import within a single package is never a layering question and is
always skipped, which is what keeps the second example above from ever
matching a `cmd/X` package against itself.

Only `python`, `ts`/`js`, and `go` are supported — `arch-metric --lang
swift` errors the same way `cycle-metric --lang swift` does: Swift files
within one module never import each other, so there's no per-file import
graph to check boundaries against in the first place.

**Exceptions** grandfather one specific, already-known violation without
disabling the rule for everyone else — the same "drop it from both the
report and the gate" contract as dead-metric's `--ignore` and crap-metric's
`--exclude`, unlike `--only-files`, which only narrows the gate:

```json
{
  "rules": [ ... ],
  "exceptions": [
    {
      "rule": "domain must not depend on infra",
      "from": "internal/domain/legacy",
      "to": "internal/infra",
      "reason": "migrating off in Q1, JIRA-1234"
    }
  ]
}
```

An exception is scoped to one named `rule` on purpose: a bare `from`/`to`
glob pair with no rule name would silently exempt that package pair from
every rule that happens to match it, including ones added later — an easy
way to build an unintentional loophole. `reason` is optional but
conventional, the same way `.crap-metric-exclude`'s trailing `#` comments
carry a reason for review.

#### `arch-metric stability`: Martin's Stable Dependencies Principle

```
arch-metric stability --lang <python|ts|go> --dir <dir> [--fail-above N] [--only-files PATH | --baseline PATH] [--json PATH]
```

A second, independent gate computed from the same import graph `check`
already builds — no rules file needed. For every package, it computes
Robert Martin's afferent coupling (Ca: how many other packages depend on
it), efferent coupling (Ce: how many packages it depends on), and
instability `I = Ce / (Ca + Ce)` (0 = maximally stable, depended on by
many, depending on nothing; 1 = maximally unstable). A violation is any
import from a more stable package into a less stable one — "depend in the
direction of stability" — since that's exactly the shape of dependency
that makes the stable package hard to change without also touching
whatever unstable thing it now leans on. Package pairs, not raw file
edges: Go's importer fans one package import out to every file in the
target package (see above), so counting raw edges would inflate a
package's coupling by its target's file count rather than by how many
packages it actually depends on — `internal/arch.packagePairs` collapses
back to one edge per unique package pair before computing anything.
`--only-files` ratchets a stability verdict by the source files on either
end of a violating edge. For repositories carrying existing stability debt,
prefer `--baseline previous-arch-stability-report.json`: it evaluates the
whole current graph but gates only package-pair violations that are new since
that baseline. That is safer for a global metric — a dependency change can
alter another package's instability without editing the file that represents
its existing edge. `--baseline` and `--only-files` are mutually exclusive.

When an `arch-metric check` rules file (including its exceptions) appears in
the changed-file list, its ratchet intentionally becomes a full verdict.
Changing policy changes the meaning of every dependency; scoping that change
to zero source edges could otherwise silently admit a newly declared
violation.

#### `arch-metric diff`: trend comparison

```
arch-metric diff --old PATH --new PATH [--top N] [--json]
```

Mirrors crap-metric's own `diff`: compares two `check` JSON reports
(matched by rule+file+import) and prints every violation that's new or
fixed since the baseline, new ones first. Unlike crap-metric's `Delta`,
there's no continuous score to track a "changed" case for — a layer
violation either exists or it doesn't — so every entry is one or the
other. Informational only; always exits `0`, the same as crap-metric's.

### test-metric: are the tests any good?

Coverage says a line *ran*; it can't say anything *checked* the result. A
test that calls a function and asserts nothing scores the same as a good
one. test-metric grades the tests themselves, in two independent gates.

#### `test-metric check` — static findings

Parses each language's test files (`*_test.go`; `test_*.py`/`*_test.py`;
`*.test.*`/`*.spec.*` for TS/JS; XCTest `func test…()` and Swift Testing `@Test`
for Swift) and reports, per test:

| Check | Fires when |
|---|---|
| `no-assertions` | the test makes zero assertions, so it can't fail on wrong behavior |
| `low-assertions` | fewer than `--min-assertions` (default 1, so off unless raised) |
| `interaction-only` (**opt-in**, `--include interaction-only`) | *every* assertion only verifies a mock/spy was called (`toHaveBeenCalled`, `assert_called_once`, testify `AssertCalled`) — the test pins the implementation, nothing checks a return value or resulting state |
| `skipped` | unconditionally skipped: `t.Skip` outside any `if`/`switch`, `@pytest.mark.skip`, `@unittest.skip`, `pytest.skip()`, `it.skip`/`xit`/`it.todo`/`describe.skip`. Conditional skips (`skipif`, `test.skipIf`, a `t.Skip` inside an `if`) are environment guards and never fire |
| `focused` | `.only`, `fit`, `fdescribe` — silently disables every other test in the run |
| `expected-failure` | `@pytest.mark.xfail`, `@unittest.expectedFailure`, Playwright/Jest `failing`, `XCTExpectFailure`, `withKnownIssue` |
| `temp-no-cleanup` | creates a temp dir/file (`os.MkdirTemp`, `tempfile.mkdtemp`, `mkdtempSync`) with no cleanup in the test or in its file's `afterEach`/`afterAll`/`tearDown` |

The gate is the finding *count* (`--fail-above`, default `0`). The report
also prints assertions per test as an informational density number.
`--ignore` disables individual checks by name; `--include` enables the
opt-in ones (only `interaction-only` today, and `--ignore` wins if both name it).
`interaction-only` is off by default because it can't tell a mock that is a
*collaborator* (verifying it pins the implementation) from a callback prop that
is the component's *output*: over a real React app it flagged 345 of 1774 tests,
nearly all of them `expect(onChange).toHaveBeenCalledWith(...)` on a component
whose callbacks are its contract. Turn it on where mocks are collaborators. Test-level facts are extracted with real parsers — `go/parser`, Python's
`ast`, the target repo's own `typescript` package (same resolution and TS7
fallback as crap-metric) — not regexes.

What counts as an assertion is deliberately generous, so a finding is
almost always real: `t.Error*`/`t.Fatal*`, testify `assert.*`/`require.*`,
Python `assert`/`self.assert*`/`pytest.raises`, `expect(...)`/
`assert.*`/supertest `.expect(...)`, and any call of an assertion-shaped
name (`assertX`, `verifyX`, `checkX`, …). In Go, a call passing the test's
`t` to a same-package helper counts when that helper's *body* fails `t` (so
`findFunc(t, ...)` counts even though its name doesn't sound like an
assertion). Not detected, by design: whether assertions are *meaningful*.
That takes semantic understanding; the mutation gate below is the honest
check.

**Swift** runs on the same native `internal/swiftlex` lexer as crap-metric
and dupe-metric (no toolchain needed, same documented limits). A test is an
XCTest `func test…()` with no parameters in a file that imports XCTest, or any
`func` with a `@Test` attribute. Assertions are `XCTAssert*`/`XCTFail`/
`XCTUnwrap`, `#expect`/`#require`, and `Issue.record`; helpers anywhere under
`--dir` (e.g. a shared `TestHelpers.swift`) count when their body asserts.
`expectation(description:)` is not an assertion. Swift has no standard mock
library, so `interaction-only` never fires there.

#### `test-metric mutation` — does the suite catch bugs?

Mutation testing injects small bugs (`<` → `<=`, `+` → `-`) and checks
whether any test fails. A surviving mutant is a bug the suite would have
let ship — it measures exactly the "asserts behavior" quality a static
check can't. Like crap-metric with coverage, test-metric doesn't run the
mutation tool itself (slow, language-specific, and the target repo's CI
already owns it); it parses the tool's JSON report into one model and
gates on the score:

- **Go**: [go-gremlins](https://github.com/go-gremlins/gremlins) — `gremlins unleash --output gremlins.json`
- **TypeScript/JS**: [Stryker](https://stryker-mutator.io) — its
  `mutation.json` (`--reporters json`; the mutation-testing-elements schema)
- **Python / Swift / anything else**: a generic
  `{"mutants":[{"file","line","mutator","status"}]}` file, `status` one of
  `killed|timeout|survived|no_coverage|ignored`, so a small converter over
  mutmut/muter output feeds the same gate

The format is auto-detected. Score = `(killed + timeout) / (killed + timeout
+ survived + no-coverage)`; mutants with no verdict (compile errors, not
viable, skipped) are left out. `--covered-only` also leaves out
never-executed mutants so the score measures assertion strength alone —
line coverage is crap-metric's job. `--fail-below` defaults to `60`.
`--min-mutants N` (default `0`, always enforce) waives the gate when fewer than
`N` mutants were graded — a score over a handful of mutants is noise (one
survivor among 3 is 67%), so it passes with a note instead of failing on chance;
the count follows `--covered-only`, and the ratchet's scoped score gets the same
waiver. The
table lists surviving mutants (`file:line`, mutator). `--dir` relativizes
absolute paths in the report, and `--only-files` re-scores just the
touched files' mutants — the practical way to run this in PR CI, since a
full mutation run is slow but a per-changed-file one isn't.

`--minimum-graded N` is the strict evidence floor: fewer than `N` graded
mutants fails even if the score would otherwise be 100%. The default is
`0` for direct CLI compatibility; use `--minimum-graded 1` when CI requires
a mutation result. This differs from the older `--min-mutants` waiver,
which passes a small sample. When both are set, the strict floor wins.
With `--only-files` and a strict floor, pass `--dir` and `--lang` so the
tool can identify eligible changed source files. A docs-only change is
not applicable; a changed source file with no mutants is insufficient
analysis. The scoped evidence is printed separately, while JSON remains
the full report.

### dead-metric: is there code nothing uses?

Unused functions, exports and files are the cheapest simplification there is:
deleting them costs nothing at runtime and shrinks everything else's surface.
Like test-metric's mutation gate, dead-metric doesn't run a detector — reachability
analysis is language-specific and the target repo's CI already owns it. It parses
the tool's JSON report into one model and gates on the count:

- **Go**: [`deadcode`](https://pkg.go.dev/golang.org/x/tools/cmd/deadcode) —
  `deadcode -json ./... > deadcode.json`. Whole-program reachability from
  every `main`. Deliberately **without `-test`**: see the policy below. Generated
  files are skipped. A clean run prints a bare `null`, which is a valid empty report.
- **TypeScript/JS**: [`knip`](https://knip.dev) — `knip --production --reporter json > knip-report.json`
  (`--production` is the strict mode: it drops test files as entry points. knip exits 1 when it finds issues, so don't let that abort the step; and don't name the report `knip.json`, which is knip's own config filename and would be read as, or overwrite, the project's config). Unused
  files, exports, types, and enum/namespace members are read; unused *dependencies*
  are not (manifest hygiene, not dead code). knip needs its entry points configured
  (`knip.json`) to avoid reporting live code as dead. **Set
  `"ignoreExportsUsedInFile": true`**: by default knip calls an export unused when
  only its own file uses it, which is a live symbol with a needless `export`, not
  dead code. On a real React app (d_amp_d) that one setting took 32 findings to 8,
  and the 8 were genuine: unused re-export shims and a hook nothing called.

- **Python**: [`vulture`](https://github.com/jendrikseipp/vulture) —
  `vulture --min-confidence 80 src > vulture.txt || true` (vulture exits 3 when it
  finds anything). Text output only; imports, functions, classes, methods,
  variables/attributes, and unreachable/unsatisfiable code are read. **Always pass
  `--min-confidence 80`**: vulture's default 60% tier guesses at anything not called
  by name, and frameworks call everything by registration. On a real FastAPI app
  (grounded) it flagged 225 symbols at 60% — every route handler, ORM column, pydantic
  field and enum member, all live — and 0 at 80%+. What survives 80% (unused imports,
  unused arguments, unreachable code) is nearly always real. Use vulture's
  `--ignore-decorators` / `--ignore-names` for the rest, or this tool's `--ignore`.
  **A clean vulture run prints nothing**, and an empty file is indistinguishable from a
  step that never ran, so auto-detect refuses it: pass `--format vulture` to accept an
  empty report as a clean pass.

- **Swift**: [`periphery`](https://github.com/peripheryapp/periphery) —
  `periphery scan --project App.xcodeproj --schemes App --format json --quiet > periphery.json`
  (needs `xcodebuild`, so the mac-mini runner; about 10 s per app, and it also scans
  the app's local SwiftPM packages). Only results hinted `unused` are read.
  `assignOnlyProperty` is dropped on purpose: on a persisted model (Codable,
  SwiftData) a field written but never read in code is usually read by the coder or
  the database — 23 of the 46 results on health-suite's seven apps, none obviously
  dead. `redundantPublicAccessibility` is API tidiness, not dead code. Periphery
  results carry absolute paths, so pass `--dir` (the directory your `--only-files`
  list is relative to).

**Policy: code only tests use is dead.** Every language is run in its strict mode
(no `deadcode -test`, `knip --production`, vulture over the app paths only, Periphery
over the app scheme). A function nothing but its own tests calls is a feature that
isn't there: delete it and its tests together, and if it ever comes back, write its
spec and tests again from scratch rather than resurrecting tests for code no one asked
for. The cost is that a core library's tested-but-unused helpers get flagged — that is
the point.

```
dead-metric --report deadcode.json [--format deadcode|knip|vulture|periphery] [--dir DIR] [--ignore FILE] [--fail-above N] [--top N] [--only-files PATH] [--json PATH]
```

The format is auto-detected (only an empty vulture report needs `--format`). `--only-files` narrows the gate, never the report,
like every other tool. `--dir` relativizes absolute paths in the report.

**`--ignore FILE`** is the allowlist for code that is live in fact but dead to the
analyzer — reflection targets, plugin entry points, a library's public API.
Without it the false positives get the gate switched off. One entry per line,
`#` comments allowed:

```
Registered            # a symbol name (also covers the method Thing.Registered)
internal/gen/         # a path prefix
**/*_gen.go           # a path glob (`**/` = any depth, `*` never crosses `/`)
*.pb.go               # a glob with no `/` matches the base name
```

Ignored findings are dropped from both the report and the gate.

## Architecture

```
cmd/
  crap-metric/    dupe-metric/    escape-metric/    cycle-metric/    arch-metric/    test-metric/    dead-metric/
internal/
  ratchet/                        # shared --only-files primitives
  gomod/                          # nearest-go.mod lookup, shared by crap's Go analyzer and arch's Go importer
  swiftlex/                       # native Swift lexer, shared by analyzers/tokenizers/testscanners below
  crap/           analyzers/{golang,python,typescript,swift}
  dupe/           tokenizers/{golang,python,typescript,swift}
  escape/
  cycle/          importers/{python,typescript}          # no Go, no Swift — see README above
  arch/           importers/{python,typescript,golang}   # reuses the two above unchanged, adds Go — no Swift
  testmetric/     testscanners/{golang,python,typescript,swift}  # static test-quality checks
  mutation/                                                # mutation-report parsing + score gate
  deadcode/                                                # deadcode/knip/vulture/periphery report parsing, ignore list, count gate
testdata/
  crap/{golang,python,typescript,typescript-ts7,javascript,swift}/
  dupe/{golang,python,typescript,typescript-ts7,javascript,swift}/
  escape/{golang,python,typescript,swift}/    # .js/.jsx fixtures live alongside typescript's — same "ts" language block
  cycle/{python,typescript,javascript}/
  arch/{golang,python,typescript}/  arch/golang-stability/   # separate dir: stability's own package graph, kept isolated from check's
  test/{golang,python,typescript,swift}/  test/mutation/          # deliberately-bad tests; sample mutation reports
  dead/{deadcode.json,knip.json,vulture.txt,periphery.json}  dead/{golang,python}/                    # real tool output, captured (not hand-written)
```

Each tool's domain package (`crap`, `dupe`, `escape`, `cycle`, `arch`) and
language-adapter tree (`analyzers`, `tokenizers`, `importers`) stay
separate — their data models are genuinely different, so unifying them
would be forced, not real deduplication. Only `internal/ratchet` (truly
byte-for-byte identical across all four before this migration),
`internal/gomod`, and the CI/release pipeline were actually shared.
`internal/importers/golang` lives under the same shared `importers` tree
as python/typescript (it implements the identical `importers.Importer`
interface, returning the same `cycle.Graph` shape) even though only
arch-metric wires it up — cycle-metric's own `main.go` still explicitly
rejects `--lang go`, same as before.

`testdata` is namespaced per tool (`testdata/crap/...`,
`testdata/dupe/...`, etc.) since each tool's fixtures have different,
tool-specific content even where the file names match (e.g. every
tool has its own `golang/sample.go`).

## Status

CI (`.forgejo/workflows/ci.yml`) builds/vets/tests the whole module on
every push and PR; on `main`, it also builds all seven `linux/amd64` and
`darwin/arm64` binaries (the latter for the mac-mini runner), each with
`-ldflags "-X main.version=<short-sha>"` baked in, and publishes them to
**one** Forgejo release tagged with that same short commit SHA — pure
distribution, no report data attached, so the release job also prunes
releases down to the newest 20 after each push. It then self-checks
crap-metric/dupe-metric/escape-metric/arch-metric/test-metric/dead-metric
against the repo's own (now much larger, all-seven-tools) Go source —
cycle-metric still can't self-check, being Go-only in implementation but
not supporting Go analysis; arch-metric *can* (it's the one tool besides
crap/dupe/escape whose Go support isn't a no-op), gated by this repo's own
`.arch-metric-rules.json` at the module root — both its `check` and
`stability` gates run self-checked, the latter needing no rules file at
all. crap-metric's self-check
trend-diffs against the previous run's report, read from (and then
advanced on) a dedicated `reports` branch — one commit per run,
`crap-report.json` only — rather than a release asset.

Consumed by `d_amp_d`, `grounded`, and `health-suite` via `ci-workflows`'
`install-quality-gates` action, which installs all seven release binaries;
that action's optional `version` input pins to a specific release tag instead
of always floating on `latest`.

## Development

```
go build ./...
go vet ./...
go test ./...
```

The analyzer/tokenizer tests exercise the real underlying tools, not
mocks — `radon` and `coverage` (Python, for crap-metric) and `node` +
`typescript` (in each
`testdata/{crap,dupe}/{typescript{,-ts7},javascript}/node_modules` and
`testdata/test/typescript/node_modules`,
`npm install` there if missing) need to be available locally to run the
full suite. `python3` alone (stdlib `tokenize`, no pip package) covers
dupe-metric's and cycle-metric's Python fixtures. Swift needs nothing
extra to install — `internal/swiftlex` is pure Go, like the Go adapters
themselves.
