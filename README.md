# quality-gates

Five CI quality gate CLIs, one Go module: **crap-metric**, **dupe-metric**,
**escape-metric**, **cycle-metric**, and **test-metric**. Each answers a different
question about a change, with a deliberately different, appropriately-
scoped detection method — but they share enough (the `--only-files`
ratchet mechanism, the CI/release plumbing, the install pattern) that
running them as four separate repos meant four copies of that shared
logic drifting independently. Migrated into one module for exactly that
reason — see [CLAUDE.md](CLAUDE.md) for the incidents that motivated it.

## The five gates

| Binary | Question | Method | Default gate |
|---|---|---|---|
| `crap-metric` | Is this function complex *and* undertested? | Parses each language for cyclomatic complexity + coverage, combines via `complexity² × (1−coverage)³ + complexity` | `--fail-above 30` |
| `dupe-metric` | Is this code duplicated? | Tokenizes source, finds exact-match blocks via greedy leftmost-longest shingling | `--fail-above 5` (%) |
| `escape-metric` | Did this code opt out of type-checking/linting/error handling? | Regex-matches suppression comments and a few high-confidence whole-line patterns | `--fail-above 1` (per 1000 lines) |
| `cycle-metric` | Are these modules structurally tangled? | Regex-extracts imports, resolves to files, runs Tarjan's SCC | `--fail-above 0` (cycles) |
| `test-metric` | Can these tests actually fail? | `check`: parses test files for tests with no assertions, skips, `.only`, mock-only assertions, leaked temp dirs. `mutation`: gates on a mutation tool's score | `check --fail-above 0` (findings); `mutation --fail-below 60` (%) |

The first four support `python` and `ts`/`typescript`/`js`; `crap-metric`,
`dupe-metric`, and `escape-metric` also support `go` and `swift`.
`test-metric check` supports all four (`go`, `python`, `ts`/`js`, `swift`).
`cycle-metric` deliberately excludes both: Go's compiler already refuses
to build a package-import cycle, so a detector for it would always
report zero; Swift files within one module never import each other at
all (no per-file import graph exists the way Python/TS have one), and
cycles between separate modules would need `Package.swift`-level
resolution this tool doesn't implement — see `--lang swift`'s own error
message and CLAUDE.md's Swift entry for the full reasoning. See each
package's doc comment (`internal/crap`, `internal/dupe`, `internal/escape`,
`internal/cycle`) for the full reasoning and known characteristics of its
method.

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
`cycle.Cycle` counts as touched if *any* file in it matches, `test-metric
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
crap-metric   check --lang <python|go|ts|swift> --dir <dir> [--coverage PATH] [--fail-above N] [--verbose] [--only-files PATH] [--exclude GLOB]... [--exclude-file PATH] [--json PATH]
crap-metric   diff  --old PATH --new PATH [--top N] [--json]
dupe-metric   check --lang <python|go|ts|swift> --dir <dir> [--min-tokens N] [--fail-above PCT] [--only-files PATH] [--json PATH]
escape-metric check --lang <python|ts|swift> --dir <dir> [--fail-above RATE] [--only-files PATH] [--json PATH]
cycle-metric  check --lang <python|ts> --dir <dir> [--fail-above N] [--only-files PATH] [--json PATH]
test-metric   check --lang <go|python|ts|swift> --dir <dir> [--fail-above N] [--min-assertions N] [--ignore KIND,...] [--include KIND,...] [--only-files PATH] [--json PATH]
test-metric   mutation --report PATH [--dir <dir>] [--fail-below PCT] [--covered-only] [--only-files PATH] [--json PATH]
<any tool>    version
```

Every `check` also takes `--top N` (rows to print, default 20, `0` = all).
`version` prints the build's short commit SHA (`dev` for a plain
`go build`/`go run`, since it's baked in via `-ldflags` — see Status).

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
line coverage is crap-metric's job. `--fail-below` defaults to `60`. The
table lists surviving mutants (`file:line`, mutator). `--dir` relativizes
absolute paths in the report, and `--only-files` re-scores just the
touched files' mutants — the practical way to run this in PR CI, since a
full mutation run is slow but a per-changed-file one isn't.

## Architecture

```
cmd/
  crap-metric/    dupe-metric/    escape-metric/    cycle-metric/    test-metric/
internal/
  ratchet/                        # shared --only-files primitives
  swiftlex/                       # native Swift lexer, shared by analyzers/tokenizers/testscanners below
  crap/           analyzers/{golang,python,typescript,swift}
  dupe/           tokenizers/{golang,python,typescript,swift}
  escape/
  cycle/          importers/{python,typescript}          # no Go, no Swift — see README above
  testmetric/     testscanners/{golang,python,typescript,swift}  # static test-quality checks
  mutation/                                                # mutation-report parsing + score gate
testdata/
  crap/{golang,python,typescript,typescript-ts7,javascript,swift}/
  dupe/{golang,python,typescript,typescript-ts7,javascript,swift}/
  escape/{golang,python,typescript,swift}/    # .js/.jsx fixtures live alongside typescript's — same "ts" language block
  cycle/{python,typescript,javascript}/
  test/{golang,python,typescript,swift}/  test/mutation/          # deliberately-bad tests; sample mutation reports
```

Each tool's domain package (`crap`, `dupe`, `escape`, `cycle`) and
language-adapter tree (`analyzers`, `tokenizers`, `importers`) stay
separate — their data models are genuinely different, so unifying them
would be forced, not real deduplication. Only `internal/ratchet` (truly
byte-for-byte identical across all four before this migration) and the
CI/release pipeline were actually shared.

`testdata` is namespaced per tool (`testdata/crap/...`,
`testdata/dupe/...`, etc.) since each tool's fixtures have different,
tool-specific content even where the file names match (e.g. every
tool has its own `golang/sample.go`).

## Status

CI (`.forgejo/workflows/ci.yml`) builds/vets/tests the whole module on
every push and PR; on `main`, it also builds all five `linux/amd64` and
`darwin/arm64` binaries (the latter for the mac-mini runner), each with
`-ldflags "-X main.version=<short-sha>"` baked in, and publishes them to
**one** Forgejo release tagged with that same short commit SHA — pure
distribution, no report data attached, so the release job also prunes
releases down to the newest 20 after each push. It then self-checks
crap-metric/dupe-metric/escape-metric/test-metric against the repo's own (now much
larger, all-five-tools) Go source — cycle-metric still can't self-check,
being Go-only in implementation but not supporting Go analysis.
crap-metric's self-check trend-diffs against the previous run's report,
read from (and then advanced on) a dedicated `reports` branch — one
commit per run, `crap-report.json` only — rather than a release asset.

Consumed by `d_amp_d`, `grounded`, and `health-suite` via `ci-workflows`'
`install-quality-gates` action (which replaced four separate
`install-*-metric` actions; it still needs a one-line addition to fetch
`test-metric`, which the release job already publishes); that action's optional `version` input pins
to a specific release tag instead of always floating on `latest`.

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
