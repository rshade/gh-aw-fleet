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

The project is at **v0.1.4**. The 2026-06-01 transition to usage-based Copilot
billing has shifted the near-term agenda: fleet operators need cross-repo cost
visibility, and fleet — uniquely positioned to know which repo runs which
profile pinned to which ref — is the natural layer to provide it.

## Release Composition Rule

Each tagged release should include at least one **cost-visibility** item
and at least one **security** item. Cost because the 2026-06-01
usage-based Copilot billing transition makes cost a continuous operator
concern; security because the prt-scan-era supply-chain landscape makes
fleet-layer policy enforcement the natural defense. A release that ships
only correctness work, or that lands one of the two domains without the
other, drifts the project from the two mandates that justify a
fleet-layer tool existing.

**Identifying candidates**:

- **Cost**: items labeled `cost` on GitHub — currently #53, #59, #60
  (#57 shipped, see Completed Milestones).
- **Security**: items labeled `security` on GitHub — the supply-chain
  conflict scanners #101 / #100 (live incident, promoted to Immediate
  Focus), the security epic #36 with children #38–#40, plus #49
  (shipped).

**Status**: v0.1.x → v0.2 satisfied this (#54 / #55 / #56 cost
prereqs, #37 Layer 1 security scanner). v0.3 is composition-complete
and awaiting its release-please cut: the cost half landed 2026-05-15
(#57 consumption rollup) and the security half landed 2026-05-21 (#49
`--strict` on public repos, PR #88). v0.4 needs a fresh cost + security
pair. Its security half is already forming: the Renovate / Dependabot
conflict scanners (#101 / #100, promoted to Immediate Focus on
2026-06-08 off a live `finfocus#1246` incident). On the cost side, the
likely candidate is a real-failure-triggered diagnostic refresh (#53).

**Exception**: a release scoped as a single `fix:` hotfix (no feature
changes) is exempt — the rule applies to feature-bearing releases.

## Immediate Focus (v0.4 in progress)

v0.2 is tagged (release-please commit `c416f89`, 2026-05-14). v0.3
shipped its composition pair — the cross-fleet consumption rollup
(#57, cost, PR #83) and the `--strict`-on-public-repos security default
(#49, PR #88) — plus the one-liner installers (#43, PR #92, 2026-06-10).
Immediate Focus now holds the v0.4 pair: the supply-chain conflict
scanners (#101 / #100, security — promoted 2026-06-08 off a live
`finfocus#1246` incident) and the FinOps v0.79.2 baseline (#108, cost —
implemented this session, PR pending). The `/roadmap sync` gate notes
depth 3 > target 1 (single-WIP soft notice); the extra slots are the
deliberate security-incident pair plus the cost baseline that unblocks
the FinOps roadmap (#102–#107, #112–#115).

- [ ] [#101](https://github.com/rshade/gh-aw-fleet/issues/101) Detect
  Dependabot configs that conflict with gh-aw-managed pins `[M]` `security`
  *Parse `.github/dependabot.yml` via `yaml.v3`; if a `github-actions`
  ecosystem block lacks `ignore` rules for the gh-aw actions, emit a
  `Finding` quoting the exact `ignore:` YAML. Dependabot can't
  glob-ignore lock files, so there's no Rule B equivalent — it protects
  dependency names only. Promoted by `/roadmap sync` on 2026-06-08 —
  operator override of single-WIP for the live `finfocus#1246` incident;
  fills v0.4's release-composition security slot.*
- [ ] [#100](https://github.com/rshade/gh-aw-fleet/issues/100) Detect
  Renovate configs that conflict with gh-aw-managed pins `[M]` `security`
  *Probe `renovate.json` / `.renovaterc[.json]` / `renovate.json5` /
  `.github/renovate.json` (hujson-tolerant) and emit a `Finding` per
  missing `packageRules` block: disable `gh-aw-actions` bumps (Rule A)
  and keep Renovate off generated `*.lock.yml` files (Rule B). Promoted
  by `/roadmap sync` on 2026-06-08 alongside its Dependabot sibling #101
  for full dependency-bot coverage.*
- [ ] [#108](https://github.com/rshade/gh-aw-fleet/issues/108) gh-aw
  v0.79.2 baseline + capture `--json` / compile-env spikes `[M]` `cost`
  `finops` `spec-first`
  *Raises the gh-aw floor to v0.79.2 across configs / docs / tests,
  recompiles the tracked locks, and captures real `forecast` / `logs
  --json` fixtures — the enabling spike for the FinOps roadmap
  (#102–#107). Implemented this session; PR pending (`feat(consumption)`
  closes #103 + #108). Fills v0.4's release-composition cost slot
  alongside the security pair #101 / #100.*

## Near-Term Vision (v0.4 — FinOps build-out + operator QoL)

With v0.3 shipped (#57 + #49 + #43) and Immediate Focus holding the v0.4
pair (#101 / #100 security, #108 cost baseline), Near-Term holds the
FinOps build-out the #108 spike unblocks — forecast (#102), the logs-AIC
source (#103), trigger-risk lint (#104) — plus the deploy/consumption
follow-ups surfaced this session (#112, #115), a deploy correctness bug
(#98), a diagnostics expansion (#99), and two docs items (#94 / #95).

### FinOps / cost visibility

The 2026-06-01 usage-based Copilot billing transition makes AI-credit
(AIC) attribution a continuous operator concern; the #108 v0.79.2 spike
captured the upstream `--json` surfaces these build on.

- [ ] [#103](https://github.com/rshade/gh-aw-fleet/issues/103) Source AIC
  from `gh aw logs --json`, not `aw_info.json` cost `[M]` `cost` `finops`
  *Pivot the `consumption` rollup to `gh aw logs --json` (AIC; USD =
  `aic * 0.01`) behind a `--source logs|artifacts` flag, decoupled from
  any deployed report workflow. **Implemented this session; PR pending.***
- [ ] [#102](https://github.com/rshade/gh-aw-fleet/issues/102) `gh-aw-fleet
  forecast` — fleet-wide pre-spend cost projection `[L]` `cost` `finops`
  *Wrap `gh aw forecast --json` across the fleet, aggregate projected AIC
  (P50/P95 per #108's spike), group `--by repo|profile|cost-center|tier`.*
- [ ] [#104](https://github.com/rshade/gh-aw-fleet/issues/104) Cost-oriented
  trigger-risk lint over the resolved fleet `[S]` `security` `cost` `finops`
  *Flag trigger shapes that invite runaway AIC spend (unbounded schedules,
  fork-triggered LLM runs) across the resolved workflow set.*

### Documentation

- [ ] [#94](https://github.com/rshade/gh-aw-fleet/issues/94) Surface
  dry-run as a design principle + add concrete workflow examples to the
  README `[S]` `documentation`
  *Two positioning gaps: the intro defines agentic workflows abstractly
  without naming what they do (code review, doc updates, malicious-code
  scans, PR fixes), and the dry-run gate reads as a feature in the
  Commands table rather than a load-bearing operator-UX principle. Net
  README growth < 20 lines; no code changes.*
- [ ] [#95](https://github.com/rshade/gh-aw-fleet/issues/95) Add
  backlinks to the announcement post (README footer + release notes +
  repo description) `[S]` `documentation`
  *Three low-maintenance placements pointing at the blog as the
  source-of-truth for announcements: a bottom-of-README Resources line,
  a release-notes "Related" link, and a repo-description link to the
  blog tag index. No top-of-README marketing link by design.*

### Operator quality of life

- [ ] [#98](https://github.com/rshade/gh-aw-fleet/issues/98) Fix
  `ensureInit` probing a stale marker filename — re-inits every run, no
  drift detection `[M]` `bug`
  *`ensureInit` probes `.github/agents/agentic-workflows.agent.md`, but
  real `gh aw init` (v0.77.5) writes `agentic-workflows.md` — the guard
  never short-circuits, so init re-runs every deploy/sync and every PR
  body falsely claims "this repo was not yet initialized." CI missed it
  because the fake-gh stub writes the same wrong filename. Fix: OR
  multiple real markers so `--no-agent`/`--no-skill` don't fool it, and
  fail loudly on the next upstream rename.*
- [ ] [#99](https://github.com/rshade/gh-aw-fleet/issues/99) Add
  `CollectHints` entries for `no aw.yml manifest` / `already exists`
  `gh aw add` errors `[S]`
  *Two `gh aw add` failure modes surface raw upstream output today: the
  missing-package-manifest error (agentics has no root `aw.yml`; the
  fleet installs workflows by name) and the `already exists in
  .github/workflows/` collision (re-run with fleet `--force`). Concrete
  instance of the broader catalog-expansion item below.*
- [ ] [#112](https://github.com/rshade/gh-aw-fleet/issues/112) `gh aw
  compile --strict` drops the `--engine` override, producing wrong-engine
  lock files `[S]` `bug`
  *Latent: a strict (public) deploy of an engine-overridden workflow
  recompiles to the markdown's native engine, demanding the wrong secret
  at runtime. The non-strict path was fixed this session; the strict path
  (`runGhAwCompileStrict`) needs the same `--engine` forwarding.*
- [ ] [#115](https://github.com/rshade/gh-aw-fleet/issues/115) Auto-suppress
  the `agentics-maintenance.yml` strict-check failure on public repos `[M]`
  *Every public repo the fleet deploys to inherits the generated
  `agentics-maintenance.yml`, whose required empty choice option fails
  actionlint's `syntax-check`. Deploy could write/merge a
  `.github/actionlint.yaml` suppression — carefully, never clobbering an
  operator's existing config (manual fix documented in README
  Troubleshooting).*

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
  Layer 1 (#37) shipped 2026-05-07 — by the epic's own promotion rule, the
  remaining children (#38–#40) are eligible for Near-Term in the next sync.*
  - [x] [#37](https://github.com/rshade/gh-aw-fleet/issues/37) Layer 1
    scanner: secrets + compiled-YAML + fleet-structural rules `[L]`
    (shipped 2026-05-07)
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

### FinOps (deferred / upstream-blocked)

Cost-visibility items gated on data we don't have yet, on upstream changes
outside our control, or that extend the FinOps build-out once the Near-Term
core (#102 / #103 / #104) lands. Trigger conditions documented in the linked
issues.

- [ ] [#107](https://github.com/rshade/gh-aw-fleet/issues/107) Tier-driven
  `GH_AW_DEFAULT_*` guardrail injection at compile `[M]` `cost` `finops`
  `spec-first`
  *Inject per-tier AIC / turn caps at `gh aw compile` time. #108's spike
  confirmed `GH_AW_DEFAULT_*` are honored as overridable defaults — can
  leave spec-first, no upstream FR needed.*
- [ ] [#106](https://github.com/rshade/gh-aw-fleet/issues/106) Cap-hit
  diagnostic hints (max-ai-credits / max-turns exceeded) `[S]` `cost`
  `finops` `spec-first`
  *`CollectHints` entries for AIC / turn-cap exhaustion so operators get an
  actionable message on first cap hit. Unblocked once #107 lands.*
- [ ] [#113](https://github.com/rshade/gh-aw-fleet/issues/113) `consumption
  --source logs`: bounded concurrency + no-download fast path `[M]` `finops`
  *The #103 per-workflow fan-out is O(N·M) sequential `gh aw logs` calls,
  each downloading artifacts. Bound concurrency + investigate a
  metadata-only path before it bites larger fleets.*
- [ ] [#105](https://github.com/rshade/gh-aw-fleet/issues/105) Track: OTel
  export / agentic-ops MCP — out-of-scope decision record `[S]` `community`
  `finops` `spec-first`
  *Decision record keeping observability-export out of fleet scope (no
  daemon, no persistent state); revisit only if upstream surfaces a
  pull-based MCP.*
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

- [ ] [#114](https://github.com/rshade/gh-aw-fleet/issues/114) Fleet-managed
  marker/manifest per repo: track deployed gh-aw version + detect/refresh
  stale init artifacts `[L]`
  *Write `.github/aw/fleet-manifest.json` on deploy (managed, gh-aw version,
  profiles); make `status` / `sync` version-aware so init-artifact drift is
  detectable instead of surfacing as a runtime failure. Supersedes the
  drift-detection half of #98.*
- [ ] Per-profile `engine` override `[S]`
  *Today `engine` is a fleet-level default. Some profiles (e.g.,
  `security-plus`) might want a different engine. Schema change + resolution
  precedence: repo > profile > fleet default.*
- [ ] Cross-repo PR linking `[S]` `cross-repo`
  *When `upgrade --all` opens N PRs, link them to each other in PR bodies
  so a human reviewer can navigate the rollout. Pure formatting; no schema
  change.*

### Status command refinements

Both follow-ups extend the v1 `status` command (#10, shipped 2026-Q2). They
were deferred at v1 to keep the initial command surface small.

- [ ] [#61](https://github.com/rshade/gh-aw-fleet/issues/61) Opt-in
  `--ref <branch>` flag for `status` non-default-branch checks `[S]`
  *Today `status` reads each repo's default branch. Some operators stage
  workflow changes on a feature branch; an opt-in `--ref` flag would let
  `status` diff against an arbitrary ref. Trigger: real demand from at
  least one external operator.*
- [ ] [#62](https://github.com/rshade/gh-aw-fleet/issues/62) Opt-in
  SHA-resolution drift mode for `status` to reduce false positives `[S]`
  *Today `status` does strict string comparison: a SHA pin pointing to the
  same commit as a tag is reported as drifted. Optional flag would resolve
  both refs to commit SHAs via `gh api` (2N extra calls per drifted
  candidate). Trigger: operators report content-identical false positives.*

## Completed Milestones

### 2026-Q2

- [x] [#43](https://github.com/rshade/gh-aw-fleet/issues/43) Add
  `install.sh` and `install.ps1` one-liner installers `[M]` `community`
  *Shipped 2026-06-10 in PR #92. Checksum-verified curl/iwr one-liners
  shipped as release assets (`.goreleaser.yml` `extra_files`) and on
  `main` for a fallback URL; tamper test asserts non-zero exit on a
  corrupted `checksums.txt`. The community-leverage half of the v0.3
  Immediate Focus.*
- [x] [#49](https://github.com/rshade/gh-aw-fleet/issues/49) Compile
  workflows with `--strict` on public repos by default `[S]` `security`
  *Shipped 2026-05-21 in PR #88. Auto-flips `gh aw compile --strict`
  on when the target repo is public (queried via `gh api`); operators
  on private repos remain opt-in via flag. Closes v0.3's release-
  composition security half. Pairs with #57 on the cost half — the
  composition pair the v0.3 release-please cut now ships.*
- [x] [#57](https://github.com/rshade/gh-aw-fleet/issues/57) Add
  `gh-aw-fleet consumption` subcommand for cross-fleet billing rollups
  `[L]`
  *Shipped 2026-05-15 in PR #83. Read-only fleet-wide aggregator over
  each repo's `api-consumption-report` output. Two-layer fetch:
  discovery via `gh api` discussions (`audits` category + tracker
  marker), data via `aw_info.json` + `run_summary.json` from run
  artifacts. Three temporal modes (`--latest` / `--trailing Nd` /
  `--since YYYY-MM-DD`); four group-by axes (`--by
  repo|profile|cost-center|workflow`). `cost *float64` placeholder ships
  nil-until-positive — populated when upstream `aw_info.json` `cost`
  field stabilizes (tracked as #59). The fleet feature the
  2026-06-01 usage-based Copilot billing transition justifies; satisfies
  v0.3's release-composition cost half.*
- [x] [#48](https://github.com/rshade/gh-aw-fleet/issues/48) `sync`
  preflight + apply mis-trigger "refusing to resume" check on
  internally-prepared clones `[S]`
  *Shipped 2026-05-14 in PR #81. Resume guard now correctly bypasses the
  check on internally-prepared clone directories that flow through
  deploy/sync's own state-prep path. Closes the last v0.2 correctness
  blocker.*
- [x] [#73](https://github.com/rshade/gh-aw-fleet/issues/73) Support HuJson
  for inline config documentation `[M]`
  *Shipped 2026-05-12 in PR #78. `internal/fleet/load.go` runs every
  config file (`fleet.json`, `fleet.local.json`, `templates.json`,
  `profiles/default.json`) through `hujson.Standardize()` before
  `json.Unmarshal`; writes use direct AST mutation (`Add`) or RFC 6902
  patches (`SaveTemplates`) to preserve operator-authored comments. The
  boundary note on third-party deps was resolved by Constitution v1.1.0
  §Third-Party Dependencies, which adds `tailscale/hujson` as an
  approved direct dependency.*
- [x] [#55](https://github.com/rshade/gh-aw-fleet/issues/55) Add optional
  `cost_center` field to `RepoSpec` `[M]`
  *Shipped 2026-05-12 in PR #78. Free-form string per repo, surfaced as
  an appended column in `gh-aw-fleet list`. Additive: no `SchemaVersion`
  bump, no envelope change, silently accepted when absent. Becomes the
  `--by cost-center` group-by key for #57.*
- [x] [#54](https://github.com/rshade/gh-aw-fleet/issues/54) Add optional
  `tier` annotation to profile definitions `[M]`
  *Shipped 2026-05-12 in PR #78. Advisory `minimal | standard | premium`
  field on `Profile`, surfaced as a parallel `TIERS` column in `list`.
  No enforcement — annotation only. Becomes the `--by tier` group-by
  key for #57.*
- [x] [#56](https://github.com/rshade/gh-aw-fleet/issues/56) Add
  `api-consumption-report` workflow to a fleet profile `[S]`
  *Shipped 2026-05-10 as the new `observability-plus` opt-in profile in
  `profiles/default.json`. Provides the per-repo data feed the upcoming
  `consumption` subcommand (#57) aggregates. Pure profile composition,
  no Go code. Opting a repo in incurs recurring Copilot-credit cost since
  the report is itself an LLM workflow.*
- [x] [#52](https://github.com/rshade/gh-aw-fleet/issues/52) Add diagnostic
  hint for HTTP 402 / Copilot billing-quota exceeded errors `[S]`
  *Shipped 2026-05-10 in the same release as #56. Added a `hints[]` entry
  in `internal/fleet/diagnostics.go` for `HTTP 402` / `Payment Required`
  patterns. Lands before the 2026-06-01 usage-based billing transition so
  operators get an actionable message on first cap hit. Follow-up #53
  expands the pattern catalog once real failures are observed.*
- [x] [#37](https://github.com/rshade/gh-aw-fleet/issues/37) Layer 1
  security scanner: secrets + compiled-YAML + fleet-structural rules `[L]`
  *First foundation of the security epic (#36): added `internal/security`
  scanner that runs against the resolved workflow set and surfaces findings
  on `DeployResult` / `SyncResult` / `UpgradeResult`. Advisory-only at this
  layer; promotion to blocking arrives with #38's `--strict` flag.*
- [x] [#11](https://github.com/rshade/gh-aw-fleet/issues/11) Preflight
  check for Actions enabled and workflow write permissions `[M]`
  *`checkActionsSettings()` runs alongside `checkEngineSecret()` during
  deploy preflight; surfaces direct settings-page links when either repo-
  level setting would silently break deployed workflows. Pure preflight,
  no auto-remediation.*
- [x] [#10](https://github.com/rshade/gh-aw-fleet/issues/10) Implement
  `status [repo]` subcommand `[M]`
  *Diffs desired (`fleet.json`) vs actual (per-repo workflow set + pins)
  via `gh api` without cloning. Human-readable drift report by default,
  `--output json` envelope for CI/LLM consumption, non-zero exit on drift.
  Strict-string comparison; SHA-resolution mode tracked separately as
  #62.*
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
- **Release-composition labels**: `cost`, `security` (see Release
  Composition Rule above)
- **Contribution flags**: `community`, `cross-repo`, `spec-first`
- **Issue metadata convention**: optional `<!-- roadmap-meta -->` HTML
  block in issue bodies; emitted by `/create-issue` when applicable,
  parsed by `/roadmap sync` to rank promotion candidates. Schema in
  [`CONTEXT.md` → Roadmap Sync Behavior →
  `roadmap-meta` issue-body convention][meta-spec]. Scaffolded in
  `.github/ISSUE_TEMPLATE/feature_request.md`.

[meta-spec]: ./CONTEXT.md#roadmap-meta-issue-body-convention

> Tracked as GitHub issues: Immediate Focus — the supply-chain scanners
> (#101, #100) and the FinOps v0.79.2 baseline (#108); Near-Term — the
> FinOps core (#102, #103, #104), the deploy/consumption follow-ups
> (#112, #115), the deploy bug (#98), diagnostics hint (#99), and docs
> items (#94, #95); Future Vision — the security epic (#36 with children
> #38–#40), the FinOps deferred set (#105, #106, #107, #113, #53, #59,
> #60), the fleet-manifest marker (#114), and the Status refinements
> (#61, #62). Shipped since the last sync: #43 (installers, 2026-06-10).
> Excluded as operational noise: #5 (Dependency Dashboard) and #111 (bot
> failure report). The unlinked Near-Term and Future items don't have
> issues yet — open them as work picks up, then run `/roadmap sync`.
