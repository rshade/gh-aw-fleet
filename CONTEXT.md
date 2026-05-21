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

## Roadmap Sync Behavior

The `/roadmap sync` skill consumes the policy below when deciding whether
to promote items into "Immediate Focus." Other skills (notably
`/create-issue`) detect that this project uses the `roadmap-meta`
issue-body convention by searching this section for the literal string
`roadmap-meta`. This is the first section in CONTEXT.md that prescribes
behavior for an external tool rather than for the codebase itself —
it is a policy contract, not a hard boundary.

### Promotion policy

- **`target_focus_depth: 1`** — Immediate Focus should hold exactly one
  open issue with the `roadmap/current` label at any time. Single-WIP
  discipline; the sync gate prompts to promote one more when the count
  drops below target.
- **`composition_required: [cost, security]`** — Each feature-bearing
  release (between two release-please tags) must include at least one
  issue per listed label. When the in-flight release is missing a slot,
  the sync gate temporarily bumps `target_focus_depth` by 1 to make
  room for the missing-slot item alongside the current one.
- **`procrastination_threshold_days: 7`** — If Immediate Focus has been
  empty for longer than this, the gate's intro escalates from
  informational to "no work has been in flight for N days; pick a
  candidate."
- **`epic_promotion: enforce`** — Children of an `[EPIC]` issue become
  eligible for the gate's ranking as soon as the parent closes the
  relevant prerequisite. Today this means #36's children (#38–#40)
  became eligible when #37 shipped; the gate surfaces them in the
  ranked candidate list without requiring manual `roadmap/future` →
  `roadmap/next` relabeling.

### Gate behavior (every sync run)

1. Compute current Immediate Focus depth (open issues with
   `roadmap/current`).
2. Compute effective target (`target_focus_depth` plus any
   composition bump).
3. **Silent pass** if current == target *and* composition is balanced.
4. **Prompt** if current < target *or* composition is unbalanced. The
   file write completes only after the operator selects from the
   ranked candidate list.
5. **Soft notice** if current > target. No gate; just a one-line
   summary suggesting demotion.

### Candidate scoring

Candidates include all open `roadmap/next` issues, plus `roadmap/future`
issues whose `roadmap-meta` declares an `epic-parent` whose
prerequisite has closed. Score (higher = better promotion fit):

- `+30` fills a missing composition slot for the in-flight release
- `+15` epic-promotion-eligible (parent's prerequisite closed)
- `+10` `effort/small`; `+5` `effort/medium`; `+0` `effort/large`
- `+10` has dependents (another issue's `roadmap-meta unblocks`
  references it)
- `+8`  labeled `community` and contributor pipeline is thin
- `-3`  labeled `spec-first` but has no `roadmap-meta` block (soft
  nudge to add trigger info; not a gate)
- `-20` `trigger-pending` field present and condition not yet met

The skill presents the top 3–4 ranked candidates via `AskUserQuestion`,
each option's description showing the top two score contributors plus
any negatives. The list always includes a "Deliberately empty (release
in flight)" escape hatch.

### "Deliberately empty" annotation

When the operator picks the escape hatch, the gate writes a one-line
breadcrumb into the Immediate Focus placeholder:

```text
*Deliberately empty (YYYY-MM-DD): <reason>. Set by /roadmap sync.*
```

On the next sync, the gate reads this and adapts its intro
("You marked this empty N days ago waiting on X. Has that changed?")
rather than treating the state as fresh. Since `/roadmap sync` is
manually invoked, the date is a courtesy reminder for future-you, not
an enforced gate.

### `roadmap-meta` issue-body convention

Issues opt into structured metadata by embedding an HTML comment block
anywhere in their body. Absent block = "always eligible, no metadata."

```html
<!-- roadmap-meta
trigger: <free-form human-readable trigger description>
trigger-pending: <YYYY-MM-DD | event-name | upstream-flag-name>
unblocks: 38, 39, 40
epic-parent: 36
-->
```

All fields are optional; include only those that apply. Semantics:

- `trigger` — human-readable; surfaced in the gate's candidate
  description so the operator knows *why* an item is deferred.
- `trigger-pending` — only `YYYY-MM-DD` values are auto-checked
  against today's date; free-form event names ("upstream flag ships")
  stay operator-evaluated, so the gate treats them as "not yet met"
  until the operator removes the field.
- `unblocks` — comma-separated issue numbers. Used by the
  `+10 has dependents` score adjustment in reverse.
- `epic-parent` — single issue number. Used by
  `epic_promotion: enforce` to detect when a child becomes
  promotion-eligible.

`/create-issue` detects the convention by the literal string
`roadmap-meta` appearing in this file and emits the block on new
issues only when at least one field has a known value. Bug fixes and
most enhancements don't need the block; spec-first items, epic
children, and items with documented triggers do.

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
