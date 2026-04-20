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

The CLI has two config files: `fleet.local.json` (private, gitignored, real fleet state) and `fleet.json` (public, committed, example tracking only `rshade/gh-aw-fleet`). `LoadConfig` tries `fleet.local.json` first and falls back. `go run . list` prints `(loaded fleet.local.json)` or `(loaded fleet.json)` to stderr so you know which is active.

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
- **Clone dirs at `/tmp/gh-aw-fleet-*` are breadcrumbs after failure** — the tool preserves them when `--apply` fails mid-pipeline. Don't delete them; the user inspects / resumes from them.
- **`--work-dir` resumes a prior run.** Deploy's commit gate checks `hasStagedOrUnstagedWorkflowChanges` so a previously-interrupted `--apply` completes correctly when re-invoked with `--work-dir <clone>`.

## Testing deploys requires care

`--apply` pushes a branch and opens a PR on an **external** repository. In interactive sessions, always get explicit user go-ahead ("go", "apply", "yes") in the turn before running `--apply`. In subagent test contexts, hard-stop instructions must forbid `--apply` — dry-runs are the test surface.

## Skills

The `skills/` directory contains four SKILL.md files codifying recurring operator workflows: `fleet-deploy`, `fleet-eval-templates`, `fleet-upgrade-review`, `fleet-onboard-repo`. Each skill encodes the three-turn pattern (dry-run → user approval → apply) and the gpg-failure manual-finish paste template. When adding features that affect these flows, update the relevant SKILL.md alongside the code.

## .claude/settings.json

Committed at repo root; shared with collaborators and subagents. Allows `go build/vet/test/run`, `gh aw/api/repo/pr`, `git` read ops, and common shell tools. Denies `git add`, `git commit`, `git rebase --continue`, `git push --force`, `git reset --hard`. When adding a new developer command, add it to the allowlist so subagents don't prompt.

## Active Technologies
- Go 1.25.8 (from `go.mod`), using `github.com/spf13/cobra` v1.10.2 for CLI wiring.
- `fleet.local.json` is the private, gitignored source of truth; `fleet.json` is the committed public example.

## Recent Changes
- 001-add-subcommand: added `gh-aw-fleet add <owner/repo>` subcommand (cobra) for onboarding repos into `fleet.local.json`.
