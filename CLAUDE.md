# quality-gates

Seven CI quality gate CLIs (Go), one module, for repos on
`git.roost-r.com/cadeh`: `crap-metric` (complexity × undertested),
`dupe-metric` (duplication), `escape-metric` (suppressed checks),
`cycle-metric` (import cycles), `arch-metric` (declared layer rules),
`test-metric` (test quality), `dead-metric` (unused code). See
[README.md](README.md) for the formula/algorithm, CLI usage, and
architecture.

This repo is a 2026-09-05 merge of four previously-standalone repos of
the same names. They stayed separate as long as they had genuinely
separate lessons to learn; migrated once the thing they had in common
(the `--only-files` ratchet, the CI/release pipeline, the install
action) was drifting across four copies instead of living in one place.
The lessons below are inherited from all four histories — a lesson
learned once in `escape-metric` didn't need re-learning in
`cycle-metric` because `cycle-metric` was built after, reading this file
first. Keep doing that: read this before adding an eighth tool
(`arch-metric` was the seventh, added post-merge, entirely inside this
repo — see its own entry below for what it inherited from `cycle-metric`
by doing exactly that) or a new language adapter to an existing one.

## Keep the README current

Whenever a CLI's flags, an adapter's behavior/requirements, or the
shared `internal/ratchet` contract changes, update `README.md` in the
same change. It's the only doc a consuming repo's CI setup is written
against.

## `--only-files` narrows the gate, never the report

Every tool's full, unfiltered report is always what gets printed and
written to `--json` — `--only-files` only changes which subset the exit
code is computed from. This was a real mid-build correction in
crap-metric (the first implementation filtered functions *before*
building the report, silently hiding pre-existing hotspots from the
output entirely) and is now enforced once, structurally, by
`internal/ratchet` only ever handing back a filtered *subset* for a
tool's CLI to build a second, separate report from — never a mutated
primary report.

## A tool's per-item `File` field must be relative to `--dir`, always

`internal/ratchet.Matches` does suffix matching against a changed-file
list that's repo-root-relative. Every adapter (`analyzers`,
`tokenizers`, `importers`) must resolve its own `File` values relative
to `--dir` (`filepath.Rel`/`os.path.relpath`/`path.relative`), not
whatever the raw walk produced — otherwise ratchet matching silently
fails whenever `--dir` itself has `..` components. This was a real bug,
independently, in both dupe-metric's tokenizers and escape-metric's
`Scan` (found via a failing test using `--dir "../../testdata/..."`).
`cycle-metric`'s importers got this right from day one *because* they
were built after dupe-metric's and escape-metric's fixes existed to copy
the pattern from. Any new adapter must keep doing this.

## escape-metric's ratchet needs its own line-count denominator

`escape.Result.LinesByFile` exists specifically so the ratchet's scoped
rate divides by *touched files'* own line count, not the whole-repo
`TotalLines` — using the whole-repo total would dilute a small change's
hatches into a rate too tiny to ever trip `--fail-above`, so the ratchet
would exist but never actually gate anything. Any tool whose gate is a
*rate* rather than a raw count needs the same care: the ratchet's
denominator must scope down with its numerator.

## Adapter/tokenizer/importer tests use the real tools, not mocks

`internal/analyzers/*`, `internal/tokenizers/*`, and
`internal/importers/*` all test against the actual underlying
tool — `radon`, `go/parser`/`go/scanner`, the target repo's own
`typescript` package, Python's stdlib `tokenize` — not hand-picked
fixture output. Keep it that way: mocking the underlying tool's output
format is exactly the kind of assumption that silently drifts from
reality when that tool's output shape changes.

## dupe-metric self-check threshold: 18%, not the tool's own default of 5%

Right after the four-repo merge, self-checking dupe-metric against this
repo's own combined Go source came back at 17.08% duplication — a real
CI failure caught the same day this repo was created, not a hypothetical.
Two genuinely identical blocks got extracted as a result — `--only-files`
loading/error-handling (`ratchet.Load`, replacing an identical 7-line
block copy-pasted into all four `cmd/X/main.go` files) and each Report
type's JSON encode/decode + Passed→exit-code mapping (`internal/reportio`,
replacing 4× identical `WriteJSON`/`ReadReport`/`ExitCode` bodies) — both
real fixes, not threshold-dodging, and both left the tool strictly
smaller and more correct. That brought it down to ~15%, not below 5%.

