# quality-gates

Four CI quality gate CLIs, one Go module: **crap-metric**, **dupe-metric**,
**escape-metric**, and **cycle-metric**. Each answers a different
question about a change, with a deliberately different, appropriately-
scoped detection method — but they share enough (the `--only-files`
ratchet mechanism, the CI/release plumbing, the install pattern) that
running them as four separate repos meant four copies of that shared
logic drifting independently. Migrated into one module for exactly that
reason — see [CLAUDE.md](CLAUDE.md) for the incidents that motivated it.

## The four gates

| Binary | Question | Method | Default gate |
|---|---|---|---|
| `crap-metric` | Is this function complex *and* undertested? | Parses each language for cyclomatic complexity + coverage, combines via `complexity² × (1−coverage)³ + complexity` | `--fail-above 30` |
| `dupe-metric` | Is this code duplicated? | Tokenizes source, finds exact-match blocks via greedy leftmost-longest shingling | `--fail-above 5` (%) |
| `escape-metric` | Did this code opt out of type-checking/linting/error handling? | Regex-matches suppression comments and a few high-confidence whole-line patterns | `--fail-above 1` (per 1000 lines) |
| `cycle-metric` | Are these modules structurally tangled? | Regex-extracts imports, resolves to files, runs Tarjan's SCC | `--fail-above 0` (cycles) |

All four support `python` and `ts`/`typescript`/`js`; `crap-metric` and
`dupe-metric` also support `go`. `cycle-metric` deliberately excludes Go
— the compiler already refuses to build a package-import cycle, so a
detector for it would always report zero. See each package's doc comment
(`internal/crap`, `internal/dupe`, `internal/escape`, `internal/cycle`)
for the full reasoning and known characteristics of its method.

## Ratcheting: `--only-files`

All four `check` commands accept `--only-files PATH`, pointing at a
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
`cycle.Cycle` counts as touched if *any* file in it matches.

In CI, the changed-file list itself comes from `ci-workflows`'
`compute-changed-files` action (a `git diff` against the PR base or the
previous push's `before` SHA) — see that repo's README for what it
needs from the caller (git installed before checkout, `fetch-depth: 0`).

## Usage

```
crap-metric   check --lang <python|go|ts> --dir <dir> [--coverage PATH] [--fail-above N] [--verbose] [--only-files PATH] [--json PATH]
crap-metric   diff  --old PATH --new PATH [--top N] [--json]
dupe-metric   check --lang <python|go|ts> --dir <dir> [--min-tokens N] [--fail-above PCT] [--only-files PATH] [--json PATH]
escape-metric check --lang <python|ts> --dir <dir> [--fail-above RATE] [--only-files PATH] [--json PATH]
cycle-metric  check --lang <python|ts> --dir <dir> [--fail-above N] [--only-files PATH] [--json PATH]
```

Every `check` also takes `--top N` (rows to print, default 20, `0` = all).

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
  `typescript` package (classic compiler API). TypeScript 7 drops that
  API from its main entry point — the script falls back to Microsoft's
  official `@typescript/typescript6` compat package if present. See
  CLAUDE.md for the full TS7 story.

### crap-metric: trend diffing

`crap-metric diff --old <report.json> --new <report.json>` compares two
JSON reports (matched by file+name) and prints every function that's
new, removed, or changed score, worst-regression-first. Informational
only — it always exits `0`; the gate stays `check`'s job.

### dupe-metric / escape-metric: how each language is analyzed

No external tool needed for either language in **escape-metric** (pure
Go regex over raw text) or for **Go** in dupe-metric (native
`go/scanner`). **Python** tokenizing uses an embedded script against the
standard library's `tokenize` module (no pip package needed).
**TypeScript/JS** tokenizing uses an embedded Node script against the
target repo's own `typescript` package's scanner — same TS7 fallback as
crap-metric.

Both tools skip comments and test files (`*_test.go`, `test_*.py`/
`*_test.py`, `*.test.ts`/`*.spec.ts`) — duplicate/suppressed test
boilerplate is common and isn't the kind of finding these gates are for.

### cycle-metric: import resolution

Pure regex extraction, no parser: Python's `import`/`from` (absolute and
relative) and TS/JS's `import`/`export`/`require` specifiers (relative
only — bare/aliased imports are treated as external). Both languages are
scanned **with test files included**, unlike the other three tools — a
cycle is a structural property of the whole import graph, and excluding
tests could hide a real tangle. Every cycle's report includes both the
full set of files involved and a reconstructed concrete `Chain` — a
literal path back to its own start, not just an unordered set.

## Architecture

```
cmd/
  crap-metric/    dupe-metric/    escape-metric/    cycle-metric/
internal/
  ratchet/                        # shared --only-files primitives
  crap/           analyzers/{golang,python,typescript}
  dupe/           tokenizers/{golang,python,typescript}
  escape/
  cycle/          importers/{python,typescript}
testdata/
  crap/{golang,python,typescript,typescript-ts7}/
  dupe/{golang,python,typescript,typescript-ts7}/
  escape/{golang,python,typescript}/
  cycle/{python,typescript}/
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
every push and PR; on `main`, it also builds all four `linux/amd64`
binaries and publishes them to **one** Forgejo release tagged with the
short commit SHA, then self-checks crap-metric/dupe-metric/escape-metric
against the repo's own (now much larger, all-four-tools) Go source —
cycle-metric still can't self-check, being Go-only in implementation but
not supporting Go analysis.

Consumed by `d_amp_d` via `ci-workflows`' `install-quality-gates` action,
which replaced that repo's four separate `install-*-metric` actions.

## Development

```
go build ./...
go vet ./...
go test ./...
```

The analyzer/tokenizer tests exercise the real underlying tools, not
mocks — `radon` and `coverage` (Python, for crap-metric) and `node` +
`typescript` (in each `testdata/{crap,dupe}/typescript{,-ts7}/node_modules`,
`npm install` there if missing) need to be available locally to run the
full suite. `python3` alone (stdlib `tokenize`, no pip package) covers
dupe-metric's and cycle-metric's Python fixtures.
