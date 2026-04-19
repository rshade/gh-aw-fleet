# gh-aw-fleet Strategic Roadmap

> **Boundaries**: All items below must respect the constitutional limits in
> [`CONTEXT.md`](./CONTEXT.md). Anything that crosses a hard no is either
> rejected or delegated to an upstream tool (`gh aw`, `gh`, `git`).
>
> **LOE legend**: `[S]` Small (1-2h) · `[M]` Medium (½d-1d) · `[L]` Large (multi-day)

## Vision

`gh-aw-fleet` is a thin, declarative orchestrator that keeps a small fleet of
repositories aligned to a curated set of GitHub Agentic Workflows. It composes
`gh aw`, `gh`, and `git` — it does not replace any of them. The roadmap below
is biased toward *operator ergonomics* (resume, preflight, packaging) over
*feature surface area* (new commands, new abstractions). When in doubt: less
code, more delegation.

The project is at **v0.1** — initial bootstrap landed in commit `8543956`
(`feat: bootstrap gh-aw-fleet CLI, profiles, and operator skills`).

## Immediate Focus (v0.2 — CLI completeness + deploy UX)

The CLI surface advertises seven subcommands but two are stubs (#9, #10).
Closing that gap removes user surprise and unblocks the `fleet-onboard-repo`
skill from leaning on manual `fleet.local.json` edits. Bundled in this
milestone: a small set of deploy-time UX fixes (#6, #7) that operators feel
on every run.

### CLI completeness

- [ ] [#8](https://github.com/rshade/gh-aw-fleet/issues/8) Resume `deploy`
  from `--work-dir` instead of re-cloning `[M]`
  *Recommended starting point. Today the flag exists but a retried run
  re-clones. Make `Deploy` detect a populated `--work-dir`, skip
  clone/init/add when staged changes exist, and re-enter at the commit gate
  (the `hasStagedOrUnstagedWorkflowChanges` check is already there). Tested
  by interrupting `--apply` after gpg failure.*
- [ ] [#9](https://github.com/rshade/gh-aw-fleet/issues/9) Implement
  `add <owner/repo>` subcommand `[M]`
  *Register a repo in `fleet.local.json` with profile selection, optional
  exclusions, and post-add validation. Dry-run by default; only `--apply`
  writes the file. Replaces the manual JSON-edit step in the
  `fleet-onboard-repo` skill.*
- [ ] [#10](https://github.com/rshade/gh-aw-fleet/issues/10) Implement
  `status [repo]` subcommand `[M]`
  *Diff desired (`fleet.json`) vs actual (per-repo workflow set + pins)
  WITHOUT cloning. Reuses `fetch.go` to read upstream + target via `gh api`.
  Output: human-readable drift report + non-zero exit if drift exists.
  `--json` mode for CI integration.*

### Deploy UX — warn before broken deploys

This trio shares a theme: surface broken-deploy conditions to the operator
*before* a PR merges, instead of letting workflows fail silently in
production.

- [ ] [#6](https://github.com/rshade/gh-aw-fleet/issues/6) Check org-level
  secrets to avoid false-positive missing-secret warnings `[S]`
  *Today `checkEngineSecret` only queries repo secrets. Org-managed
  installations get spurious warnings on every deploy. Mirror upstream
  `add-wizard` behavior: check repo, then fall back to org. Patch is
  drafted in the issue body. Prerequisite for #7 to be accurate.*
- [ ] [#7](https://github.com/rshade/gh-aw-fleet/issues/7) Surface
  missing-secret warning in PR body `[S]`
  *CLI prints the warning to the operator's terminal but the PR body is
  silent. Reviewers merging hours later land broken workflows. Append a
  `⚠ Setup Required` section to `prBody()` when `MissingSecret` is set.
  Implement after or alongside #6.*
- [ ] [#11](https://github.com/rshade/gh-aw-fleet/issues/11) Preflight
  check for Actions enabled and workflow write permissions `[M]`
  *Two repo-level settings (Actions toggle, workflow token permissions)
  silently break every deployed workflow if mis-set. Add
  `checkActionsSettings()` alongside the existing `checkEngineSecret()`
  call; surface warnings with direct links to the settings page. Pure
  preflight — no auto-remediation.*
- [ ] [#12](https://github.com/rshade/gh-aw-fleet/issues/12) Fix
  `default.json` pinning `githubnext/agentics` to `main` `[S]`
  *Active CONTEXT.md violation: the default profile pins agentics to a
  floating ref, which CLAUDE.md forbids. Upstream has no tags yet, so the
  issue lays out three options (SHA pin, push upstream for tags, document
  exception) and requires an explicit decision before a fix lands.*

## Near-Term Vision (v0.3 — operator quality of life)

Once the CLI is complete, the next layer is making it pleasant under load —
preflight before destructive ops, faster sanity checks, packaging.

- [ ] `sync --dry-run` runs deploy preflight `[M]`
  *Today `sync --dry-run` computes the diff but doesn't validate that the
  resolved workflows would actually `gh aw add` cleanly (404s on bad pins,
  unknown-property errors, layout mismatches). Wire `gh aw add --dry-run`
  through the resolved set so drift surfaces before `--apply`.*
- [ ] Package as a `gh` extension `[M]` `community`
  *Move from `go run .` / built binary to `gh extension install rshade/gh-aw-fleet`.
  Requires renaming the binary (`gh-aw-fleet`), adding the extension manifest,
  and updating `goreleaser` to publish the right artifact shape. Lowers install
  friction for external contributors significantly.*
- [ ] Expand `CollectHints` diagnostic catalog `[S]`
  *`internal/fleet/diagnostics.go` already scans for unknown-property, HTTP 404,
  and gpg failures. Add hints for: rate-limit (HTTP 403/429), unsigned-commit
  rejection on protected branches, network-DNS errors, `gh auth` token expiry.
  Each hint is a single map entry plus a unit test.*
- [ ] Unit test scaffold for pure-logic packages `[M]`
  *No `_test.go` files exist today. Start with `internal/fleet/load.go`
  (LoadConfig precedence, ResolveWorkflows merge logic) and `diagnostics.go`
  (CollectHints pattern matching) — both are pure and don't need subprocess
  mocking. Defer integration tests for `deploy.go` until later (require live
  `gh` + `git`).*

## Future Vision (Long-Term)

Speculative or contingent on upstream changes. Not committed; revisit when the
project has more shape or external contributors arrive. Organized by the three
lenses from the 2026-Q2 brainstorm session.

### Supply-chain trust (the prt-scan era)

Context: 2026 has seen AI-assisted supply-chain attacks against GitHub Actions
at machine speed (prt-scan campaign, April 2026). `gh-aw-fleet` sits at the
chokepoint where every workflow is pinned and deployed — it's the natural
layer to enforce fleet-wide policy that per-repo tools cannot.

- [ ] Pin hygiene validator `[S]`
  *New `fleet validate` command: scan `fleet.json` and every resolved
  `SourceRef` for floating refs (`main`, `latest`, branch names) and fail on
  violations. Configurable severity per source so exceptions get documented
  explicitly instead of living silently in profiles. Pure parse, no
  subprocess.*
- [ ] Source attestation check `[M]` `spec-first`
  *When GitHub's 2026 workflow-dep SHA-locking API ships, cross-check each
  fleet pin against the published attestation. Warn on mismatch. Blocked on
  upstream; file as a watch-item when the API lands.*
- [ ] Trigger-pattern lint across the fleet `[M]`
  *Scan installed workflows for known-risky trigger shapes
  (`pull_request_target` without fork guards, `issue_comment` without
  `author_association` filter). Zero-clone via `gh api`; shares plumbing with
  the proposed `status` subcommand (#10). Direct operator-facing response to
  prt-scan-class attacks.*

### Community / ecosystem

Context: `gh aw` is in technical preview; ~100 agentic workflows already
exist at GitHub. Anyone running >3 workflows across >2 repos hits the same
problems this project solves. These ideas lower the barrier for others to
adopt.

- [ ] Profile registry (lightweight) `[M]` `community`
  *`fleet import-profile github://owner/repo` clones a standalone profile
  repo into local `profiles/`. No registry service — delegates to `gh` +
  `git`. Unlocks sharing curated profiles (security-plus, compliance,
  per-language starters) without anyone building infrastructure.*
- [ ] `fleet init` scaffolding wizard `[M]` `community`
  *First-run experience: operator answers a few questions (Go? TypeScript?
  security-critical?) and gets a sensible starter `fleet.local.json`. Writes
  only to `fleet.local.json`, dry-run by default. Removes the biggest day-1
  friction.*
- [ ] First-class `local` source for custom workflows `[L]`
  *Elevate the `local` source type so internal workflows get the same
  lifecycle (SHA-pinned from operator's monorepo, visible in `status`,
  resolved in `list`) as upstream sources. Extends `SourceLayout`; requires
  thinking through how the operator's monorepo git state intersects fleet
  state. High payoff for enterprise adoption.*
- [ ] Third source layout (beyond `gh-aw` + `agentics`) `[M]` `spec-first`
  *Adding a new upstream like `anthropic/agents-library` is a single
  `SourceLayout` entry plus `ResolvedWorkflow.Spec()` consumer. Wait until a
  concrete second-party catalog exists upstream before generalizing.*

### Observability / closing the loop

Context: today the feedback loop ends at PR creation. Fleet-level has no
visibility into whether deployed workflows actually run successfully. These
ideas close the loop without becoming a daemon.

- [ ] Post-merge verification `[M]`
  *`fleet verify <repo>` reads via `gh api` whether each deployed workflow
  has had ≥1 successful run since its install SHA. Non-zero exit on
  failures. Catches "PR merged but Actions never ran" silently.
  Human-triggered; cron-wrappable externally.*
- [ ] Run-pattern drift detection `[M]`
  *`fleet audit --runs` reads recent runs per workflow via `gh api`. Flags
  workflows that haven't run in N days (`--stale-threshold`) or have failed
  100% of recent runs. Dead-code detection for agentic workflows that rot
  when forgotten.*
- [ ] Template-evaluation reputation layer `[M]`
  *`template eval` already grades workflows against profile taxonomy.
  Record the grade + timestamp in `templates.json` so
  `template list --sort-by-grade` surfaces trusted ones over time.
  Lightweight reputation store — no registry. Payoff scales with
  evaluation volume.*
- [ ] Fleet-wide audit dashboard (read-only, JSON-out) `[M]`
  *A `fleet audit --json` that summarizes pin freshness, drift count per
  repo, and last-deploy timestamp. Consumed by external tools (jq,
  dashboards) but produces no state itself — stays within boundaries. Could
  be the unified output surface for the three items above.*
- [ ] `template eval` deeper diff against installed workflows `[L]`
  *`template fetch` builds a catalog with frontmatter + body. A natural
  extension is to compare each profile's pinned workflow to the catalog's
  current `main` and report behavioral diffs (frontmatter changes, body
  diff hunks). Useful for `fleet-eval-templates` skill but a multi-day
  build.*

### Miscellaneous

- [ ] Per-profile `engine` override `[S]`
  *Today `engine` is a fleet-level default. Some profiles (e.g.,
  `security-plus`) might want a different engine. Schema change + resolution
  precedence: repo > profile > fleet default.*
- [ ] Cross-repo PR linking `[S]` `cross-repo`
  *When `upgrade --all` opens N PRs, link them to each other in PR bodies
  so a human reviewer can navigate the rollout. Pure formatting; no schema
  change.*

## Completed Milestones

### 2026-Q2

- [x] Bootstrap CLI, profiles, and operator skills (commit `8543956`)
  *Initial scaffolding: `cmd/`, `internal/fleet/`, `profiles/default.json`,
  four Claude skills, CI + release tooling, commitlint, golangci-lint.*

## Boundary Safeguards

These are mirrored from `CONTEXT.md` so reviewers don't need to context-switch.
Any roadmap item that violates these is rejected, not negotiated.

- Never bypass gpg signing (`--no-gpg-sign`, etc.)
- Never rewrite workflow markdown — defer to `gh aw`
- Never re-implement `gh aw` compiler logic
- No persistent state outside `fleet.json` / `fleet.local.json` (no DB, no daemon)
- Every destructive op is dry-run by default; `--apply` is the explicit gate
- Pin `gh aw` sources to released tags, never `main`
- Conventional Commits with `ci(workflows)` scope for tool-produced commits

## Tracking

- **GitHub issues**: <https://github.com/rshade/gh-aw-fleet/issues>
- **Roadmap phase labels**: `roadmap/current`, `roadmap/next`, `roadmap/future`
- **Effort labels**: `effort/small`, `effort/medium`, `effort/large`
- **Contribution flags**: `community`, `cross-repo`, `spec-first`

> Immediate Focus items (#8, #9, #10) are tracked as GitHub issues. The
> Near-Term and Future items don't have issues yet — open them as work picks
> up, then run `/roadmap sync` to keep this file aligned.
