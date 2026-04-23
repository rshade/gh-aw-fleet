# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Common commands

```bash
go build ./...        # Compile. Run before claiming a change builds.
go vet ./...          # Must pass before any edit is considered done.
go run . <cmd>        # Run the CLI directly (e.g. `go run . list`).
go run . list         # Fastest sanity check — exercises LoadConfig end-to-end.
go run . template fetch   # ~3 min (serial); do not run in subagents with strict timeouts.
```

For third-party API shape or documentation questions (e.g. zerolog field-writer signatures, cobra persistent-flag ordering), prefer `go doc <pkg>` / `go doc <pkg>.<Symbol>` over web search — it returns the exact API surface for the pinned version in `go.mod`. When editing, `gopls` (running via the IDE or `gopls definition`/`gopls symbols`) resolves identifiers against the current module state, which catches rename drift faster than grep.

### Local gate — run before claiming any code change complete

`go build` / `go vet` are necessary but **not sufficient**. CI runs stricter checks (gofmt alignment, golangci-lint, full test suite) that build+vet do not. Before reporting any code change "done," run the full gate:

```bash
make fmt         # apply gofmt in place (or `make fmt-check` to verify without writing)
make lint        # golangci-lint — can exceed 5 minutes; use extended timeout, do NOT skip
make test        # full test suite
# — or, in one shot: —
make ci          # runs: fmt-check vet lint test  (this is the CI gate)
```

**Do not claim a task complete until `make ci` passes locally.** `go build` passing means nothing — prior commits have landed lint/fmt failures that CI rejected because only build+vet were run. When in doubt, run `make ci`.

The CLI has two config files: `fleet.json` (public, committed, example tracking only `rshade/gh-aw-fleet`) and `fleet.local.json` (private, gitignored, real fleet state). `LoadConfig` reads `fleet.json` as the **base** and overlays `fleet.local.json` on top — when both exist they are merged (local profiles/repos/defaults add to or override base entries); when only one exists it is used directly. `go run . list` prints `(loaded fleet.json + fleet.local.json)`, `(loaded fleet.json)`, or `(loaded fleet.local.json)` to stderr so you know which mode is active.

## Architecture big-picture

`gh-aw-fleet` is a **thin orchestrator** around three upstream tools: `gh aw` (the markdown→GitHub Actions compiler), `gh` (PR creation), and `git` (branching/commits). It never rewrites workflow markdown. All value is in *which repo gets which profile, when, pinned to which ref* — not in file manipulation. When adding features, prefer shelling out to upstream tools over re-implementing their logic.

**Declarative reconcile loop**: `fleet.json` declares desired state; `deploy`/`sync`/`upgrade` compute diffs and apply. Every command is dry-run by default and requires `--apply` for destructive operations.

**Pin-per-profile, keyed by source repo**: Each profile has a `sources` map (e.g., `github/gh-aw` → `v0.68.3`, `githubnext/agentics` → `main`). Bumping one source ref re-pins every workflow in that profile from that source. Profile definitions in `fleet.json` must match `profiles/default.json` verbatim for the `default` profile.

**Framing that shapes everything**: `github/gh-aw` ships the compiler + dogfooding tests; `githubnext/agentics` ships the curated library. When adding workflows to profiles, prefer agentics unless no equivalent exists. `github/gh-aw`'s `main` often contains unreleased features (like `mount-as-clis`) that break the installed CLI — always pin gh-aw sources to tags, never to `main`.

**Critical asymmetry**: `gh aw add` uses `fleet.json` pins (via `ResolvedWorkflow.Spec()`). `gh aw update` follows the workflow's *own* frontmatter `source:` line, not `fleet.json`. This means fleet.json edits don't propagate through `upgrade` — they need a `fleet sync --apply --force <repo>` to re-pin workflows to current fleet.json refs.

**`gh aw` path conventions differ by source**: agentics uses 3-part specs (`githubnext/agentics/<name>@ref`, implicit `workflows/` dir); gh-aw needs 4-part (`github/gh-aw/.github/workflows/<name>.md@ref`). `internal/fleet/fetch.go`'s `SourceLayout` map encodes this; `ResolvedWorkflow.Spec()` consumes it. Adding a third source means adding a `SourceLayout` entry.

**Diagnostic layer**: `internal/fleet/diagnostics.go` defines `CollectHints(texts...)` which scans error output for known patterns (unknown-property, HTTP 404, gpg failure) and returns actionable remediations. `cmd/deploy.go` and `cmd/upgrade.go` surface hints in failure paths. When a new class of error shows up, add a hint entry — it's a single-file edit.

## Hard invariants

