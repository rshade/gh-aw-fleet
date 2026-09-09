# Design: mise toolchain migration, Go 1.27.1, ax-go v0.6.0

- **Date**: 2026-09-08
- **Branch**: `chore/mise-toolchain` (from `origin/main`)
- **Status**: approved, pending implementation plan

## Problem

Three requests that look independent are one coupled chain.

`ax-go` raised its `go.mod` directive across the versions this repo is
being asked to adopt:

| ax-go tag | `go` directive |
| --------- | -------------- |
| v0.4.0    | 1.26.5         |
| v0.5.0    | 1.26.7         |
| v0.6.0    | 1.27.1         |

`origin/main` is on ax-go v0.4.0 and declares `go 1.26.5`. Go refuses to
build a module whose
dependency declares a higher directive, so **ax-go v0.6.0 cannot be adopted
until this module moves to Go 1.27.1**. The local toolchain is 1.26.7, so the
upgrade is not buildable today.

That makes the toolchain pin the enabler rather than a parallel chore, and it
promotes Renovate PR #222 (`go` to 1.27.x) from optional to prerequisite.

Separately, tool versions currently live in four places with no link between
them: `.github/workflows/ci.yml` (`go-version`, `golangci-lint-action.version`),
`.github/workflows/release.yml` (`go-version`), `.github/workflows/docs.yml`
(`node-version`), and `go.mod`. Nothing detects drift between them.

## Decisions

Four decisions were settled during brainstorming. They constrain everything
below.

1. **mise pins tools only; the Makefile stays the task runner.** `mise.toml`
   carries `[tools]` and no `[tasks]`. All ten existing Makefile targets keep
   their names, and one is added: `ensure`, which installs the pinned
   toolchain. (A second addition, `drift`, came later out of review — see the
   Makefile section.) `make ci` remains the gate, so `AGENTS.md`, `CLAUDE.md`,
   and the six `skills/*/SKILL.md` files need no rewrite. This mirrors the
   sibling `ax-go` repo, which shares the Go toolchain.
2. **The in-flight `020-overview-model-health` work is committed first**, on
   its own branch, before the toolchain branch is cut from `origin/main`. The
   two must not share a branch: the Go 1.27.1 bump would break `make ci`
   against uncommitted feature code.
3. **All nine open bot PRs are absorbed into this one branch.** This is the
   literal reading of the request and was chosen with its tradeoff stated: the
   diff mixes npm churn with the toolchain migration, and #221/#222's `ci.yml`
   edits are written and then deleted within the same branch. Commit ordering
   (below) keeps that legible.
4. **The Dependabot/Renovate overlap is out of scope.** Both bots stay as
   configured. A follow-up issue records the collision.

## Approach

`mise.toml` becomes the single source of truth for tool versions. The Makefile
reads versions back out of it rather than duplicating them. CI installs from it
via `jdx/mise-action@v4`, so a local `make ci` and a CI run execute identical
binaries. A drift guard asserts `go.mod` and `mise.toml` agree on Go.

### mise.toml

```toml
[tools]
go = "1.27.1"
golangci-lint = "2.13.2"
node = "24"
```

All three verified available via `mise ls-remote`.

Deliberately narrower than `ax-go`'s, which also pins `actionlint`,
`govulncheck`, and `pipx:specify-cli`. `make ci` in this repo invokes none of
those, and pinning a tool no target runs would invent a contract the Makefile
does not have. `node = "24"` matches `docs.yml`'s existing `node-version: 24`.

### Makefile

Targets `build test vet fmt fmt-check lint ci tidy clean help` are unchanged.
Two additions, both ported from `ax-go`:

```make
GOLANGCI_LINT_VERSION := $(shell awk -F'"' '/^golangci-lint =/{print $$2}' mise.toml)

.PHONY: ensure
ensure:
	@command -v mise >/dev/null 2>&1 || \
		(echo "mise not found: https://mise.jdx.dev/installing-mise.html"; exit 1)
	mise install
```

Reading the version back out of `mise.toml` avoids duplicating it, but does
not by itself guarantee the value matches what's on `PATH` — that depends on
mise's shims winning `PATH`, which cannot be assumed (see below).

This is expected to resolve a standing local workaround: `make lint` previously
resolved a different `golangci-lint` binary than the intended one, because
`PATH` order picked one up ahead of the other. mise's shims make the pinned
2.13.2 unambiguous. **This must be re-tested rather than assumed** — see
Verification.

