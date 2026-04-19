# gh-aw-fleet Context & Boundaries

> **Purpose**: This document is the project constitution. Every feature, idea,
> and PR must be evaluated against the boundaries below. If a proposal would
> violate them, the answer is either "no" or "delegate it to an upstream tool."
> See `ROADMAP.md` for what we are actively building within these boundaries.

## Core Architectural Identity

`gh-aw-fleet` is a **declarative fleet manager for GitHub Agentic Workflows**.
It maintains consistent agentic-workflow deployments across a set of target
repositories by reconciling desired state (`fleet.json`) to actual state via
the upstream `gh aw` extension. It is a **thin orchestrator** — its value lies
in *which repo gets which profile, when, pinned to which ref* — not in any
file manipulation it performs itself.

The tool composes three upstream CLIs (`gh aw`, `gh`, `git`) into one
declarative reconcile loop. It does not replace, fork, or wrap their behavior.

## Technical Boundaries (Hard No's)

These are non-negotiable. Crossing any of them is grounds to reject a feature
even when technically feasible.

### Workflow content

- **Never rewrite or edit workflow markdown.** Compilation is `gh aw`'s job.
  We pass specs (`owner/repo/path@ref`) and trust the upstream tool. If `gh aw`
  doesn't expose a behavior we need, the fix is upstream — not a workaround
  that touches the markdown directly.
- **Never re-implement `gh aw` compiler logic.** No parsing of frontmatter for
  rewriting, no patching of generated `.lock.yml` files, no duplicate validation
  of workflow schemas. Validation that already lives in `gh aw` is not our
  problem to re-host.

### State

- **No persistent state outside `fleet.json` / `fleet.local.json`.** No SQLite,
  no embedded KV, no cache server, no daemon mode. Every run reads the JSON,
  shells out, and exits. Clone directories under `/tmp/gh-aw-fleet-*` are
  preserved on failure as breadcrumbs but are *not* a state store — the user
  inspects them or `--work-dir` resumes from them, then they are discarded.
- **No long-running services.** No `fleet serve`, no webhook listener, no
  background reconciler. Reconciliation is human-triggered.

### Mutating operations

- **Never bypass gpg signing.** No `--no-gpg-sign`, no
  `-c commit.gpgsign=false`, no `git config commit.gpgsign false`. When gpg
  fails the tool stops and prints a manual-finish recipe; the user completes
  the commit in their own shell. This is enforced at the `.claude/settings.json`
  deny-list level.
- **Never run `git add` / `git commit` / `git push` outside the Go binary's
  own `exec.Command` calls.** Subagents and ad-hoc shell sessions invoke them
  through the tool, never directly.
- **Every destructive operation is dry-run by default.** `deploy`, `sync`, and
  `upgrade` print what they *would* do unless `--apply` is passed. Subagents
  in test contexts must be hard-stopped from passing `--apply`.
- **Pin `gh aw` sources to released tags, never to `main`.** Upstream `main`
  often contains unreleased features that break the installed CLI; we pin
  conservatively in `profiles/default.json` and bump deliberately.

### Conventions

- **Conventional Commits with `ci(workflows)` scope** for any commit the tool
  produces. Subject ≤ 72 chars, no trailing period, type is always `ci`
  (these are CI configuration changes, not features). See `commitMessage()`
  and `upgradeTitle()` in `internal/fleet/`.

## Data Source of Truth

- **Fleet desired state** lives in `fleet.local.json` (private, gitignored)
  with `fleet.json` as the public example. Read by `internal/fleet/load.go`
  via `LoadConfig`.
- **Profile workflow sets** live in `fleet.json`'s `profiles{}` map (or
  separate files under `profiles/`) and are resolved by `ResolveWorkflows`.
- **Source pins (`ref`)** live in each profile's `sources{}` map, keyed by
  source repo. Consumed by `ResolvedWorkflow.Spec()` when calling `gh aw add`.
- **Source layout** (path shape per upstream) lives in the `SourceLayout` map
  in `internal/fleet/fetch.go` and is consumed by `ResolvedWorkflow.Spec()`.
- **Per-repo overrides** live in `repos[<repo>]` in `fleet.local.json` and are
  merged by `ResolveWorkflows`.
- **Workflow content** lives **upstream** (`github/gh-aw`,
  `githubnext/agentics`, or `local`) and is resolved at compile time by
  `gh aw` — never by us.
- **Template catalog** lives in `templates.json`, refreshed by
  `template fetch`, consumed by the `template` subcommand for evaluation.

`fleet.local.json` takes precedence; `fleet.json` is the public example tracking
only `rshade/gh-aw-fleet`. `LoadConfig` prints `(loaded fleet.local.json)` or
`(loaded fleet.json)` to stderr.

A critical asymmetry: `gh aw add` uses our pins (via `ResolvedWorkflow.Spec()`),
but `gh aw update` follows the *workflow's own* `source:` frontmatter, not
`fleet.json`. To re-pin existing workflows after editing `fleet.json` requires
`fleet sync --apply --force <repo>`.

## Interaction Model

### Inbound

- CLI invocation: `go run . <subcommand>` or built binary
- Operator-driven via Claude skills (`fleet-deploy`, `fleet-eval-templates`,
  `fleet-onboard-repo`, `fleet-upgrade-review`) — each skill encodes the
  three-turn pattern: dry-run → user approval → apply

### Outbound (subprocess only)

- `gh aw {init,add,update,upgrade}` — workflow compilation, install, version bumps
- `gh repo clone`, `gh pr create`, `gh api` — repo operations and metadata
- `git status / diff / commit / push / branch` — branch lifecycle and signed commits
- `gpg` (implicit, via `git commit -S`) — signing; failure → manual handoff

### Out of scope

- Direct GitHub API mutation for workflow files — always via `gh aw`
- AI-engine selection logic — fleet declares an `engine` default, `gh aw`
  enforces it
- Secret management — relies on the operator's `gh auth` and `gpg-agent`
  state; we do not store, transport, or prompt for secrets
- Workflow execution monitoring — that's GitHub Actions' UI

## Verification

Before merging a feature, ask:

1. **Does it shell out to an upstream tool, or does it duplicate one?** If
   it duplicates, push the work upstream instead.
2. **Does it persist state outside `fleet.json` / `fleet.local.json`?** If
   yes, redesign so the JSON remains the only authority.
3. **Does it perform a destructive op without a dry-run gate?** If yes, add
   one and require `--apply` to mutate.
4. **Does it touch git signing or shell out to git from a non-tool path?**
   If yes, it must go through the Go binary's `exec.Command` flow and respect
   gpg-agent failures (preserve clone, print manual-finish recipe).
5. **Does it require `--apply` in a test or subagent context?** If yes,
   redesign — only interactive operators with explicit approval pass `--apply`.
6. **Does it pin upstream `gh-aw` to `main` or another moving target?** If
   yes, change to a released tag.
