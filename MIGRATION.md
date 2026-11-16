# Standalone cycle and arch release canary

The extracted cycle and architecture gates now have independent binary
releases. `ci-workflows` supports `cycle-version` and `arch-version`; the
original suite release remains available as the fallback while consumer
workflows are migrated gradually.

## Published releases

| Gate | Release | Tag target | Tag CI | Published assets |
|---|---|---|---|---|
| `cycle-metric` | [v0.1.0](https://git.roost-r.com/quality-gates/cycle/releases/tag/v0.1.0) | [`bd7677f`](https://git.roost-r.com/quality-gates/cycle/commit/bd7677f1f2dd11f53ba924c998f99626ca3e437e) | [run #5](https://git.roost-r.com/quality-gates/cycle/actions/runs/5) | `cycle-metric-linux-amd64`, `cycle-metric-darwin-arm64`, `SHA256SUMS` |
| `arch-metric` | [v0.1.0](https://git.roost-r.com/quality-gates/arch/releases/tag/v0.1.0) | [`58f0efa`](https://git.roost-r.com/quality-gates/arch/commit/58f0efa0677e252a07d1aa619802a2154ebc3bbc) | [run #5](https://git.roost-r.com/quality-gates/arch/actions/runs/5) | `arch-metric-linux-amd64`, `arch-metric-darwin-arm64`, `SHA256SUMS` |

Both tags are `v0.1.0`. The release workflows passed their build, self-check,
and publish jobs. The two platform binaries and each `SHA256SUMS` manifest
were downloaded and verified. On macOS arm64, the binaries report
`cycle-metric bd7677f` and `arch-metric 58f0efa` respectively.

The known-good suite fallback is release [`c1f3728`](https://git.roost-r.com/cadeh/quality-gates/releases/tag/c1f3728).
Its cycle and arch macOS arm64 binaries also passed manifest verification and
both report version `c1f3728`.

## Installer canary

Canary PR [#35](https://git.roost-r.com/cadeh/quality-gates/pulls/35) adds a
four-case matrix in `.forgejo/workflows/ci.yml`. Each case starts in a fresh
job and installs suite release `c1f3728`, with both standalone pins, only the
cycle pin, only the arch pin, or no standalone pins. The job checks the
installed source versions and asserts the expected count, exit status, and
finding details for cycle checks on the Python, TypeScript, and JavaScript
fixtures; architecture checks on the Python, TypeScript, and Go fixtures; and
a Go stability check on its dedicated fixture.

The initial four-case matrix passed in [run #118](https://git.roost-r.com/cadeh/quality-gates/actions/runs/118).
After strengthening the assertions to require every expected finding, all four
cases passed in [run #122](https://git.roost-r.com/cadeh/quality-gates/actions/runs/122)
on canary implementation head `0893323dcff553b1633ef42a46b8990bc9f93811`.
After the documentation refresh, the final-head verification reran all four
cases successfully in [run #123](https://git.roost-r.com/cadeh/quality-gates/actions/runs/123)
on head `9701a39b3b971017c7b788e77798d126008989e8`.
This canary does not switch other consumer workflows away from the suite
release; migrate those consumers separately.