The rest is cross-tool *structural* similarity, not copy-paste: parallel
Python/TypeScript adapters across `analyzers`/`tokenizers`/`importers`
(same task shape — run an embedded script, parse its JSON output — for
a different purpose each), and each tool's `WriteTable` following the
same "summary line, then a row loop, then a truncation note" shape with
genuinely different columns. Forcing these into one generic abstraction
would be exactly the kind of unification this file already warns
against elsewhere ("their data models are genuinely different, so
unifying them would be forced, not real deduplication") — so the
self-check's `--fail-above` was raised to 18 instead, with this entry as
the paper trail for why. If a future change pushes it meaningfully
higher than that, look for *actual* copy-paste first (the ratchet/
reportio pattern) before just raising the number again — this exception
is for real structural echoes from the four-way merge, not a blank
check for any future duplication in this repo.

## dupe-metric: "boundary bleed" is expected, don't special-case it away

`internal/dupe/shingle.go`'s `Find` can report a duplicate block a token
or two longer than a human would draw the boundary, when trailing/
leading content past the true shared block happens to coincide too
(documented in README, asserted by `TestFindSameFileBoundaryBleed`).
Inherent to matching raw tokens instead of AST/statement boundaries —
PMD-CPD and jscpd have the same property. Don't add a language-specific
heuristic ("stop at a closing brace") to suppress it; that trades a
well-understood property for fragile per-language special-casing. The
one thing that *was* a real bug and got fixed: `extend()` could grow a
same-file match until its range overlapped the range it was matching
against; `maxExtend` caps this (`TestFindSameFilePreventsOverlap`).

## escape-metric: regex-only is a deliberate v1 scope

Every pattern in `internal/escape/patterns.go` is a comment marker or a
whole-line shape chosen specifically because it's precise without a real
parser. Don't add a pattern needing semantic context to avoid false
positives (e.g. "any type usage") without also adding AST machinery to
back it — a regex guess at that misfires constantly on identifiers and
string contents, worse than not having the check. When adding a
pattern, add a matching line to the relevant `testdata/escape/<lang>`
fixture and a real test asserting the count, not just a regex that looks
right: `discarded-result` (Go) was first written as `\w+\(...\)`, which
compiled fine but silently missed every package-qualified call
(`fmt.Sprintf(...)`) since `\w+` doesn't match a dot — caught only by
running it against a fixture with a qualified call.

**Known limitation, not a bug**: a comment *discussing* a marker
triggers it too — regex can't tell "this line documents the pattern"
from "this line suppresses something". Caught by escape-metric's own
first CI run, when its own doc comment's `"// nolint"` example matched
its own `nolint` regex. Fixed by rephrasing the comment, not narrowing
the regex (narrowing it would just create a different, more fragile
blind spot). A docs-heavy file failing this gate isn't automatically a
real suppression — check before assuming.

## cycle-metric: regex-only import extraction, verify against a real cycle

Same reasoning as escape-metric: per-language import syntax is regular
enough to extract without a real parser; the hard part is resolving a
specifier to a scanned file, which needs care regardless of how the
specifier was extracted. A future improvement (multi-line parenthesized
Python import lists, tsconfig path aliases) should extend the existing
resolver, not reach for a full parser unless accuracy actually needs
one. Build any new resolution rule against a real fixture with a genuine
cycle (`testdata/cycle/{python,typescript}/pkg/{a,b}`), verified
end-to-end, before writing the formal test — not by reasoning about the
regex in the abstract. Python's resolver went through a real bug this
way: the first version only resolved the module part of
`from X import Y`, missing the case where `Y` itself is the target file
(`from . import b`, module empty, `b` the imported name).

## arch-metric: reuse cycle-metric's importers, add Go for the opposite reason cycle-metric excludes it

`arch-metric` checks declared layer rules ("domain may not import infra",
"cmd/* may not import each other") against the real import graph — the
Dependency Inversion Principle as a gate, and a real dogfood: this repo's
own `.arch-metric-rules.json` gates `cmd/*` staying independent and
`internal/*` never depending on any `cmd/*` binary, both self-checked in
CI (`.forgejo/workflows/ci.yml`'s `self-check` and `self-check-pr` jobs).

**python's and typescript's importers are used completely unchanged.**
`internal/arch.EdgesFromFileGraph` reads a `cycle.Graph` — the exact
value `python.Importer`/`typescript.Importer` already return, untouched —
and derives each edge's package from `filepath.Dir` on either side. No
importer code for those two languages changed at all; only a
new consumer was written for their existing output. That's the reuse the
top-level design note promised, and it worked out exactly as expected:
zero changes needed in either importer.

**Go needed a new importer precisely because cycle-metric doesn't have
one — and for the mirror-image reason.** `cycle-metric --lang go` is
unsupported because the compiler already refuses to build an import
cycle, so a cycle detector for Go would always report zero. Layering is
different: the compiler has no opinion on it at all, so a perfectly
acyclic, perfectly buildable Go program can still violate a declared
boundary — which makes Go the language where this tool's Go support
matters *most*, not least. `internal/importers/golang` fills that gap via
`go/parser`'s import-only mode plus the enclosing module's own `go.mod`
(module-path resolution factored out to `internal/gomod`, shared with
crap-metric's Go analyzer, once it became clear both needed the identical
nearest-go.mod walk — the same "extract on the second real occurrence"
call as `internal/ratchet`/`internal/reportio` during the four-repo
merge).

**Go's import unit is the package, not the file — reconciled by fan-out,
not by inventing a new node shape.** Python/TS produce genuine
file-to-file edges because that's what their import statements name.
Go's `import` statement names a whole package with no per-file
granularity at all. Rather than let a directory string masquerade as a
`cycle.Graph` node (which is documented as "every edge points at a real
file" — a Go directory violates that contract, and a future reader
diffing the three importers' node semantics would have no way to tell
that was intentional), a resolved Go import fans out to *every other
scanned file in its target package's directory*: a file importing a
package structurally depends on every file that makes up that package.
This keeps `cycle.Graph`'s own contract identical across all three
importers, so `EdgesFromFileGraph` needed no Go-specific branch at all —
just like reusing the graph type promised.

**Rule patterns reuse `internal/exclude`'s glob engine unchanged, not a
new one.** A rule's `from`/`deny` values are matched with the exact same
`exclude.Compile`/`Set.Matches` that `--exclude` and dead-metric's
`--ignore` already use. This wasn't just convenience: it exercised an
already-tested property that turned out to be exactly the boundary
semantics a layer rule needs for free — a pattern with no wildcard
(`"internal/domain"`) already matches that directory *and everything
below it*, because `exclude`'s glob-to-regexp compiler appends
`(?:/.*)?$` to every compiled pattern (originally so `--exclude
internal/gen` drops a whole tree, not just files literally at that
path). Writing a second, arch-specific pattern matcher would have had to
re-derive that same "directory match implies subtree match" behavior
from scratch, with its own new edge cases to get wrong.

## arch-metric grew three follow-ups without becoming a framework

`stability`, exceptions, and `diff` were added deliberately staying
inside "one narrow structural check with a binary answer" (see the top
of this entry) rather than reaching for a richer rule language — the
line that separates this tool from something like ArchUnit.

**`arch-metric stability` reuses the same edge graph `check` already
builds — no new importer, no new graph type.** It computes Robert
Martin's afferent/efferent coupling and instability (`Ce/(Ca+Ce)`) per
package and flags any edge from a more stable package into a less stable
one (the Stable Dependencies Principle: "depend in the direction of
stability"). The one real subtlety: Martin's metrics are a *package*
graph property, but `internal/importers/golang` fans a single package
import out to many file-level edges (see above) — computing Ca/Ce
straight from raw edges would inflate a package's coupling by its
target's file count instead of by how many packages it actually depends
on. `internal/arch/stability.go`'s `packagePairs` collapses to one edge
per unique `(FromPkg, ToPkg)` pair before computing anything, which is
also why it's a top-level helper `CheckStability` shares with `Check`
rather than being folded into `Report` — it's a graph-level operation,
not a per-rule one. Caught by writing
`TestStabilitiesDedupesFanOutToOnePackagePair` *before* the
implementation (this whole feature set was built test-first): the naive
per-edge version passed every test until that one, which was written
specifically because the Go fan-out design decision above was already
known to be a landmine for exactly this kind of downstream counting.

**Exceptions are scoped to one named rule, not a bare from/to pattern
pair.** `{"rule": "...", "from": "...", "to": "..."}` — the `rule` field
is required and matched exactly. A bare pattern pair with no rule name
would silently exempt that package pair from every rule that happens to
match it, including ones added to the file later; scoping to a rule name
makes an exception's blast radius exactly as wide as the person who
wrote it could see at the time. It drops a matching violation from both
the report and the gate — the same contract as dead-metric's `--ignore`
and crap-metric's `--exclude`, deliberately different from `--only-files`
(which only narrows the gate, never the report).

**`arch-metric diff` mirrors crap-metric's `diff` but is simpler, because
a layer violation has no continuous score.** crap-metric's `Delta` has a
"changed" case (a function's CRAP score moved between two runs);
`arch.DiffEntry` only ever has "new" or "fixed", because a violation
either exists or it doesn't. Matched by `(rule, file, import)` rather
than crap's `(file, name)` — the extra `rule` key matters here because
the same file+import pair can appear under two different rules
simultaneously (e.g. one rule about `cmd/*` independence and another
about `internal/*` layering, both tripped by the same edge), which
would collide under a two-part key.

**Building all three surfaced a real, self-inflicted duplication bug
before it ever reached the self-check gate.** Running `dupe-metric`
against the finished feature set (a habit worth repeating any time new
CLI subcommands or report types get added — see "CI self-checks this
repo" below) found three genuine repeats: `cmd/arch-metric/main.go`'s
`runCheck` and `runStability` shared a same-file block from copy-pasting
one to write the other; `filterForRatchet`/`filterStabilityForRatchet`
in the same file were byte-for-byte identical except for the violation
type; and `Report.WriteTable`/`StabilityReport.WriteTable` shared their
entire truncation-and-verdict tail. The first was left alone — same
judgment call as the dupe-metric self-check threshold entry below: two
subcommands of one tool sharing a skeleton is exactly the kind of
structural echo forcing into one function would obscure, not clarify,
and `test-metric`'s own `check`/`mutation` split already establishes that
precedent. The other two were real, mechanical, same-file duplication
with no domain reason to differ, so they got a real fix: a generic
`filterForRatchet[T any]` (parameterized on two accessor funcs, same
shape as `reportio.WriteJSON[T any]`), and `internal/arch/table.go`'s
`truncateRows[T any]`/`writeGateVerdict` shared by both `WriteTable`s and
`WriteDiffTable`. Bringing this repo's overall duplication from 14.73%
back down to 14.06% — comfortably under the self-check's 18% ceiling
either way, but fixed anyway because it was real, not because the gate
demanded it.

**A fixture placement mistake, caught by its own test, not by review.**
The `stability` fixture was first written nested inside
`testdata/arch/golang/` (alongside `check`'s domain/infra/cmd fixture) —
and `TestRunStabilityNoViolationsOnLayerFixture` immediately failed,
because `--dir testdata/arch/golang` now recursively picked up the new
`stability/` subtree too, producing a real (if accidental) violation from
the combined graph. Moved to a sibling `testdata/arch/golang-stability/`
directory instead. The lesson generalizes: any fixture directory nested
under another tool invocation's `--dir` is implicitly part of that
invocation's input, Go's own `testdata`-name exclusion notwithstanding —
new fixtures need a directory boundary check against every existing
`--dir` argument that could recursively contain them, not just a
uniqueness check against sibling fixture names.

## Swift support: native lexer shared by two tools, cycle-metric excludes it

`internal/swiftlex` is a hand-written, pure-Go lexer for Swift — no
subprocess, no Swift toolchain dependency for the analysis itself (only
for coverage *generation*, done by the target repo's own CI, same as
every other language). It's shared by `internal/analyzers/swift`
(crap-metric) and `internal/tokenizers/swift` (dupe-metric), the same way
Go's own analyzer/tokenizer both sit on stdlib `go/*` packages — except
here we're writing the "stdlib" ourselves, since Go has none for Swift.

Three things were deliberately left unbuilt rather than guessed at with a
heuristic, each documented in `internal/swiftlex`'s package doc comment
rather than solved:

- **Regex literals** (`/pattern/`, Swift 5.7+) aren't supported — `/` is
  always lexed as division. A regex-vs-division heuristic's failure mode
  isn't a miscounted token, it's corrupted brace-depth tracking for the
  rest of the file (function-boundary detection depends on it), which is
  worse than under-supporting a rare literal form.
- **String interpolation** (`"\(expr)"`) contents aren't tokenized
  separately — the whole literal, interpolation included, is one opaque
  token. The lexer still tracks interpolation nesting internally so the
  literal's own boundaries are always found correctly (including when a
  nested interpolated expression contains its own string with its own
  quotes) — it just doesn't emit separate tokens for what's inside.
- **Ternary `cond ? a : b`** isn't counted toward cyclomatic complexity —
  lexically indistinguishable from optional chaining/optional-type `?`
  without real parsing.

The sharpest correctness case in `internal/analyzers/swift` wasn't lexing
at all, it was **nested local `func`** (a real Swift feature Go has no
equivalent of): a naive keyword-triggered boundary detector emits a
second, overlapping `crap.Function` for it. The fix mirrors how Go's own
analyzer already avoids the equivalent problem — `go/ast.Inspect(fn, ...)`
walks nested `FuncLit`s into the same `FuncDecl`'s complexity count rather
than emitting them separately — by only looking for new function starts
at the top level (outside any function body already being scanned), so a
nested `func`'s branches fold into the enclosing function automatically,
with no special-casing needed. Computed-property accessors
(`get`/`set`/`willSet`/`didSet`) are the other side of the same v1 scope
choice: their bodies are brace-tracked over (so they can't corrupt
anything) but never emitted as their own entry, so their complexity is
simply invisible rather than misattributed.

**cycle-metric deliberately excludes Swift too, for a different reason
than Go.** Go's exclusion is "the compiler already forbids it, so the
check would always report zero." Swift's is structural: files within one
module never import each other at all — there's no per-file import
statement to extract the way Python/TS have one, so file-level cycle
detection would find nothing for a typical single-target app. Real cycles
could only exist between separate SPM modules (`import OtherModule`),
which would need `Package.swift`-level target-to-directory resolution —
and `internal/ratchet`'s `--only-files` matching is suffix-based against
real file paths, so module-granularity graph nodes wouldn't plug into the
existing ratchet mechanism without extending it. Decided not to build
that for v1; `cmd/cycle-metric/main.go`'s `importerFor` returns an
explicit explanatory error for `--lang swift`, same pattern as its `"go"`
case, rather than falling through to the generic "unknown language"
message.

## test-metric: static checks read facts, a mutation report is the real answer

`test-metric check` exists because coverage can't see a test that asserts
nothing. Scanners (`internal/testscanners/*`) only extract per-test *facts*
(`testmetric.Test`); every rule and threshold lives once in
`internal/testmetric`. Scanners use real parsers (`go/parser`, Python `ast`,
the target's `typescript`), consistent with the "tests use the real tools"
rule above. Lessons from building it, so they aren't re-learned:

- **Prefer false negatives to false positives on "no assertions".** A gate
  that cries wolf gets ignored. So assertion detection is generous
  (assertion-shaped helper names count), and in Go a same-package helper
  taking `*testing.T` counts when its *body* fails `t` (`assertingHelpers`,
  fixpoint over helpers-of-helpers). This came from dogfooding: the
  repo's own `TestAnalyzeExtensionQualifiesName` asserts only through
  `findFunc(t, ...)`, which a naming-only heuristic flagged. Same lesson as
  above: run a new check against this repo before trusting it.
- **Only *unconditional* skips are findings.** `t.Skip` inside an `if`/
  `switch`, `skipif`, `test.skipIf` are environment guards (this repo's own
  tests skip when `node`/`radon` is missing — CI separately
  asserts no skips fire). Flagging them would fail every repo with a
  legitimate tool-missing guard.
- **Dogfooding crashed the first Go scanner:** `ast.Inspect` calls back with
  a nil node after a node's children, and a `switch`'s absent `Init`/`Tag`
  are nil interfaces; walking those panicked. Regression fixtures for
  switch/type-switch/select live in `testdata/test/golang`.
- **Mutation testing is ingested, not run.** Same stance as crap-metric with
  coverage: the target repo's CI owns the slow, language-specific tool
  (go-gremlins, Stryker); `internal/mutation` parses its JSON into one model.
  The report-format structs were written from those tools' documented
  schemas, and the fixtures in `testdata/test/mutation` are hand-written to
  match — *not* captured from a real run. Verify against a real
  gremlins/Stryker report before trusting a new field, and add it to the
  fixture then.
- **Swift shares `swiftlex`'s function-boundary logic.** `FindFuncBodyOpen`/
  `MatchBrace` were lifted out of `internal/analyzers/swift` into
  `internal/swiftlex` (with their tests) so the analyzer and the test scanner
  can't drift apart on what a function body is. Swift's `throw XCTSkip` guard
  detection is brace-context tracking (a `{` after `if`/`guard`/`else`/
  `switch` marks a guard) — a heuristic, but a wrong guess only ever
  under-reports a skip. A real gotcha found by the fixture: a scan loop that
  jumps past each func's body also jumps past its *name* token, so
  file-level facts keyed on a function's name (`tearDown`) must be recorded at
  the `func` keyword, not by matching tokens in the outer loop.
- **Skipped/Suite subtleties:** a skipped test's body never runs, so its
  assertion count is not reported (no double finding); a `describe`-level
  block only carries skip/focus, never assertion checks.

- **Two false-positive classes surfaced by the first real-app run
  (d_amp_d's 1774 vitest/Playwright tests) — run a new check on a real
  codebase before shipping it as a default.** (1) `interaction-only` flagged
  345 tests: in a React app, `expect(onChange).toHaveBeenCalledWith(...)` on a
  callback prop *is* the behavior, and statically a collaborator mock and an
  output callback look identical. It is now opt-in (`testmetric.OptIn`,
  `--include`). (2) The TS scanner treated Playwright's `test.afterAll(...)`/
  `beforeEach`/`test.step(...)` as tests, reporting an anonymous "no-assertions"
  test; only `test.<skip|only|todo|fixme|each|…>` (`TEST_MODS`) are test
  declarations now. Both have regression fixtures.

## dead-metric: ingest `deadcode`/`knip`/`vulture`/`periphery`, capture their real output

Same stance as mutation testing: the detector is someone else's mature
tool, run by the target repo's CI; `internal/deadcode` only parses. Unlike
the mutation fixtures, `testdata/dead/{deadcode.json,knip.json,vulture.txt,periphery.json}` are **real
captured output** (from `testdata/dead/golang` and a throwaway knip
project), not hand-written from docs — and capturing them paid off
immediately: dogfooding on this repo showed `deadcode -json` prints a bare
`null`, not `[]`, when nothing is dead, which no docs-derived fixture
would have had. Regenerate them from the tools, don't edit them.
**Strict on purpose: code only tests use is dead** (no `deadcode -test`,
`knip --production`). The first self-check under it flagged
`dupe.ReadReport`, a one-line wrapper only its own round-trip test called —
deleted, with the test pointed at `reportio.ReadReport` directly. Ignore-list entries exist for reflection/plugin
targets; add one with a comment saying *why* it's live, never to make a
real finding go away.

Run each ingested tool on a real codebase before recommending a
configuration — the defaults are wrong in a way only that shows. knip on
d_amp_d: 32 findings by default, 8 real with `ignoreExportsUsedInFile`.
vulture on grounded's FastAPI backend: 225 findings at its default 60%
confidence, *every one* a framework-registered route/column/field, and 0 at
`--min-confidence 80`. Both fixes live in the README, not in code: the
tools own their thresholds. And vulture prints nothing when clean, so an
empty report can't be told from a step that silently didn't run (CI has to
swallow its exit code 3): auto-detect rejects empty input and
`--format vulture` opts in, rather than a gate that passes on a broken step.
Periphery's JSON is a top-level array like `deadcode`'s, so a misdetected one
would parse to zero findings and pass silently — `Parse` tells them apart by the
`hints` key (tested). Its `assignOnlyProperty` hint was 23 of 46 results on
health-suite and mostly persisted-model fields, so only `unused` is read.

## This host's edge blocks Python urllib's default User-Agent

Any CI step that calls `git.roost-r.com`'s API from Python
(`urllib.request`) needs an explicit `User-Agent` header. Verified
directly: that host's edge returns 403 for Python urllib's default UA
specifically — curl's default UA and an arbitrary custom UA both get
200, so it's blocking the known Python signature, not bots generally. A
bare `urlopen(url)` 403s silently-ish (as a Python traceback, easy to
misread as a real API error) rather than reaching the API.

## TypeScript 7 status (checked 2026-09-03)

TS 7.0 GA'd July 8, 2026 as the Go-native compiler rewrite
(`typescript-go`/"Project Corsa", ~8-12x faster builds). Its npm
package's main entry point **drops the classic `ts.createSourceFile`
compiler API entirely** — only explicitly-unstable `typescript/unstable/*`
subpaths are exposed. A stable programmatic API is promised for 7.1, not
yet released. `typescript-eslint`, `ts-morph`, `ts-jest`, Angular, Vue,
and Svelte are all still stuck on 5.x/6.x for this exact reason.

Both `internal/analyzers/typescript/complexity.js` (crap-metric) and
`internal/tokenizers/typescript/scanner.js` (dupe-metric) handle this by
falling back to `@typescript/typescript6` — Microsoft's own official
compat package, re-exporting the full classic API — when the target
repo's resolved `typescript` has no `createSourceFile` (i.e. v7+). Only
works if that fallback is actually installed as a devDependency in the
target repo; otherwise the script errors out clearly. Covered by
`TestAnalyzeFallsBackToTypescript6ForTS7` /
`TestTokenizeFallsBackToTypescript6ForTS7` against
`testdata/{crap,dupe}/typescript-ts7` (real TS7 + `@typescript/typescript6`
installs — run `npm install` there if those tests are skipping).
`cycle-metric`'s TS importer is unaffected — pure regex, no `typescript`
package dependency at all.

No repo in `~/dev` is on TS7 today except `d_amp_d` (confirmed on
`~7.0.0`, hence `@typescript/typescript6` already in its
`app/package.json`) — this isn't hypothetical anymore for that repo
specifically. Revisit once 7.1's stable API ships: at that point
`typescript-go`'s compiler internals becoming an embeddable Go module
(not just the `tsgo` CLI) would let both analyzers drop the Node
subprocess entirely and parse TS natively, matching the Go
adapters' own approach — worth checking for when 7.1 lands, not assumed
to exist yet.

## `--lang js` was accepted and documented for months before it worked

All four tools' `--lang` switch statements have mapped `js`/`javascript`
to the same adapter as `ts`/`typescript` since early on, and README said
so ("All four support ... `ts`/`typescript`/`js`"). For escape-metric and
cycle-metric that was true: `patterns.go`'s `ts` block already listed
`.js`/`.jsx` in `Extensions`, and the TS importer's `codeExtensions`
already included them too, both being generic over the extension list
with no TS-specific logic. For crap-metric and dupe-metric it wasn't:
`complexity.js`'s and `scanner.js`'s own `walk()` functions matched only
`/\.(ts|tsx)$/`, so `--lang js` against a pure-JS directory silently
walked zero files and reported empty, every time — no error, just a
gate that could never fail.

The reason it went unnoticed: `cmd/*/main_test.go`'s dispatch tests
(`"js": "typescript.Analyzer"` etc.) only assert the flag resolves to
the right Go adapter *type* — they can't see into the embedded Node
script at all, so they stayed green through the whole time `--lang js`
was broken. Fixed by extending both scripts' `walk()` regex to
`.ts|.tsx|.js|.jsx` (plus matching `.test./.spec.` exclusions) and
replacing complexity.js's `file.endsWith('x') ? TSX : TS` heuristic with
an explicit per-extension `ScriptKind` map — TSX happened to parse plain
`.jsx` well enough that the heuristic wasn't caught by types, only by
adding real fixtures. `testdata/{crap,dupe}/javascript`,
`testdata/cycle/javascript`, and JS fixtures folded into
`testdata/escape/typescript` now exercise every adapter's actual JS path
end to end, not just its dispatch switch. Escape-metric's `TestSuffixes`
had the same class of gap in miniature — `.test.ts`/`.spec.ts` were
excluded but `.test.js`/`.spec.js` weren't, so a JS test file's
suppression comments counted as real code unlike an equivalent TS one;
fixed alongside the rest.

The lesson for a future language or extension added to an existing
adapter: a dispatch-switch test proves the CLI *routes* to the right
adapter, never that the adapter's own file-matching was updated to
actually handle it. Add a real fixture and an end-to-end test for the
new extension specifically, the same way `typescript-ts7`'s fixture
exists for the TS7 fallback path above it — don't trust the alias
existing in three places (the switch, the alias map, the README table)
to mean the fourth (the actual walk) was updated too.

## CI self-checks this repo — keep it passing for real

`.forgejo/workflows/ci.yml`'s self-check jobs run crap-metric/
dupe-metric/escape-metric/arch-metric against this repo's own Go source
and gate the build on it (cycle-metric can't — see README). If a change pushes a gate
into FAIL, the right fix is almost always a real test exercising the
flagged code — not raising `--fail-above` to make the finding go away.
`internal/crap/report_test.go` and `cmd/crap-metric/main_test.go` exist
specifically because crap-metric's self-check caught `WriteTable` and
its own CLI entrypoint at 0% coverage the first time it ran; dupe-metric's
self-check caught `internal/tokenizers/{python,typescript}` at ~90%
duplicated against each other, leading directly to the
`internal/tokenizers/tokenizer.go` extraction. The tool finding gaps in
itself, closed with real tests or real refactors rather than a raised
threshold, is the loop working as designed.

## Releases are for distribution only — report history lives on a `reports` branch

Early on, one Forgejo release per push carried both the distribution
binaries `install-quality-gates` downloads *and* `crap-report.json`, kept
around specifically so the next run's trend-diff had something to read
(`releases?limit=2`, newest-first). That conflation had a real cost: it
meant releases could never be cleaned up (deleting one could delete the
only copy of a report a later run still needed), so every push — including
doc-only ones — grew the release list forever, and no CLI could report
its own version, so a consumer floating on `install-quality-gates`'s
`latest` had no way to tell what it was actually running or pin away from
a bad push.

Fixed by separating the two concerns: `crap-report.json` now lives on a
dedicated `reports` branch (one commit per self-check run, that file
only), read via plain `git show origin/reports:crap-report.json` instead
of the Releases API — no more Python `urllib` + User-Agent workaround for
this particular lookup. With no history depending on them, releases are
now pure distribution artifacts and the release job prunes down to the
newest 20 after every push. Each binary also gets a real `version`
subcommand now, baked in via `-ldflags "-X main.version=<short-sha>"` at
build time — same convention as `gamectl`'s `-X main.version=`, not
invented fresh — and `install-quality-gates` grew an optional `version`
input so a consumer can pin to a known-good release instead of always
floating on `latest`. No semver: an internal tool with a handful of
consumer repos doesn't need major/minor/patch meaning yet, and the short
SHA plus a real `version` output already answers "what am I running" and
"can I pin a known-good build" — don't add semver machinery here unless an
actual consumer need shows up for it.

## crap-metric: size/shape gate is extra fields on `crap.Function`, not a fifth tool

CRAP's cyclomatic count and dupe-metric's duplication check both miss a
readability failure mode neither is shaped to catch: a flat 40-case
switch scores *high* on cyclomatic complexity but reads fine, while a
6-deep nested `if` scores *low* but is unreadable. Every language
analyzer already parses each function once for complexity — function
length, parameter count, nesting depth, and file length fall out of that
same parse almost for free, so they're extra fields on `crap.Function`
(`ParamCount`, `MaxNestingDepth`, `FileLines`) and a `SizeThresholds`
gate layered on `crap.Report` via `WithSize` (mirroring
`internal/mutation`'s `WithMinMutants`: kept apart from `NewReport` so
the CRAP score math stays a single pass), rather than a fifth CLI.
Function length itself isn't a stored field at all —
`Function.LineCount()` derives it from `EndLine - StartLine + 1`, which
every analyzer already sets.

**Nesting depth's else-if/elif flattening is the one subtle design
choice, and it's the same reasoning size-metric exists for in the first
place**: a chained `else if`/`elif` reads like a switch's cases, so it
must NOT nest deeper than its `if` — only a genuine nested block should
add a level. Go/TypeScript/Swift all get this from their real AST/token
structure directly (an "else if" is the next `if` sitting where the
`else` branch would be, so recursing at the *same* depth rather than
`depth+1` flattens it correctly). Python's `ast` module can't make this
distinction at all: `elif b:` and `else:\n    if b:` parse to the
*identical* tree (`orelse=[If(...)]`) — there's no way to tell a real
elif from a deliberately nested `else: if` short of comparing source
column offsets. Documented as a known limitation in
`internal/analyzers/python/size_facts.py` rather than solved — same
posture as escape-metric's regex-only v1 scope and swiftlex's documented
lexical gaps elsewhere in this file, and harmless in practice since
nobody writes `else: if` when `elif` says the same thing.

**A real bug caught during dogfooding, not a hypothetical**: the first
version of Swift's nesting-depth walker (`internal/analyzers/swift/size.go`)
classified control-flow keywords (`if`/`guard`/`while`/`for`/`switch`/
`catch`) by scanning every keyword token in a function's body span,
without regard to whether that keyword lexically belonged to the
function itself or to a **nested local func** inside it. Since
`internal/analyzers/swift/functions.go`'s own complexity walk
deliberately *does* let a nested local func's branches fold into the
enclosing function's complexity count (see the Swift support entry
above), it was easy to assume nesting depth should do the same — but a
nested func is a separate lexical scope, and letting its internal `if`s
count toward the *enclosing* function's reported nesting depth is a
correctness bug, not a design choice: `TestAnalyzeStampsSizeFields`'s
`withNestedHelper` case (a nested func with its own doubly-nested `if`)
caught it immediately by reporting nesting 2 for a function whose own
body is a single flat `return`. Fixed by having
`classifyControlBraces` jump straight past a nested func's whole body
(via the same `swiftlex.FindFuncBodyOpen`/`MatchBrace` pair
`extractFunctions` already uses to skip it) rather than walking into it
— the same "opaque nested closure" choice the Go analyzer makes
structurally for free (its walker only follows `ast.IfStmt`/`ForStmt`/
etc.'s own statement lists, never descending into a `FuncLit`), and the
safe under-counting direction for a generous, egregious-cases-only gate
either way.

**File length is stamped onto every function in that file** (same
`FileLines` value repeated), not tracked as a separate per-file report —
keeps it as "extra fields on `crap.Function`" per the design above, and
`sizeFindings` dedupes it back down to one finding per file. Known
limitation, same shape as the ratchet's suffix-match caveat above: a
file with zero functions (an interface/type-only file, a config-like
file with only top-level declarations) is invisible to the file-length
gate, since there's no `crap.Function` to stamp it onto. Not worth a
separate per-file walk in every one of four analyzers for what should be
a rare case in practice.

**Defaults were picked by running the gate against this repo's own real
source, not guessed** — same discipline as dupe-metric's 18% self-check
threshold and dead-metric's vulture/knip confidence tuning above.
`--max-lines 80`/`--max-params 6` come straight from the sizes named
when this gate was speced. `--max-nesting` and `--max-file-lines`
didn't have a speced number, so they were set from this repo's own
observed maximums at the time (nesting 4, `internal/swiftlex/lexer.go`
at 484 lines) plus headroom: nesting settled on 5 (4 had *zero*
headroom — any new function reaching depth 5 anywhere would've broken
self-check immediately) and file length on 600. `--max-lines 80` itself
is tighter than it looks: `cmd/crap-metric/main.go`'s own `runCheck` hit
84 lines while this gate was being wired up (adding four new flags to an
already-76-line function) and had to be refactored — the exclude-file-
loading sequence pulled out into `applyExcludes` in `exclude.go` — to
fit back under its own new gate before this could ship. The tool finding
a gap in itself, closed with a real refactor rather than a raised
threshold, is the same loop already described in the CI self-checks
entry above.
