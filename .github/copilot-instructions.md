# GitHub Copilot instructions for gh-aw-fleet

These instructions give GitHub Copilot (chat, code review, and the coding
agent) the context it needs to make accurate suggestions and reviews. For
deeper architecture and operator workflows, see [`AGENTS.md`](../AGENTS.md).

## What this project is

`gh-aw-fleet` is a thin orchestrator around three upstream tools — `gh aw`
(the markdown→GitHub Actions compiler), `gh` (PR creation), and `git`
(branching/commits). It never rewrites workflow markdown; its value is in
*which repo gets which profile, pinned to which ref*. Prefer shelling out to
the upstream tools over re-implementing their logic.

## Toolchain — read before flagging a compile error

This module targets **Go 1.26.4** (`go.mod` directive and the CI gate). The
full modern standard library is available and correct. Do **not** report newer
stdlib APIs as nonexistent or as "won't compile." In particular, these are
valid:

- `sync.WaitGroup.Go(func())` — runs `f` in a new goroutine, auto-managing
  `Add(1)`/`Done()` (added in Go 1.25). Used in `internal/fleet/overview.go`.
- Builtins `min`, `max`, `clear`, and `for i := range n` over an integer.
- The `slices`, `maps`, and `cmp` packages.

If an API looks unfamiliar, assume the toolchain is newer than your training
data and verify it against the Go 1.26 standard library before raising a
compile concern. Green CI on a PR is authoritative: a "won't compile" finding
that contradicts a passing build is a false positive.

## Conventions to respect in reviews

- **Commits and PR titles** use Conventional Commits with the `ci(workflows)`
  scope (these changes are CI configuration). Subject ≤72 chars, no trailing
  period.
- **Never hand-edit `CHANGELOG.md`.** release-please generates it from
  conventional-commit subjects on every push to `main`; the commit message
  *is* the changelog entry. Do not suggest manual CHANGELOG edits.
- **No new third-party dependencies** without explicit constitutional approval
  (`.specify/memory/constitution.md`). Prefer the standard library and the
  already-approved direct dependencies.
- **Never bypass gpg signing**, and never add `git add` / `git commit` /
  `git push` to tooling outside the existing `exec.Command` calls in
  `Deploy` / `Sync` / `Upgrade`.
- **Dry-run by default.** Mutating commands require `--apply`; treat that gate
  as intentional, not a missing safeguard.
- **Two `SchemaVersion` constants** exist and bump independently:
  `cmd.SchemaVersion` (the JSON output envelope) and `fleet.SchemaVersion`
  (the on-disk `fleet.json` format). Do not suggest unifying them.

## Code style

- Every exported identifier (package, type, func, method, const, var, struct
  field) needs a godoc comment: one sentence starting with the identifier
  name and ending with a period.
- Prefer expressive names over comments for unexported code; comments should
  explain *why* (constraints, invariants), not restate *what* the code does.
- `make ci` (`fmt-check vet lint test`) is the gate. Suggestions should keep
  it green.
