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

The CLI surface advertises seven subcommands but one remains a stub (#10) —
`add` (#9) shipped in 2026-Q2. Closing the remaining gap removes user
surprise. Bundled in this milestone: a set of deploy-time UX fixes
(#7, #11, #12) operators feel on every run, plus three recently-discovered
correctness bugs (#30, #31, #32) that affect CLI error UX.

### CLI completeness

- [ ] [#8](https://github.com/rshade/gh-aw-fleet/issues/8) Resume `deploy`
  from `--work-dir` instead of re-cloning `[M]`
  *Recommended starting point. Today the flag exists but a retried run
  re-clones. Make `Deploy` detect a populated `--work-dir`, skip
  clone/init/add when staged changes exist, and re-enter at the commit gate
  (the `hasStagedOrUnstagedWorkflowChanges` check is already there). Tested
  by interrupting `--apply` after gpg failure.*
- [ ] [#10](https://github.com/rshade/gh-aw-fleet/issues/10) Implement
  `status [repo]` subcommand `[M]`
  *Diff desired (`fleet.json`) vs actual (per-repo workflow set + pins)
  WITHOUT cloning. Reuses `fetch.go` to read upstream + target via `gh api`.
  Output: human-readable drift report + non-zero exit if drift exists.
  `--json` mode for CI integration.*

### Deploy UX — warn before broken deploys

This theme surfaces broken-deploy conditions to the operator *before* a PR
merges, instead of letting workflows fail silently in production. #6 landed
in 2026-Q2; #7 and #11 continue the work.

- [ ] [#7](https://github.com/rshade/gh-aw-fleet/issues/7) Surface
  missing-secret warning in PR body `[S]`
  *CLI prints the warning to the operator's terminal but the PR body is
  silent. Reviewers merging hours later land broken workflows. Append a
  `⚠ Setup Required` section to `prBody()` when `MissingSecret` is set.
  Now unblocked since #6 shipped accurate org-level checks.*
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

### Correctness fixes

Post-release bugs surfaced by real use of `v0.1.0`. All `[S]`; can be
batched into a single `fix:` PR series.

- [ ] [#30](https://github.com/rshade/gh-aw-fleet/issues/30) Errors
  double-print to stderr (cobra + `main` both print) `[S]`
  *Cobra's default error rendering and `main`'s zerolog `fatal:` handler
  both fire on the same error. Silence one — likely `RootCmd.SilenceErrors`
  is already true but one error path writes directly.*
- [ ] [#31](https://github.com/rshade/gh-aw-fleet/issues/31) `not tracked
  in fleet.json` error ignores `fleet.local.json` `[S]`
  *Error message lies: the lookup goes through `LoadConfig` which merges
  both files, but the error string only names `fleet.json`. Fix the message
  to reflect merged-config semantics.*
- [ ] [#32](https://github.com/rshade/gh-aw-fleet/issues/32) `upgrade`
  first changed-file missing leading dot (TrimSpace eats porcelain status
  char) `[S]`
  *`git status --porcelain` emits a fixed-width status column followed by
  the path starting at column 4. The parser `TrimSpace`s the line, which
  eats the leading status char and then eats the first char of the path
  when it's a dot (e.g. `.github/workflows/...`). Parse by column offset,
  not by trim.*

## Near-Term Vision (v0.3 — operator quality of life)

Once the CLI is complete, the next layer is making it pleasant under load —
preflight before destructive ops, faster sanity checks, packaging, and
making the tool legible to LLM agents piping its output. The zerolog
foundation (#24) shipped in 2026-Q2 and unblocks #25.

- [ ] [#25](https://github.com/rshade/gh-aw-fleet/issues/25) Add
  `--output json` to `list`/`deploy`/`sync`/`upgrade` for LLM consumption
  `[M]`
  *No new data — just marshal the existing `DeployResult` / `SyncResult` /
  `UpgradeResult` structs as a versioned JSON envelope
  (`schema_version: 1`) when `-o json` is set. Unlocks agentic consumers
  without regex-scraping tabwriter tables. Spec lives in
  `specs/003-cli-output-json/`; foundation (#24) is in place.*
- [ ] [#43](https://github.com/rshade/gh-aw-fleet/issues/43) Add
  `install.sh` and `install.ps1` one-liner installers `[M]` `community`
  *Replace the four-step manual `gh release download` flow with curl/iwr
  one-liners. Ship both scripts as release assets (via
  `.goreleaser.yml` `extra_files`) and on `main` for a fallback URL.
  Acceptance: checksum-verified install of `v0.1.0` on
  ubuntu/macos/windows CI runners. Complements, does not replace, the
  `gh extension` packaging item below.*
- [ ] `sync --dry-run` runs deploy preflight `[M]`
  *Today `sync --dry-run` computes the diff but doesn't validate that the
  resolved workflows would actually `gh aw add` cleanly (404s on bad pins,
  unknown-property errors, layout mismatches). Wire `gh aw add --dry-run`
  through the resolved set so drift surfaces before `--apply`.*
- [ ] Package as a `gh` extension `[M]` `community`
  *Move from `go run .` / built binary to `gh extension install rshade/gh-aw-fleet`.
  Requires renaming the binary (`gh-aw-fleet`), adding the extension manifest,
  and updating `goreleaser` to publish the right artifact shape. Lowers install
  friction for external contributors significantly. Related to #43 but a
  distinct distribution channel.*
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

- [ ] [#36](https://github.com/rshade/gh-aw-fleet/issues/36) **[EPIC]**
  Add security scanning to `upgrade`/`sync`/`deploy` pipeline `[L]`
  *Umbrella: insert a scanner layer that catches secrets, dangerous
  compiled-YAML patterns, and fleet-structural violations before PRs merge.
  Scoped into four sub-issues (#37–#40) that ship independently; promote
  to Near-Term once #37 has a landing PR.*
  - [ ] [#37](https://github.com/rshade/gh-aw-fleet/issues/37) Layer 1
    scanner: secrets + compiled-YAML + fleet-structural rules `[L]`
  - [ ] [#38](https://github.com/rshade/gh-aw-fleet/issues/38) `--strict`
    flag: promote HIGH Layer 1 findings from advisory to blocking `[S]`
  - [ ] [#39](https://github.com/rshade/gh-aw-fleet/issues/39)
    `--deep-scan` flag: prompt-injection regex signatures + optional
    classifier `[L]`
  - [ ] [#40](https://github.com/rshade/gh-aw-fleet/issues/40)
    Defense-in-depth UX: stderr + interactive prompt + PR body Security
    Findings section `[M]`
- [ ] Pin hygiene validator `[S]`
  *New `fleet validate` command: scan `fleet.json` and every resolved
  `SourceRef` for floating refs (`main`, `latest`, branch names) and fail on
  violations. Configurable severity per source so exceptions get documented
  explicitly instead of living silently in profiles. Pure parse, no
  subprocess. Complements #37's rule set; could ship as a Layer 0 scanner.*
- [ ] Source attestation check `[M]` `spec-first`
  *When GitHub's 2026 workflow-dep SHA-locking API ships, cross-check each
  fleet pin against the published attestation. Warn on mismatch. Blocked on
  upstream; file as a watch-item when the API lands.*
- [ ] Trigger-pattern lint across the fleet `[M]`
  *Scan installed workflows for known-risky trigger shapes
  (`pull_request_target` without fork guards, `issue_comment` without
  `author_association` filter). Zero-clone via `gh api`; shares plumbing with
  the proposed `status` subcommand (#10). Natural Layer 2 rule for #37's
  scanner framework once that lands.*

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

- [x] [#24](https://github.com/rshade/gh-aw-fleet/issues/24) Introduce
  zerolog for errors, warnings, and diagnostics `[M]`
  *Added `internal/log` over zerolog, wired via cobra `PersistentPreRunE`
  with `--log-level` / `--log-format`. Warnings and subprocess summaries
  now emit as structured events on stderr; user-facing tabwriter output
  stays on stdout. Unblocks #25.*
- [x] [#9](https://github.com/rshade/gh-aw-fleet/issues/9) Implement
  `add <owner/repo>` subcommand for fleet onboarding `[M]`
  *New `gh-aw-fleet add <owner/repo>` cobra subcommand onboards repos into
  `fleet.local.json` with profile selection and post-add validation.
  Dry-run by default; `--apply` is the explicit gate. Replaces the manual
  JSON-edit step in the `fleet-onboard-repo` skill.*
- [x] [#6](https://github.com/rshade/gh-aw-fleet/issues/6) Check org-level
  secrets to avoid false-positive missing-secret warnings `[S]`
  *`checkEngineSecret` now queries repo secrets first, then falls back to
  org secrets — mirroring upstream `add-wizard`. Eliminates spurious
  warnings on org-managed installations. Unblocks #7 (PR-body warning
  accuracy).*
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

> Immediate Focus items (#7, #8, #10, #11, #12, #30, #31, #32), Near-Term
> items (#25, #43), and the Future Vision security epic (#36 with children
> #37–#40) are tracked as GitHub issues. The remaining Near-Term and Future
> items don't have issues yet — open them as work picks up, then run
> `/roadmap sync` to keep this file aligned.
