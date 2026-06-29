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

The project is at **v0.2.4** (2026-06-22). The 2026-06-01 transition to
usage-based Copilot billing has shifted the near-term agenda: fleet operators
need cross-repo cost visibility, and fleet — uniquely positioned to know which
repo runs which profile pinned to which ref — is the natural layer to provide
it.

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

- **Cost**: items labeled `cost` on GitHub — #153 (Immediate Focus),
  #102 (Near-Term), #106 / #107 / #113 / #53 / #59 / #60 (Future);
  #57 / #103 / #108 / #104 / #129 shipped in the v0.2.x line and since.
- **Security**: items labeled `security` on GitHub — the security epic
  #36 with children #39 / #40 (Future); the `--strict` HIGH-finding
  promotion #38, the supply-chain conflict scanners #101 / #100, and #49
  (all shipped).

**Status**: release-please cuts **patch** bumps for `0.x` (latest tag
**v0.2.4**, 2026-06-22; `main` has unreleased commits awaiting the next tag).
v0.2.0 satisfied the rule (#54 / #55 / #56 cost prereqs, #37 Layer 1 security
scanner); the v0.2.1-v0.2.4 line shipped the cost half repeatedly — the
consumption rollup (#57), the `gh aw logs --json` AIC source (#103 / PR #116),
and the v0.79.2 FinOps baseline (#108) — alongside the security half (#49
`--strict` on public repos, plus the supply-chain conflict scanners #101 / #100
and the trigger-risk lint #104). The cost + security pair now **closed** on
`main` and awaiting the next tag is the over-budget highlight (#129, cost,
PR #160) and the HIGH-finding `--strict` promotion (#38, security, PR #161) —
both closed 2026-06-23. Immediate Focus has rotated to the cross-repo
`overview` wedge (#153, cost) to seed the following release's cost slot.

**Exception**: a release scoped as a single `fix:` hotfix (no feature
changes) is exempt — the rule applies to feature-bearing releases.

## Immediate Focus (next release in progress)

Latest tag is **v0.2.4** (2026-06-22). The prior Immediate Focus pair — the
over-budget rollup highlight (#129, cost, PR #160) and the HIGH-finding
`--strict` promotion (#38, security, PR #161) — both **closed 2026-06-23** and
now await the next release-please tag (see Completed Milestones). They cover the
in-flight release's cost + security composition slots. Per the operator's
2026-06-22 sequencing note, Immediate Focus has rotated to the cross-repo
`overview` wedge (#153, cost), which seeds the *following* release's cost slot
and unblocks the #154 / #155 observability cluster.

> `/roadmap sync` (2026-06-23) promoted #153 to single-WIP after both prior
> `roadmap/current` items closed the same day. Gate state: depth 0 → target 1,
> composition balanced (the closed pair #129 cost + #38 security covers the
> in-flight release), so exactly one slot opened. The mechanical top score was
> #40 (+20, epic-eligible), but the operator confirmed the pre-registered #153
> designation to keep the security epic from double-stacking this release.
> Demote/rotate again once #153 lands.

- [ ] [#153](https://github.com/rshade/gh-aw-fleet/issues/153)
  `gh-aw-fleet overview` — unified cross-repo health + cost + drift view `[L]`
  `cost` `finops` `cross-repo`
  *One read-only dashboard joining run health (failures/success %), cost
  (AIC/USD), and drift per repo, reusing the `Status()` + `gh aw logs`
  fan-outs. Ships standalone and locally; relates to EPIC #146.
  `unblocks: 146`. Drill-down companion #154 and CI gate #155 depend on it.*
  *Promoted by /roadmap sync on 2026-06-23 — refills the single WIP slot
  vacated by #129 / #38; anchors the next release's cost composition slot.*

## Near-Term Vision (v0.4 — FinOps build-out + operator QoL)

With the FinOps build-out shipped and the prior cost + security composition
pair (#129, #38) now closed and awaiting the next tag, Immediate Focus holds
the cross-repo `overview` wedge (#153). Near-Term holds its drill-down
companion (#154), the remaining FinOps work — forecast (#102) and consumption
scale-hardening (#119) — plus the deploy/consumption follow-ups (#112 / #115),
a diagnostics expansion (#99), the docs-theme adoption (#147), and four docs
items (#94 / #95 / #127 / #128).

### Observability / cross-repo wedge

The local cross-repo health + cost + drift view. The `overview` lead (#153) is
now in **Immediate Focus**; its drill-down companion remains here. A separate
hosted control plane (#146) consumes the same engine for org-wide
observability; this CLI view ships standalone and locally first.

- [ ] [#154](https://github.com/rshade/gh-aw-fleet/issues/154)
  `gh-aw-fleet debug [repo]` — aggregate failed agentic-run logs `[M]`
  `cross-repo`
  *Wraps `gh aw logs` and groups failed runs by root cause (via `CollectHints`)
  so overview's FAIL counts don't dead-end in the GitHub UI. Drill-down
  companion to #153.*

### FinOps / cost visibility

The 2026-06-01 usage-based Copilot billing transition makes AI-credit
(AIC) attribution a continuous operator concern; the v0.79.2 baseline
(#108, shipped in v0.2.2) captured the upstream `--json` surfaces these
build on.

- [ ] [#102](https://github.com/rshade/gh-aw-fleet/issues/102) `gh-aw-fleet
  forecast` — fleet-wide pre-spend cost projection `[L]` `cost` `finops`
  *Wrap `gh aw forecast --json` across the fleet, aggregate projected AIC
  (P50/P95 per #108's spike), group `--by repo|profile|cost-center|tier`.*
- [ ] [#119](https://github.com/rshade/gh-aw-fleet/issues/119) Paginate
  Actions workflow discovery for the logs consumption source `[M]`
  `finops`
  *`consumption --source logs` discovers workflows with a single
  `per_page=100` request; repos with >100 workflows silently undercount
  AIC. Follow pagination through the `ghWorkflowsAPI` seam; surface
  later-page failures as diagnostics, not silent partial totals.*

### Documentation

- [ ] [#127](https://github.com/rshade/gh-aw-fleet/issues/127) Document
  the two-layer FinOps cost model (aggregate rollup vs. per-run
  attribution) `[S]` `documentation` `finops`
  *New `docs/finops.md` framing Layer 1 (the deployed
  `api-consumption-report` plus the `consumption` rollup, AIC-native)
  vs. Layer 2 (optional agentics `cost-tracker`), with the currency
  caveat (Copilot AIC ≠ list-price USD) and the trigger-coupling caveat
  stated plainly. Links the FinOps issue cluster as the roadmap.*
- [ ] [#128](https://github.com/rshade/gh-aw-fleet/issues/128) Correct the
  stale `--source` default in the `fleet-budget-review` skill
  (artifacts → logs) `[S]` `documentation` `finops`
  *`skills/fleet-budget-review/SKILL.md` still documents the legacy
  `--source artifacts` path as the default; the real default is
  `--source logs` (#103). Rewrite the data-source sections, the example
  table (`AIC` / `COST` columns), and the Step-4 diagnostics to match.*
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
- [ ] [#147](https://github.com/rshade/gh-aw-fleet/issues/147) Adopt shared
  `rshade-theme` (design tokens) for a consistent look `[S]` `enhancement`
  *Follow-up to the now-shipped docs-site scaffold (#138): adopt the shared
  rshade-theme design tokens so the docs site matches the portfolio look.
  Triaged into Near-Term by `/roadmap sync` 2026-06-21 (was untriaged).*

### Operator quality of life

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
  Layer 1 (#37) shipped 2026-05-07 and the `--strict` promotion (#38) shipped
  2026-06-23; #39 / #40 remain eligible for promotion under the epic's own rule.*
  - [x] [#37](https://github.com/rshade/gh-aw-fleet/issues/37) Layer 1
    scanner: secrets + compiled-YAML + fleet-structural rules `[L]`
    (shipped 2026-05-07)
  - [x] [#38](https://github.com/rshade/gh-aw-fleet/issues/38) `--strict`
    flag: promote HIGH Layer 1 findings from advisory to blocking `[S]`
    (shipped 2026-06-23, PR #161)
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
- [ ] [#155](https://github.com/rshade/gh-aw-fleet/issues/155)
  `overview --fail-on-runs` — opt-in non-zero exit on run failures `[S]`
  `finops` `cross-repo`
  *Opt-in CI gate for the overview wedge (#153): exit non-zero when any repo
  has failed runs in the window; default stays drift-only / advisory.
  `trigger`: ships after #153 lands.*

### Control-plane enablement (agentic-fleet.ai)

Engine-side work to let a separate hosted control plane (`agentic-fleet.ai`,
private repo `agentic-fleet/control-plane`) consume `gh-aw-fleet` — in-process
via a public `pkg/` API, and by shelling out to the binary for deploys. The CLI
stays standalone; the control plane is the org-wide
observability / lifecycle layer. Long-horizon and strictly one-way (the engine
never depends on the control plane). See `agentic-fleet/control-plane/docs/`.

- [ ] [#146](https://github.com/rshade/gh-aw-fleet/issues/146) **[EPIC]**
  Control-plane enablement: public `pkg/` API + pluggable config source `[L]`
  `cross-repo`
- [ ] [#141](https://github.com/rshade/gh-aw-fleet/issues/141) Extract a public
  `pkg/fleet` config API (model + load/merge) `[L]` `cross-repo`
  *Foundational — `internal/fleet` can't be imported across a module boundary;
  unblocks #163 / #143 / #144.*
- [ ] [#163](https://github.com/rshade/gh-aw-fleet/issues/163) Pluggable
  `ConfigSource`: `FileSource` (default) + `RemoteSource` + `--config-source`
  `[L]` `cross-repo`
  *The binding point — CLI can pull config from a remote control plane;
  vendor-neutral, local default. Depends on #141.*
- [ ] [#143](https://github.com/rshade/gh-aw-fleet/issues/143) Version &
  document the remote fleet-config wire contract `[M]` `cross-repo` `spec-first`
  *The cross-repo handshake; reuses `fleet.SchemaVersion`. Depends on #141.*
- [ ] [#144](https://github.com/rshade/gh-aw-fleet/issues/144) Expose
  consumption (FinOps) + security analysis via `pkg/` for in-process reuse
  `[L]` `cross-repo` `finops` `security`
  *Reuse the existing analysis in-process. Depends on #141.*
- [ ] [#145](https://github.com/rshade/gh-aw-fleet/issues/145) Harden
  `deploy` / `sync` / `upgrade` for headless invocation by the control plane
  `[M]` `cross-repo`
  *Stable JSON envelopes, non-interactive, deterministic exit codes for the
  shell-out path. Relates to #163.*

  *Phase 1 (#156) of the broader `ax-go` adoption shipped 2026-06-22 — see
  Completed Milestones. Remaining ax-go phases (error-envelope adoption,
  `--output json` payload alignment, logger convergence, idempotency/mode/
  dry-run context) are unscheduled; open them as work picks up.*

### Miscellaneous

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

- [x] [#129](https://github.com/rshade/gh-aw-fleet/issues/129) Read-only
  over-budget highlighting in the rollup (`--budget`) `[M]` `cost` `finops`
  `spec-first`
  *Closed 2026-06-23 (PR #160). Optional per-row AIC ceiling that highlights
  hot rollup rows — the read-only "operate / anomaly" lens (no cap, no alert,
  exit 0). Filled the in-flight release's cost composition slot.*
- [x] [#38](https://github.com/rshade/gh-aw-fleet/issues/38) `--strict` flag:
  promote HIGH Layer 1 findings from advisory to blocking `[S]` `security`
  *Closed 2026-06-23 (PR #161). Child of the security epic #36, promotion-
  eligible since Layer 1 (#37) shipped 2026-05-07. Flips advisory HIGH findings
  to blocking under opt-in `--strict` across deploy/sync/upgrade. Filled the
  in-flight release's security composition slot; #36's remaining children
  (#39 / #40) stay eligible for future promotion.*
- [x] [#156](https://github.com/rshade/gh-aw-fleet/issues/156) Adopt `ax-go` as
  the AX foundation — phase 1 (amend constitution + config primitives +
  `__schema` discoverability) `[L]` `cross-repo` `spec-first`
  *Closed 2026-06-22 (PR #159). Adopted `github.com/rshade/ax-go v0.2.0` the
  constitutional way: `internal/fleet/load.go` now parses/patches via
  import-isolated `config.ParseFile` / `config.Patch`, `cmd` exposes a hidden
  additive `__schema` command, and `go.mod` declares `go 1.26.4`. Import
  boundary is `config`/`schema`/`contract` only. Remaining ax-go phases stay
  unscheduled (see Control-plane enablement).*
- [x] [#148](https://github.com/rshade/gh-aw-fleet/issues/148) Export fleet
  config contract into public `pkg/fleet` (types + SchemaVersion + JSON) —
  minimal first slice of #141 `[S]` `cross-repo`
  *Closed 2026-06-21. First slice of the control-plane epic (#146): the public
  `pkg/fleet` package now owns the `fleet.json` wire contract while
  `internal/fleet` aliases it.*
- [x] [#138](https://github.com/rshade/gh-aw-fleet/issues/138) Scaffold an Astro
  Starlight docs site consuming `rshade-theme` `[M]` `documentation`
  *Closed 2026-06-20. Reference implementation / first consumer of the shared
  theme. Follow-up theming tracked in #147.*
- [x] [#104](https://github.com/rshade/gh-aw-fleet/issues/104) Cost-oriented
  trigger-risk lint over the resolved fleet `[S]` `security` `cost` `finops`
  *Closed 2026-06-18. Flags trigger shapes that invite runaway AIC spend
  (unbounded schedules, fork-triggered LLM runs) across the resolved workflow
  set. Filled the next release's cost composition slot.*
- [x] [#101](https://github.com/rshade/gh-aw-fleet/issues/101) Detect
  Dependabot configs that conflict with gh-aw-managed pins `[M]` `security`
  *Closed 2026-06-17. Advisory `security_dependabot` scanner; security half of
  the supply-chain conflict pair (with #100).*
- [x] [#100](https://github.com/rshade/gh-aw-fleet/issues/100) Detect Renovate
  configs that conflict with gh-aw-managed pins `[M]` `security`
  *Closed 2026-06-16. Advisory `security_renovate` scanner; incident-driven
  (`finfocus#1246`).*
- [x] [#98](https://github.com/rshade/gh-aw-fleet/issues/98) Fix `ensureInit`
  probing a stale marker filename — re-inits every run, no drift detection
  `[M]` `bug`
  *Closed 2026-06-14. Superseded the marker-filename probe with the
  fleet-manifest version comparison (#114).*
- [x] [#114](https://github.com/rshade/gh-aw-fleet/issues/114)
  Fleet-managed marker/manifest per repo: deployed gh-aw version +
  detect/refresh stale init artifacts `[L]`
  *Shipped in v0.2.2 (PR #124, closed 2026-06-13). Deploy writes
  `.github/aw/fleet-manifest.json` (managed flag, gh-aw version,
  profiles); `ensureInit` now compares the manifest's `gh_aw_version`
  against the fleet's `github/gh-aw` source pin instead of probing a
  marker filename — a version mismatch triggers re-init. Supersedes the
  drift-detection half of #98.*
- [x] [#108](https://github.com/rshade/gh-aw-fleet/issues/108) gh-aw
  v0.79.2 baseline + capture `--json` / compile-env spikes `[M]` `cost`
  `finops`
  *Shipped in v0.2.2 (closed 2026-06-12). Raised the gh-aw floor to
  v0.79.2 across configs / docs / tests, recompiled the tracked locks,
  and captured real `forecast` / `logs --json` fixtures — the enabling
  spike for the FinOps build-out. Filled the v0.2.2 release-composition
  cost slot.*
- [x] [#103](https://github.com/rshade/gh-aw-fleet/issues/103) Source AIC
  from `gh aw logs --json`, not `aw_info.json` cost `[M]` `cost` `finops`
  *Shipped in v0.2.2 (PR #116, closed 2026-06-12). Pivoted the
  `consumption` rollup to `gh aw logs --json` (AIC; USD = `aic * 0.01`)
  behind a `--source logs|artifacts` flag, decoupled from any deployed
  report workflow — `--source logs` is now the default and no longer
  needs `api-consumption-report` deployed.*
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

> Tracked as GitHub issues: Immediate Focus — the cross-repo `overview` wedge
> (#153, cost), promoted 2026-06-23; Near-Term — its drill-down companion
> (#154), the FinOps build-out (#102, #119), the deploy/consumption follow-ups
> (#112, #115), a diagnostics hint (#99), the docs-theme adoption (#147), and
> docs items (#94, #95, #127, #128); Future Vision — the security epic (#36
> with children #39, #40), the FinOps deferred set (#105, #106, #107, #113,
> #53, #59, #60), the control-plane enablement cluster (#146 epic with
> #141–#145 and #163), the overview CI gate (#155), and the Status refinements
> (#61, #62). Closed on `main` awaiting the next tag: the in-flight cost +
> security composition pair #129 / #38 (both closed 2026-06-23), alongside the
> ax-go phase 1 (#156) and the `pkg/fleet` export (#148). Excluded as
> operational noise: #5 (Dependency Dashboard) and the `[aw]` bot failure
> reports (#140 / #151 / #157). Note: #162 duplicates #163 (Pluggable
> ConfigSource) and is unlabeled — close it as a dup. The unlinked Near-Term
> and Future items don't have issues yet — open them as work picks up, then
> run `/roadmap sync`.
