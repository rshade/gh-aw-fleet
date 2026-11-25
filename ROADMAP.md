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

The project is at **v0.2.5** (2026-06-30). The 2026-06-01 transition to
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

- **Cost**: items labeled `cost` on GitHub — #102 (Immediate Focus),
  #198 / #204 (Near-Term), #106 / #107 / #53 / #59 / #60 (Future);
  #57 / #103 / #108 / #104 / #129 / #153 shipped in the v0.2.x line.
- **Security**: items labeled `security` on GitHub — the security epic
  #36 with `--deep-scan` #39 (Future); the `--strict`-gate remediation pair
  #180 / #179 and the renovate hardening #197 (shipped 2026-11-20, unreleased),
  plus the `--strict` HIGH-finding promotion #38, the defense-in-depth UX child
  #40, the supply-chain conflict scanners #101 / #100, and #49 (all shipped).

**Status**: release-please cuts **patch** bumps for `0.x` (latest tag
**v0.2.5**, 2026-06-30). v0.2.0 satisfied the rule (#54 / #55 / #56 cost
prereqs, #37 Layer 1 security scanner); the v0.2.1-v0.2.4 line shipped the cost
half repeatedly — the consumption rollup (#57), the `gh aw logs --json` AIC
source (#103 / PR #116), and the v0.79.2 FinOps baseline (#108) — alongside the
security half (#49 `--strict` on public repos, plus the supply-chain conflict
scanners #101 / #100 and the trigger-risk lint #104). **v0.2.5 shipped** the
cost + security trio that had been on `main`: the over-budget highlight (#129,
cost, PR #160) and the HIGH-finding `--strict` promotion (#38, security,
PR #161) — both closed 2026-06-23 — plus the cross-repo `overview` wedge (#153,
cost, closed 2026-06-29) and the defense-in-depth UX child (#40, security,
closed 2026-06-30). The **post-v0.2.5 in-flight release** (release-please
PR #181, `0.2.6`, still open) has its **security half filled**: #180 `fix`, #179
`feat`, and #197 all closed 2026-11-20. Its **cost half is not**: no
`cost`-labeled item has closed since v0.2.5, and #102 forecast `[L]` remains in
Immediate Focus. Because #179 is a `feat`, merging #181 before #102 lands would
cut a feature-bearing release without a cost item.

**Exception**: a release scoped as a single `fix:` hotfix (no feature
changes) is exempt — the rule applies to feature-bearing releases.

## Immediate Focus (next release in progress)

Latest tag is **v0.2.5** (2026-06-30). The **post-v0.2.5 in-flight release**
(release-please PR #181, `0.2.6`) already carries its security half: the
`--strict`-gate remediation pair #180 / #179 and the renovate hardening #197
all closed 2026-11-20, alongside a batch of operator, docs, and status work (see
Completed Milestones → 2026-Q4). The one remaining in-flight item is the cost
half, **#102 forecast**, which must land before #181 merges for the release to
satisfy the composition rule.

> `/roadmap sync` (2026-11-20): silent pass. Depth 1 (#102) equals
> `target_focus_depth` 1; security is covered by the closed #179 / #180 / #197,
> and cost is covered by #102 in flight, so no composition bump applies. Hold
> release-please PR #181 until #102 closes.

- [ ] [#102](https://github.com/rshade/gh-aw-fleet/issues/102)
  `gh-aw-fleet forecast` — fleet-wide pre-spend cost projection `[L]` `cost`
  `finops`
  *Wrap `gh aw forecast --json` across the fleet, aggregate projected AIC
  (P50/P95 per #108's spike), group `--by repo|profile|cost-center|tier`. The
  cost half of the in-flight release's composition — pre-spend/preventive,
  thematically paired with the security-hardening work.*
  *Promoted by /roadmap sync on 2026-07-07 — fills the cost composition slot for
  #179's feature-bearing release; picked over #198 as the foundational FinOps
  core.*

## Near-Term Vision (v0.4 — FinOps build-out + operator QoL)

Immediate Focus holds the cost item forecast (#102), the last open piece of the
post-v0.2.5 release. Near-Term holds the overview drill-down companion (#154),
the remaining FinOps work — consumption scale-hardening (#119), the
period-over-period `--compare` lens (#198), and the actor/trigger attribution
axes (#204) — plus the agentics-maintenance strict-check follow-up (#115) and the
docs-theme adoption (#147).

### Observability / cross-repo wedge

The local cross-repo health + cost + drift view. The `overview` lead (#153)
shipped 2026-06-29, and its model-support advisory (#201) and CI gate (#155)
shipped 2026-11-20 (see Completed Milestones). The drill-down companion (#154)
remains. A separate hosted control plane (#146) consumes the same engine for
org-wide observability; this CLI view ships standalone and locally first.

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

- [ ] [#198](https://github.com/rshade/gh-aw-fleet/issues/198)
  `consumption --compare` — period-over-period spend delta by workflow `[L]`
  `cost` `finops`
  *The "why did spend change?" lens: run the existing per-`--by` rollup over the
  current window and the immediately-preceding equal-length window, join on the
  group key, and rank per-key AIC deltas by magnitude. Completes the FinOps
  quartet (rollup answers *what*, `--budget` a static threshold, forecast
  *future*; `--compare` answers *what moved*). A ranked cost candidate to pair
  with #179's feature-bearing release.*
- [ ] [#204](https://github.com/rshade/gh-aw-fleet/issues/204)
  `consumption --by actor|trigger` — attribute AIC to what caused each run `[M]`
  `cost` `finops`
  *The "what is spending?" lens: two new run-grained group-by axes — `--by actor`
  (the run's `triggering_actor`, e.g. `dependabot[bot]`) and `--by trigger` (the
  GitHub event: `schedule` / `push` / `workflow_dispatch` / …) — so a spend spike
  can be named by its cause (dependabot churn vs. runaway cron vs. manual
  dispatch), not just its repo/workflow. Sibling of #198: `--compare` is the
  temporal derivative, this is the orthogonal dimensional axis; both from the same
  fleet-observability announcement thread. Needs a new `ghRunsMetaAPI` seam (one
  `actions/runs` list call per repo, joined by `run_id`) since the `gh aw logs`
  AIC payload carries no event/actor field.*
- [ ] [#119](https://github.com/rshade/gh-aw-fleet/issues/119) Paginate
  Actions workflow discovery for the logs consumption source `[M]`
  `finops`
  *`consumption --source logs` discovers workflows with a single
  `per_page=100` request; repos with >100 workflows silently undercount
  AIC. Follow pagination through the `ghWorkflowsAPI` seam; surface
  later-page failures as diagnostics, not silent partial totals.*

### Documentation

- [ ] [#147](https://github.com/rshade/gh-aw-fleet/issues/147) Adopt shared
  `rshade-theme` (design tokens) for a consistent look `[S]` `enhancement`
  *Follow-up to the now-shipped docs-site scaffold (#138): adopt the shared
  rshade-theme design tokens so the docs site matches the portfolio look.
  Triaged into Near-Term by `/roadmap sync` 2026-06-21 (was untriaged).*

### Operator quality of life

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
  Layer 1 (#37) shipped 2026-05-07, the `--strict` promotion (#38) shipped
  2026-06-23, and the defense-in-depth UX child (#40) shipped 2026-06-30 (v0.2.5).
  The `--strict`-gate remediation pair #180 / #179 shipped 2026-11-20 and makes
  `--strict` enableable on a real fleet; #39 remains eligible for promotion
  under the epic's own rule.*
  - [x] [#37](https://github.com/rshade/gh-aw-fleet/issues/37) Layer 1
    scanner: secrets + compiled-YAML + fleet-structural rules `[L]`
    (shipped 2026-05-07)
  - [x] [#38](https://github.com/rshade/gh-aw-fleet/issues/38) `--strict`
    flag: promote HIGH Layer 1 findings from advisory to blocking `[S]`
    (shipped 2026-06-23, PR #161)
  - [x] [#40](https://github.com/rshade/gh-aw-fleet/issues/40)
    Defense-in-depth UX: stderr + interactive prompt + PR body Security
    Findings section `[M]` (shipped 2026-06-30, v0.2.5)
  - [x] [#180](https://github.com/rshade/gh-aw-fleet/issues/180) `fix`: demote
    actionlint compiled-lock-file findings from HIGH to MEDIUM `[S]`
    (shipped 2026-11-20, PR #303)
  - [x] [#179](https://github.com/rshade/gh-aw-fleet/issues/179) `feat`:
    fleet-side `security.suppress` waiver for the `--strict` gate `[M]`
    (shipped 2026-11-20, PR #306)
  - [ ] [#39](https://github.com/rshade/gh-aw-fleet/issues/39)
    `--deep-scan` flag: prompt-injection regex signatures + optional
    classifier `[L]`
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

## Completed Milestones

### 2026-Q4

- [x] [#62](https://github.com/rshade/gh-aw-fleet/issues/62) `status`: opt-in
  `--resolve-refs` SHA-resolution drift mode. Closed 2026-11-20. `[S]`
- [x] [#61](https://github.com/rshade/gh-aw-fleet/issues/61) `status`: opt-in
  `--ref <branch>` for non-default-branch checks. Closed 2026-11-20. `[S]`
- [x] [#155](https://github.com/rshade/gh-aw-fleet/issues/155) `overview`:
  `--fail-on-runs` opt-in non-zero exit on run failures. Closed 2026-11-20.
  `[S]`
- [x] [#197](https://github.com/rshade/gh-aw-fleet/issues/197) `renovate`:
  protect generated `agentics-maintenance.yml` from bumps. Closed 2026-11-20.
  `[S]`
- [x] [#95](https://github.com/rshade/gh-aw-fleet/issues/95) `docs`: README
  footer backlink to the announcement post. Closed 2026-11-20. `[S]`
- [x] [#94](https://github.com/rshade/gh-aw-fleet/issues/94) `docs`: README
  names workflow examples, dry-run as principle. Closed 2026-11-20. `[S]`
- [x] [#127](https://github.com/rshade/gh-aw-fleet/issues/127) `docs`: two-layer
  FinOps cost model in `docs/finops.md`. Closed 2026-11-20. `[S]`
- [x] [#128](https://github.com/rshade/gh-aw-fleet/issues/128) `skills`:
  `fleet-budget-review` documents the `logs` source default. Closed 2026-11-20.
  `[S]`
- [x] [#179](https://github.com/rshade/gh-aw-fleet/issues/179) `security`:
  `security.suppress` waivers for the `--strict` gate. Closed 2026-11-20. `[M]`
- [x] [#99](https://github.com/rshade/gh-aw-fleet/issues/99) `diagnostics`:
  hints for missing `aw.yml` and already-exists `add` errors. Closed 2026-11-20.
  `[S]`
- [x] [#112](https://github.com/rshade/gh-aw-fleet/issues/112) `deploy`: forward
  `--engine` to the strict compile. Closed 2026-11-20. `[S]`
- [x] [#180](https://github.com/rshade/gh-aw-fleet/issues/180) `security`:
  demote actionlint lock-file findings to MEDIUM. Closed 2026-11-20. `[S]`
- [x] [#201](https://github.com/rshade/gh-aw-fleet/issues/201) `overview`: flag
  repos failing on unsupported Copilot models. Closed 2026-11-20. `[M]`

### 2026-Q2

- [x] [#40](https://github.com/rshade/gh-aw-fleet/issues/40) `security`:
  defense-in-depth UX: stderr, prompt, PR-body findings. Closed 2026-06-30.
  `[M]`
- [x] [#153](https://github.com/rshade/gh-aw-fleet/issues/153) `overview`:
  cross-repo health + cost + drift dashboard. Closed 2026-06-29. `[L]`
- [x] [#129](https://github.com/rshade/gh-aw-fleet/issues/129) `consumption`:
  read-only over-budget highlighting via `--budget`. Closed 2026-06-23. `[M]`
- [x] [#38](https://github.com/rshade/gh-aw-fleet/issues/38) `security`:
  `--strict` promotes HIGH Layer 1 findings to blocking. Closed 2026-06-23.
  `[S]`
- [x] [#156](https://github.com/rshade/gh-aw-fleet/issues/156) `config`: adopt
  `ax-go` phase 1: config primitives + `__schema`. Closed 2026-06-22. `[L]`
- [x] [#148](https://github.com/rshade/gh-aw-fleet/issues/148) `pkg/fleet`:
  export the fleet config contract into public `pkg/fleet`. Closed 2026-06-21.
  `[S]`
- [x] [#138](https://github.com/rshade/gh-aw-fleet/issues/138) `docs`: scaffold
  Astro Starlight docs site on `rshade-theme`. Closed 2026-06-20. `[M]`
- [x] [#104](https://github.com/rshade/gh-aw-fleet/issues/104) `security`:
  cost-oriented trigger-risk lint over the resolved fleet. Closed 2026-06-18.
  `[S]`
- [x] [#101](https://github.com/rshade/gh-aw-fleet/issues/101) `security`:
  detect Dependabot configs conflicting with gh-aw pins. Closed 2026-06-17.
  `[M]`
- [x] [#100](https://github.com/rshade/gh-aw-fleet/issues/100) `security`:
  detect Renovate configs conflicting with gh-aw pins. Closed 2026-06-16. `[M]`
- [x] [#98](https://github.com/rshade/gh-aw-fleet/issues/98) `deploy`: fix
  `ensureInit` stale-marker probe re-initing every run. Closed 2026-06-14. `[M]`
- [x] [#114](https://github.com/rshade/gh-aw-fleet/issues/114) `deploy`:
  per-repo fleet manifest drives init version drift. Closed 2026-06-13. `[L]`
- [x] [#108](https://github.com/rshade/gh-aw-fleet/issues/108) `finops`: gh-aw
  v0.79.2 baseline + `--json` / compile-env spikes. Closed 2026-06-12. `[M]`
- [x] [#103](https://github.com/rshade/gh-aw-fleet/issues/103) `consumption`:
  source AIC from `gh aw logs --json`. Closed 2026-06-12. `[M]`
- [x] [#43](https://github.com/rshade/gh-aw-fleet/issues/43) `install`:
  checksum-verified `install.sh` / `install.ps1` installers. Closed 2026-06-10.
  `[M]`
- [x] [#49](https://github.com/rshade/gh-aw-fleet/issues/49) `deploy`: compile
  with `--strict` on public repos by default. Closed 2026-05-21. `[S]`
- [x] [#57](https://github.com/rshade/gh-aw-fleet/issues/57) `consumption`:
  cross-fleet billing rollup subcommand. Closed 2026-05-15. `[L]`
- [x] [#48](https://github.com/rshade/gh-aw-fleet/issues/48) `sync`: fix false
  "refusing to resume" on prepared clones. Closed 2026-05-14. `[S]`
- [x] [#73](https://github.com/rshade/gh-aw-fleet/issues/73) `config`: HuJson
  comments and trailing commas in config files. Closed 2026-05-12. `[M]`
- [x] [#55](https://github.com/rshade/gh-aw-fleet/issues/55) `config`: optional
  `cost_center` field on `RepoSpec`. Closed 2026-05-12. `[M]`
- [x] [#54](https://github.com/rshade/gh-aw-fleet/issues/54) `config`: optional
  `tier` annotation on profiles. Closed 2026-05-12. `[M]`
- [x] [#56](https://github.com/rshade/gh-aw-fleet/issues/56) `profiles`:
  `observability-plus` profile with `api-consumption-report`. Closed 2026-05-10.
  `[S]`
- [x] [#52](https://github.com/rshade/gh-aw-fleet/issues/52) `diagnostics`: hint
  for HTTP 402 Copilot billing-quota errors. Closed 2026-05-10. `[S]`
- [x] [#37](https://github.com/rshade/gh-aw-fleet/issues/37) `security`: Layer 1
  scanner: secrets, compiled-YAML, structural rules. Closed 2026-05-07. `[L]`
- [x] [#11](https://github.com/rshade/gh-aw-fleet/issues/11) `deploy`: preflight
  for Actions enabled + workflow write permissions. Closed 2026-05-01. `[M]`
- [x] [#10](https://github.com/rshade/gh-aw-fleet/issues/10) `status`: `status
  [repo]` drift subcommand without cloning. Closed 2026-04-29. `[M]`
- [x] [#32](https://github.com/rshade/gh-aw-fleet/issues/32) `upgrade`: keep
  leading dot on first changed-file path. Closed 2026-04-28. `[S]`
- [x] [#8](https://github.com/rshade/gh-aw-fleet/issues/8) `deploy`: resume from
  `--work-dir` instead of re-cloning. Closed 2026-04-28. `[M]`
- [x] [#7](https://github.com/rshade/gh-aw-fleet/issues/7) `deploy`:
  missing-secret warning in the PR body. Closed 2026-04-28. `[S]`
- [x] [#12](https://github.com/rshade/gh-aw-fleet/issues/12) `profiles`: pin
  `githubnext/agentics` to a SHA, not `main`. Closed 2026-04-24. `[S]`
- [x] [#25](https://github.com/rshade/gh-aw-fleet/issues/25) `cmd`: `--output
  json` envelope for list/deploy/sync/upgrade. Closed 2026-04-23. `[M]`
- [x] [#30](https://github.com/rshade/gh-aw-fleet/issues/30) `cmd`: stop errors
  double-printing to stderr. Closed 2026-04-23. `[S]`
- [x] [#31](https://github.com/rshade/gh-aw-fleet/issues/31) `config`: "not
  tracked" error names `fleet.local.json` too. Closed 2026-04-23. `[S]`
- [x] [#24](https://github.com/rshade/gh-aw-fleet/issues/24) `log`: zerolog for
  errors, warnings, and diagnostics. Closed 2026-04-22. `[M]`
- [x] [#9](https://github.com/rshade/gh-aw-fleet/issues/9) `add`: `add
  <owner/repo>` onboarding subcommand. Closed 2026-04-20. `[M]`
- [x] [#6](https://github.com/rshade/gh-aw-fleet/issues/6) `deploy`: check
  org-level secrets before warning missing secret. Closed 2026-04-19. `[S]`
- [x] Bootstrap CLI, profiles, and operator skills (commit `8543956`)

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

> Tracked as GitHub issues: Immediate Focus — the cost item #102 (forecast),
> the last open piece of the post-v0.2.5 release; Near-Term — the overview
> drill-down companion (#154), the FinOps build-out (#198, #204, #119), the
> agentics-maintenance strict-check follow-up (#115), and the docs-theme
> adoption (#147); Future Vision — the security epic (#36 with child #39), the
> FinOps deferred set (#105, #106, #107, #113, #53, #59, #60), and the
> control-plane enablement cluster (#146 epic with #141–#145 and #163).
> **Closed 2026-11-20, unreleased** (release-please PR #181, `0.2.6`): the
> security half #180 / #179 / #197 plus #201, #155, #112, #99, #61, #62, #94,
> #95, #127, #128. Hold #181 until #102 lands so the release carries a cost
> item. Excluded as operational noise: #5 (Dependency Dashboard), #200 (`[aw]`
> No-Op Runs), #196 (pin-align chore, `dependencies`), and the `[aw]` Code
> Simplifier failure reports (#292–#295). Flagged for manual close: #162
> (Pluggable ConfigSource) duplicates #163 — close it as a dup (`/roadmap sync`
> lacks permission to close issues it did not open). The unlinked Near-Term and
> Future items don't have issues yet — open them as work picks up, then run
> `/roadmap sync`.
