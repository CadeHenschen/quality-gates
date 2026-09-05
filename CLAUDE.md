# quality-gates

Four CI quality gate CLIs (Go), one module, for repos on
`git.roost-r.com/cadeh`: `crap-metric` (complexity × undertested),
`dupe-metric` (duplication), `escape-metric` (suppressed checks),
`cycle-metric` (import cycles). See [README.md](README.md) for the
formula/algorithm, CLI usage, and architecture.

This repo is a 2026-09-05 merge of four previously-standalone repos of
the same names. They stayed separate as long as they had genuinely
separate lessons to learn; migrated once the thing they had in common
(the `--only-files` ratchet, the CI/release pipeline, the install
action) was drifting across four copies instead of living in one place.
The lessons below are inherited from all four histories — a lesson
learned once in `escape-metric` didn't need re-learning in
`cycle-metric` because `cycle-metric` was built after, reading this file
first. Keep doing that: read this before adding a fifth tool or a new
language adapter to an existing one.

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

## CI self-checks this repo — keep it passing for real

`.forgejo/workflows/ci.yml`'s self-check jobs run crap-metric/
dupe-metric/escape-metric against this repo's own Go source and gate the
build on it (cycle-metric can't — see README). If a change pushes a gate
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
