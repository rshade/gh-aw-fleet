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

The project is at **v0.1.1**. The 2026-06-01 transition to usage-based Copilot
billing has shifted the near-term agenda: fleet operators need cross-repo cost
visibility, and fleet — uniquely positioned to know which repo runs which
profile pinned to which ref — is the natural layer to provide it.

## Immediate Focus (v0.2 — CLI completeness + billing readiness)

The CLI surface advertises seven subcommands but `status` (#10) remains a
stub. Closing that gap removes user surprise. Bundled in this milestone: a
deploy-time preflight (#11), one billing-readiness diagnostic (#52, the
smallest billing-related ship), and one freshly-discovered correctness bug
(#48).

### CLI completeness

- [ ] [#10](https://github.com/rshade/gh-aw-fleet/issues/10) Implement
  `status [repo]` subcommand `[M]`
  *Diff desired (`fleet.json`) vs actual (per-repo workflow set + pins)
  WITHOUT cloning. Reuses `fetch.go` to read upstream + target via `gh api`.
  Output: human-readable drift report + non-zero exit if drift exists.
  `--json` mode for CI integration.*
- [ ] [#11](https://github.com/rshade/gh-aw-fleet/issues/11) Preflight
  check for Actions enabled and workflow write permissions `[M]`
  *Two repo-level settings (Actions toggle, workflow token permissions)
  silently break every deployed workflow if mis-set. Add
  `checkActionsSettings()` alongside the existing `checkEngineSecret()`
  call; surface warnings with direct links to the settings page. Pure
  preflight — no auto-remediation.*

### Billing readiness (2026-06-01 transition)

- [ ] [#52](https://github.com/rshade/gh-aw-fleet/issues/52) Add diagnostic
  hint for HTTP 402 / Copilot billing-quota exceeded errors `[S]`
  *Recommended starting point in this batch. Single-file edit to
  `internal/fleet/diagnostics.go` `hints[]` table; exact pattern shape
  established by the existing unknown-property / 404 / gpg entries. Forward-
  references the `consumption` subcommand (#57) once it ships. Ship before
  2026-06-01 so operators get an actionable message instead of raw
  `gh aw` stderr the first time a billing cap fires.*

### Correctness

- [ ] [#48](https://github.com/rshade/gh-aw-fleet/issues/48) `sync`
  preflight + apply mis-trigger "refusing to resume" check on
  internally-prepared clones `[S]`
  *Surfaced after #8 landed in 2026-Q2. The resume guard fires on
  internally-prepared clone dirs that should pass. Single fix path; can
  ship as a `fix:` PR.*

## Near-Term Vision (v0.3 — billing visibility + operator quality)

Once Immediate Focus closes, the next layer is the cross-fleet consumption
view (#57 and its prerequisites #54/#55/#56), which is *the* fleet feature
the new billing model justifies. Wrapped around that: a security default
flip (#49), packaging (#43, gh-extension item), and the existing operator-QoL
items.

### Billing visibility (consumption tracking)

These four issues are sequenced: #54, #55, #56 are independent prerequisites
to #57. Land them in any order; #57 needs all three.

- [ ] [#54](https://github.com/rshade/gh-aw-fleet/issues/54) Add optional
  `tier` annotation to profile definitions `[M]`
  *Advisory `minimal | standard | premium` field on `Profile`. Surface in
  `list` output. No enforcement — annotation only. Becomes the `--by tier`
  group-by key for #57.*
- [ ] [#55](https://github.com/rshade/gh-aw-fleet/issues/55) Add optional
  `cost_center` field to `RepoSpec` `[M]`
  *Free-form string per repo. Surface in `list`. Same shape as #54 (additive
  metadata, no enforcement). Becomes the `--by cost-center` key for #57.*
- [ ] [#56](https://github.com/rshade/gh-aw-fleet/issues/56) Add
  `api-consumption-report` workflow to a fleet profile `[S]`
  *Pure profile composition — no Go code. Recommended path: new
  `observability-plus` opt-in profile (matches existing `*-plus` naming).
  Provides the per-repo data feed #57 aggregates. Cost-aware operators
  will care: the workflow itself consumes ~$1–2/day per repo running it.*
- [ ] [#57](https://github.com/rshade/gh-aw-fleet/issues/57) Add
  `gh-aw-fleet consumption` subcommand for cross-fleet billing rollups
  `[L]`
  *The fleet feature the new billing model justifies. Two-layer fetch:
  discovery via `gh api` discussions (category=audits + tracker marker),
  data via `aw_info.json` from run artifacts. `--latest` / `--trailing` /
  `--since` modes; `--by repo|profile|cost-center|workflow` grouping.
  Ships `cost *float64` placeholder for the future Copilot credit field
  (tracked separately as #59).*

### Distribution + security defaults

- [ ] [#43](https://github.com/rshade/gh-aw-fleet/issues/43) Add
  `install.sh` and `install.ps1` one-liner installers `[M]` `community`
  *Replace the four-step manual `gh release download` flow with curl/iwr
  one-liners. Ship both scripts as release assets (via
  `.goreleaser.yml` `extra_files`) and on `main` for a fallback URL.
  Acceptance: checksum-verified install of `v0.1.0` on
  ubuntu/macos/windows CI runners. Complements, does not replace, the
  `gh extension` packaging item below.*
- [ ] [#49](https://github.com/rshade/gh-aw-fleet/issues/49) Compile
  workflows with `--strict` on public repos by default `[S]` `security`
  *Public repos benefit from the upstream `gh aw` strict-mode validations
  (e.g., `permissions:` defaults, action-pin checks). Default to `--strict`
  when the target repo is public; document the auto-flip in the deploy
  output. Operators on private repos opt in via flag.*

### Operator quality of life

- [ ] `sync --dry-run` runs deploy preflight `[M]`
  *Today `sync --dry-run` computes the diff but doesn't validate that the
  resolved workflows would actually `gh aw add` cleanly (404s on bad pins,
  unknown-property errors, layout mismatches). Wire `gh aw add --dry-run`
  through the resolved set so drift surfaces before `--apply`.*
- [ ] Package as a `gh` extension `[M]` `community`
  *Move from `go run .` / built binary to
  `gh extension install rshade/gh-aw-fleet`. Requires renaming the binary
  (`gh-aw-fleet`), adding the extension manifest, and updating `goreleaser`
  to publish the right artifact shape. Lowers install friction for external
  contributors significantly. Related to #43 but a distinct distribution
  channel.*
- [ ] Expand `CollectHints` diagnostic catalog `[S]`
  *`internal/fleet/diagnostics.go` already scans for unknown-property, HTTP
  404, gpg failures, and (post-#52) billing-quota. Add hints for: rate-limit
  (HTTP 403/429), unsigned-commit rejection on protected branches,
  network-DNS errors, `gh auth` token expiry. Each hint is a single map
  entry plus a unit test.*
- [ ] Unit test scaffold for pure-logic packages `[M]`
  *`internal/fleet/load.go` (LoadConfig precedence, ResolveWorkflows merge
  logic) and `diagnostics.go` (CollectHints pattern matching) — both are
  pure and don't need subprocess mocking. Defer integration tests for
  `deploy.go` until later (require live `gh` + `git`).*

## Future Vision (Long-Term)

Speculative or contingent on upstream changes. Not committed; revisit when
the project has more shape or external contributors arrive.

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

### Billing visibility (deferred / upstream-blocked)

These three items extend the Near-Term billing-visibility work but are gated
on data we don't have yet, or on upstream changes outside our control. Each
is a placeholder; trigger conditions documented in the linked issue.

- [ ] [#53](https://github.com/rshade/gh-aw-fleet/issues/53) Expand
  billing-quota diagnostic patterns once real failures are observed `[S]`
  *Follow-up to #52. Initial patterns (`HTTP 402`, `Payment Required`) ship
  with high-confidence string matches; broader vocabulary (`spending limit`,
  `credit limit`, etc.) needs real failure samples to ground. Trigger:
  2–3 production failures captured post-2026-06-01.*
- [ ] [#59](https://github.com/rshade/gh-aw-fleet/issues/59) Surface
  Copilot credit attribution once `aw_info.json` `cost` field stabilizes
  upstream `[S]` `spec-first`
  *#57 ships with a `cost *float64` placeholder; this issue fills it. Field
  exists in upstream `aw_info.json` schema but is undocumented. Trigger:
  upstream documents semantics, OR fleet captures enough samples to
  reverse-engineer with a documented "may break on upstream changes"
  caveat.*
- [ ] [#60](https://github.com/rshade/gh-aw-fleet/issues/60) Profile-level
  model override (blocked on upstream `gh aw add --model`) `[S]` `spec-first`
  *Letting operators retarget a whole profile to a cheaper model under
  usage-based billing. Blocked by the thin-orchestrator / never-rewrite-
  markdown invariant; clean implementation requires upstream `--model`
  flag. File companion upstream feature request at `github/gh-aw`. Trigger:
  flag ships in a tagged release.*

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

- [x] [#32](https://github.com/rshade/gh-aw-fleet/issues/32) `upgrade`
  first changed-file missing leading dot (TrimSpace eats porcelain status
  char) `[S]`
  *Parser now reads `git status --porcelain` by column offset instead of
  trimming, preserving leading-dot paths like `.github/workflows/...`.*
- [x] [#8](https://github.com/rshade/gh-aw-fleet/issues/8) Resume `deploy`
  from `--work-dir` instead of re-cloning `[M]`
  *`Deploy` now detects a populated `--work-dir` with staged changes and
  re-enters at the commit gate (`hasStagedOrUnstagedWorkflowChanges`),
  skipping clone/init/add. Tested by interrupting `--apply` after gpg
  failure and re-running with `--work-dir <clone>`.*
- [x] [#7](https://github.com/rshade/gh-aw-fleet/issues/7) Surface
  missing-secret warning in PR body `[S]`
  *PR body now appends a `⚠ Setup Required` section when `MissingSecret`
  is set, so reviewers landing the PR hours later don't ship broken
  workflows.*
- [x] [#12](https://github.com/rshade/gh-aw-fleet/issues/12) Fix
  `default.json` pinning `githubnext/agentics` to `main` `[S]`
  *Default profile now pins agentics to a SHA, restoring the
  pin-to-released-tag invariant from CONTEXT.md (closest equivalent until
  upstream tags arrive).*
- [x] [#25](https://github.com/rshade/gh-aw-fleet/issues/25) Add
  `--output json` to `list`/`deploy`/`sync`/`upgrade` for LLM consumption
  `[M]`
  *Versioned JSON envelope (`schema_version: 1`) on stdout when
  `-o json` is set; tabwriter remains the default. Unlocks agentic
  consumers without regex-scraping. Spec at `specs/003-cli-output-json/`.*
- [x] [#30](https://github.com/rshade/gh-aw-fleet/issues/30) Errors
  double-print to stderr (cobra + `main` both print) `[S]`
  *Cobra's default error rendering silenced; main's zerolog handler now
  the sole error sink.*
- [x] [#31](https://github.com/rshade/gh-aw-fleet/issues/31) `not tracked
  in fleet.json` error ignores `fleet.local.json` `[S]`
  *Error message now names both `fleet.json` and `fleet.local.json` so
  operators know where to add the missing repo.*
- [x] [#24](https://github.com/rshade/gh-aw-fleet/issues/24) Introduce
  zerolog for errors, warnings, and diagnostics `[M]`
  *Added `internal/log` over zerolog, wired via cobra `PersistentPreRunE`
  with `--log-level` / `--log-format`. Warnings and subprocess summaries
  now emit as structured events on stderr; user-facing tabwriter output
  stays on stdout. Unblocked #25.*
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
  warnings on org-managed installations. Unblocked #7.*
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

> Immediate Focus items (#10, #11, #48, #52), Near-Term billing-visibility
> items (#54, #55, #56, #57), Near-Term distribution / security items (#43,
> #49), the Future Vision security epic (#36 with children #37–#40), and the
> Future Vision billing-deferred items (#53, #59, #60) are tracked as GitHub
> issues. The unlinked Near-Term and Future items don't have issues yet —
> open them as work picks up, then run `/roadmap sync` to keep this file
> aligned.