Two further Makefile changes came out of the branch's own review rather than
this design. `ci` now depends on a `drift` target that runs
`scripts/check-toolchain-drift.sh`, and `.github/workflows/ci.yml` invokes
`make drift` rather than the script directly — so the gate has one
definition instead of two that can diverge. And `lint` now compares the
`golangci-lint` actually on PATH against `GOLANGCI_LINT_VERSION` before
running, failing with `make ensure` when the binary is absent and with
`mise exec -- make lint` when it is merely the wrong version. Reading the
pin without asserting it was not enough, because mise's shims do not
reliably win PATH.

### CI workflows

`ci.yml` drops both `actions/setup-go` and `golangci/golangci-lint-action`:

```yaml
- uses: actions/checkout@v7
- uses: jdx/mise-action@v4
- run: go mod download
- run: make vet
- run: make fmt-check
- run: make test
- run: make lint
```

Dropping `golangci-lint-action` is the point: CI stops holding its own opinion
about the linter version, and a local `make ci` becomes a true predictor of CI.

A drift guard is added — the check that would have caught the ax-go v0.6.0
floor bump before it surfaced as an opaque build error:

```yaml
- name: go.mod matches mise.toml
  run: |
    go_mod=$(awk '/^go /{print $2; exit}' go.mod)
    mise_go=$(awk -F'"' '/^go =/{print $2; exit}' mise.toml)
    [ "$go_mod" = "$mise_go" ] || {
      echo "go.mod says $go_mod, mise.toml says $mise_go"; exit 1; }
```

`release.yml` swaps `actions/setup-go` for `jdx/mise-action@v4` so releases
build on the pinned toolchain.

`docs.yml` swaps `actions/setup-node` for `jdx/mise-action@v4` and adds an
explicit `actions/cache` for `~/.npm` keyed on `docs/package-lock.json`. This
forfeits `setup-node`'s built-in caching in exchange for `mise.toml` owning the
node version outright; leaving `node-version: 24` duplicated in a second file
is the exact drift this migration exists to remove.

### Dependency upgrades

`go.mod` moves from `go 1.26.5` to `go 1.27.1`, matching `mise.toml`.
`github.com/rshade/ax-go` moves v0.4.0 to v0.6.0.

The ax-go upgrade is **source-compatible, and narrower than it looks**. Across
the three packages this repo imports — `config`, `schema`, `contract` — the
entire v0.4.0-to-v0.6.0 delta is two new exported functions in
`contract/context.go`:

```go
func WithApproval(ctx context.Context, granted bool) context.Context
func ApprovalFromContext(ctx context.Context) bool
```

`schema/schema.go` and the `config` package are byte-identical between the two
tags. Nothing this repo calls changes signature, so **no `__schema` wire change
and no version bump are owed by this upgrade.**

The one genuine breaking change in the range, `feat(guard)!: default-on
structured audit logging for Guard/Perform`, lands in `guard` — a package
outside this repo's import boundary and therefore not in the build.

### npm and docs