- **Never bypass gpg signing.** Don't add `--no-gpg-sign`, `-c commit.gpgsign=false`, or `git config commit.gpgsign false` to any command. The tool lets gpg fail; the user finishes manually in their shell. `.claude/settings.json` denies these at the allowlist level.
- **Never run `git add`, `git commit`, `git push` from the Bash tool.** The Go tool invokes these via `exec.Command` inside `Deploy`/`Upgrade`/`Sync` — that's the one legitimate path. Claude never invokes git directly. `.claude/settings.json` deny list enforces this.
- **Commit messages and PR titles use Conventional Commits with `ci(workflows)` scope.** See `commitMessage()` / `upgradeTitle()` in `internal/fleet/`. Subject ≤72 chars, no trailing period, type is always `ci` (these ARE CI configuration changes).
- **Never hand-edit `CHANGELOG.md`.** Release-please manages it: on every push to `main`, the action in `.github/workflows/release-please.yml` opens a release PR that prepends a `## [version]` section generated from conventional commits. The type→section map lives in `release-please-config.json` (`feat:` → Added, `fix:` → Fixed, `refactor:` → Changed, `perf:` → Performance, `docs:` → Documentation; `chore:`/`ci:`/`build:`/`test:`/`style:` are hidden). The **commit message IS the changelog entry** — write the subject accordingly. Manual edits to version sections will be overwritten. This overrides the global CLAUDE.md rule "always update CHANGELOG.md." (The stale `## Unreleased` block at the top is a pre-automation artifact, not a pattern to extend.)
- **Clone dirs at `/tmp/gh-aw-fleet-*` are breadcrumbs after failure** — the tool preserves them when `--apply` fails mid-pipeline. Don't delete them; the user inspects / resumes from them.
- **`--work-dir` resumes a prior run.** Deploy's commit gate checks `hasStagedOrUnstagedWorkflowChanges` so a previously-interrupted `--apply` completes correctly when re-invoked with `--work-dir <clone>`.

## Code self-documentation

- **Every exported identifier gets a godoc comment.** Package, type, function, method, const, var, and exported struct field. Format: one complete sentence starting with the identifier name and ending with a period (`// Config is the declarative desired state for the fleet.`). For `Opts` / `Result` structs, document field meaning inline (`Force bool // pass --force to gh aw add`) — mirror the `DeployOpts` / `DeployResult` pattern in `internal/fleet/deploy.go`.
- **One `// Package foo` comment per package** on any file. Keep it short (1–3 sentences describing scope, not a feature tour).
- **Prefer expressive names over comments for unexported code.** If an internal helper needs a comment to explain *what* it does, rename it. Comments on unexported code should explain *why* — constraints, invariants, non-obvious tradeoffs — not restate the code.
- **`make lint` enforces this** via `revive` and `staticcheck`. Do **not** re-add the `should have a package comment` / `exported X should have comment` suppressions to `.golangci.yml` — they were removed deliberately. If a check fires against a legitimate exception, narrow the suppression to that path, don't blanket-disable the rule.
- **Two `SchemaVersion` constants live in this codebase.** `cmd.SchemaVersion` versions the JSON output envelope (wire contract); `fleet.SchemaVersion` versions the on-disk fleet.json format. They bump independently. Godoc on both must state which they are — the linter won't catch that they mean different things.

## Testing deploys requires care

`--apply` pushes a branch and opens a PR on an **external** repository. In interactive sessions, always get explicit user go-ahead ("go", "apply", "yes") in the turn before running `--apply`. In subagent test contexts, hard-stop instructions must forbid `--apply` — dry-runs are the test surface.

## Skills

The `skills/` directory contains five SKILL.md files codifying recurring operator workflows: `fleet-deploy`, `fleet-eval-templates`, `fleet-upgrade-review`, `fleet-onboard-repo`, and `fleet-build-profile`. Each skill encodes the three-turn pattern (dry-run → user approval → apply) and the gpg-failure manual-finish paste template. `fleet-build-profile` picks up where `fleet-eval-templates` stops — the latter evaluates which upstream workflows deserve inclusion, the former materializes a chosen set as a `profiles.<name>` entry in `fleet.json` (and, for the `default` profile, the mirrored `profiles/default.json`). When adding features that affect these flows, update the relevant SKILL.md alongside the code.

## .claude/settings.json

Committed at repo root; shared with collaborators and subagents. Allows `go build/vet/test/run`, `gh aw/api/repo/pr`, `git` read ops, and common shell tools. Denies `git add`, `git commit`, `git rebase --continue`, `git push --force`, `git reset --hard`. When adding a new developer command, add it to the allowlist so subagents don't prompt.

## Active Technologies
- Go 1.25.8 (from `go.mod`), using `github.com/spf13/cobra` v1.10.2 for CLI wiring and `github.com/rs/zerolog` v1.x for structured logging on stderr.
- `fleet.local.json` is the private, gitignored source of truth; `fleet.json` is the committed public example.
- Structured logging: `internal/log.Configure(level, format)` wires a zerolog global logger in root's `PersistentPreRunE`; warnings/errors/subprocess summaries emit on stderr, tabwriter status stays on stdout.
- Go 1.25.8 (from `go.mod`). + `encoding/json` (stdlib, new usage site); `github.com/spf13/cobra` v1.10.2 (existing); `github.com/rs/zerolog` v1.35.1 (existing, landed in #34). No new third-party dependencies — constitution Principle I. (main)
- N/A (no persistent state; envelope writes are transient to stdout). (main)

## Recent Changes
- 002-add-zerolog-logging: added `--log-level` / `--log-format` persistent flags; `⚠ WARNING:` lines in `deploy`/`sync` moved to stderr as structured `warn` events; subprocess summaries at `debug`.
- 001-add-subcommand: added `gh-aw-fleet add <owner/repo>` subcommand (cobra) for onboarding repos into `fleet.local.json`.