Five bot PRs land as one commit against `docs/package.json` and
`docs/package-lock.json`: astro monorepo (#218), sharp 0.35.4 (#224),
`@astrojs/starlight` ^0.42.0 (#226), `starlight-links-validator` ^0.26.0
(#227), and js-yaml 4.3.2 (#220).

## Commit sequence

Eight commits in dependency order. Each is independently green except where the
note says otherwise.

| # | Commit | Rationale |
| - | ------ | --------- |
| 1 | `fix(schema): restore MCP positional args after ax-go tool rename` | `cmd/schema.go`, `cmd/schema_test.go`. Fixes a pre-existing bug found during verification (CI has been red on `main` since #213); ax-go v0.4.0 moved MCP tool-name construction to a `-`-joined form no switch case matched. Independent of the toolchain migration; landed first because it blocks nothing else. |
| 2 | `build: pin the toolchain in mise.toml` | `mise.toml` + Makefile readback. Nothing else can move until Go 1.27.1 is installable. Absorbs #221's golangci-lint 2.13.2 as a mise pin. |
| 3 | `refactor(json): use pointer receivers on result MarshalJSON` | `internal/fleet/result_json.go`, `internal/fleet/result_json_test.go`. golangci-lint 2.13.2, pinned in commit 2, adds `recvcheck`, which flags `DeployResult`/`UpgradeResult` for mixing receivers; no behavior change, since every call site already passes a pointer. |
| 4 | `ci: install the pinned toolchain via mise-action` | `ci.yml`, `release.yml`, `docs.yml`, plus the drift guard. |
| 5 | `build: raise the go directive to 1.27.1` | `go.mod`, `go.sum`. Satisfies the drift guard added in commit 4. Supersedes #222. |
| 6 | `fix(deps): update ax-go to v0.6.0` | `go.mod`, `go.sum`. Only possible after commit 5. Supersedes #225 and #228. |
| 7 | `chore(deps): update docs npm dependencies` | `docs/package.json`, `docs/package-lock.json`. Supersedes #218, #220, #224, #226, #227. |
| 8 | `docs: record the mise toolchain contract` | `AGENTS.md`, `CLAUDE.md`, `README.md`, `.github/copilot-instructions.md`, `docs/src/content/docs/install.md`, `specs/021-mise-toolchain-migration/`. |

Conventional Commits are required; `commitlint` gates PR titles via
`.github/workflows/commitlint.yml`. Per `AGENTS.md`, `CHANGELOG.md` is never
hand-edited — release-please generates it from these subjects.

## Bot PR disposition

| PR | Bot | Disposition |
| -- | --- | ----------- |
| #221 golangci-lint v2.13.2 | renovate | Superseded by commit 2 — version moves to `mise.toml`. |
| #222 go 1.27.x | renovate | Superseded by commits 2 and 5. |
| #225 ax-go v0.6.0 | renovate | Absorbed by commit 6. |
| #228 ax-go 0.5.0 | dependabot | Superseded — duplicate of #225 at an older version. |
| #218, #224, #226, #227 | renovate | Absorbed by commit 7. |
| #220 js-yaml 4.3.2 (security) | dependabot | Absorbed by commit 7. |

After merge, Renovate re-detects `go` and `golangci-lint` against `mise.toml`
via the `mise` manager, which `config:recommended` already enables. Renovate
proposes `mise.toml` and `go.mod` under separate managers with no grouping
between them, so a Go bump arrives as a red PR until `go mod edit` is run in
the same branch. A `packageRule` grouping `matchManagers: ["mise", "gomod"]` on
`matchDepNames: ["go", "golang"]` would make those PRs green on arrival; it is
filed as a follow-up. The existing `packageRules` protecting `gh-aw-actions`
and `*.lock.yml` are untouched.

## Verification

`make ci` is the gate, run after commit 6 and again at the end. Beyond that:

- `go build ./...` after commit 6, confirming ax-go v0.6.0 compiles.
- `make test`, which covers `cmd/schema_test.go` — the only ax-go-facing test
  surface.
- `go run . __schema | jq` and `go run . __schema --as mcp | jq`, compared
  byte-for-byte against the same commands run before the upgrade. The expected
  result is **no diff at all**, since `schema/schema.go` is unchanged between
  v0.4.0 and v0.6.0.
- `go run . list`, the fastest end-to-end `LoadConfig` check, since
  `internal/fleet/load.go` consumes `ax-go/config`.
- **`cd docs && npm ci && npm run build`, run locally.** `docs.yml` triggers
  only on pushes to `main`, so CI will not validate commit 7 on this branch.
  Five npm bumps landing unvalidated is the principal risk of the
  single-branch choice.
- Confirm `make lint` and `mise exec -- golangci-lint --version` agree on
  2.13.2, testing whether the local `PATH`-shadowing workaround is now
  unnecessary.

## Risks

| Risk | Mitigation |
| ---- | ---------- |
| Astro/starlight bumps break the docs build, unseen by branch CI | Local `npm run build` is a required verification step, not optional. |
| Go 1.27.1 surfaces new vet or lint findings across the codebase | `make ci` after commit 5, before the ax-go bump, isolates toolchain findings from dependency findings. |
| `jdx/mise-action@v4` behaves differently from `setup-go`'s module caching | mise-action caches tool installs; `go mod download` remains an explicit step. Cache misses cost time, not correctness. |
| Reviewer cannot separate toolchain from dependency churn | The eight-commit sequence is the mitigation; review commit-by-commit. |

## Out of scope

Follow-up issues to file, not to implement here:

1. **Dependabot/Renovate consolidation.** Both bots cover `gomod` and npm,
   producing duplicate PRs (#225 vs #228, #220 vs #218). Deferred by decision 4.
2. **Adopt `schema.WithNonDeterministicFields`** for the `overview` and
   `consumption` envelopes, which carry timestamps and other per-run values.
   Available since ax-go v0.4.0 — already on `main`, already unused.
   Optionally also `contract.WithApproval` / `ApprovalFromContext`, new in
   v0.6.0, for the `--apply` confirmation gate.
3. **`copilot-setup-steps.yml` version gap.** It pins
   `github/gh-aw-actions/setup-cli` at v0.79.8 while requesting
   `version: v0.68.3`, against a fleet pinned to v0.81.6. Unrelated to this
   work but worth a look.
4. **`actionlint` in `mise.toml`.** Excluded because no Makefile target runs
   it. Reconsider if a workflow-lint target is ever added.
