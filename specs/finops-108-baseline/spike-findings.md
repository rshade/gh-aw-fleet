# FinOps baseline spike — findings (issue #108)

Enabling spike that raises the gh-aw floor to **v0.79.2** and empirically
verifies the three upstream surfaces the FinOps roadmap (#102/#103/#104/#106/#107)
depends on. Run 2026-06-10 with `gh aw` pinned to **v0.79.2**.

> **Why v0.79.2 needs an explicit pin.** Every `gh aw` CLI release ≥ v0.78.0 is
> tagged **`Pre-release`**; v0.77.5 was the latest *stable*, so
> `gh extension upgrade aw` will **not** advance to v0.79.2. Install with:
> `gh extension remove aw && gh extension install github/gh-aw --pin v0.79.2`.

---

## 1. `GH_AW_DEFAULT_*` compile env vars — **VERDICT: HONORED as overridable defaults ✅**

These env vars are **not** in `gh aw compile --help` on v0.79.2, so the behavior
was tested empirically: a single workflow (`ci-doctor.md`) compiled in an isolated
git repo, baseline vs. env-set vs. frontmatter-override.

### Honored as compile-time defaults (baked into `.lock.yml`)

| Env var | Baseline lock value | With env set | Where in the lock |
| --- | --- | --- | --- |
| `GH_AW_DEFAULT_MAX_DAILY_AI_CREDITS=20` | `GH_AW_MAX_DAILY_AI_CREDITS: "500000"` | `"20"` | agent-job env |
| `GH_AW_DEFAULT_MAX_AI_CREDITS=5` | firewall `apiProxy.maxAiCredits: 1000` | `5` | `awf-config.json` |
| `GH_AW_DEFAULT_MAX_TURNS=3` | `GH_AW_MAX_TURNS: ${{ vars.GH_AW_DEFAULT_MAX_TURNS \|\| '' }}` | `3` | agent-job env |
| `GH_AW_DEFAULT_TIMEOUT_MINUTES=7` | `timeout-minutes: 10` (from frontmatter) | *unchanged* | — |

### Overridden by per-workflow frontmatter (so they are *defaults*, not forced)

Recompiling with frontmatter `max-turns: 99`, `max-ai-credits: 42`,
`max-daily-ai-credits: 77` **while the conflicting env vars were still set**:

| Knob | Env default | Frontmatter | Lock result |
| --- | --- | --- | --- |
| `max-turns` | 3 | 99 | **99** (frontmatter wins) |
| `max-daily-ai-credits` | 20 | 77 | **77** (frontmatter wins) |
| `max-ai-credits` (agent job) | 5 | 42 | **42** (frontmatter wins) |
| `timeout-minutes` | 7 | 10 | **10** (frontmatter wins) |

The frontmatter keys are documented in
`github/gh-aw` `pkg/parser/schemas/main_workflow_schema.json` (`max-turns`,
`max-ai-credits`, `max-daily-ai-credits`; deprecated alias `max-runs`).

**Nuance:** the override applies to the **primary agent job**. A *secondary*
job (e.g. activation/detection) still inherits the compile-time env default — so
`GH_AW_DEFAULT_*` acts as a floor where frontmatter is silent.

**Impact on #107:** the tier-driven guardrail-injection design is **viable as
specified** — inject `GH_AW_DEFAULT_*` at compile time per tier and individual
workflows can still raise/lower their own caps. **No upstream FR is required —
issue #107 can leave `spec-first`.**

---

## 2. `gh aw forecast --json` — schema captured (see `internal/fleet/testdata/forecast/`)

- **AIC-denominated** on v0.79.2 (was "effective tokens" on v0.77.5).
- Percentiles are **`p50_aic_per_run` + `p95_aic_per_run`** — **not** the
  P10/P50/P90 band #102 assumed. Projection fields:
  `{weekly,monthly,}_projected_aic`, `avg_aic`, `observed_runs_per_period`,
  `success_rate`.
- `sampled_runs: 0` ⇒ all-zero record (cold start), distinct from "cheap".
- Monte Carlo; slow; `--days` accepts only `7`/`30`; emits **partial** results on
  `--timeout` (minutes) expiry.

## 3. `gh aw logs --json` — schema captured (see `internal/fleet/testdata/logs/`)

- ✅ **`runs[].aic`** (per-run AI Credits) and ✅ **`summary.total_aic`** exist —
  the actual-spend signal #103 wants. `aic` key is **absent on failed runs**.
- ❌ **`episodes` / `edges` come back `null`** even with successful runs — #103's
  assumed `.episodes[].total_aic` path is unusable; source from
  `runs[].aic` / `summary.total_aic`.
- ⚠️ `runs[]` is **non-uniform** (`classification: baseline|normal`); only
  `normal` runs carry `token_usage`/`turns`/engine fields. No `cost` field at all
  under Copilot AI-Credits.

---

## Floor + ref bump shipped alongside this spike

- Documented + CI gh-aw floor raised to **v0.79.2** (hard bump: README, AGENTS.md,
  CONTEXT.md, `CompileStrictMinVersion`, diagnostics + tests).
- `github/gh-aw` source ref bumped `v0.68.3 → v0.79.2` across all profiles
  (`fleet.json`, `fleet.local.json`, `profiles/default.json`, `fleet.example.json`).
  All 7 gh-aw-sourced workflows verified to resolve at the `v0.79.2` tag.
- The 4 `.lock.yml` files recompiled under v0.79.2 (reconciles the stale
  `compiler_version: v0.68.3` metadata header).

## Cross-issue impact summary

| Issue | Spike result |
| --- | --- |
| #102 forecast | Schema grounded; **fix assumption to P50/P95 (not P10/P90), AIC not tokens.** |
| #103 AIC re-source | **`runs[].aic` + `summary.total_aic` confirmed; `episodes` is null — drop that path.** |
| #104 trigger-risk lint | Cap field names confirmed (`max-turns`/`max-ai-credits`); not directly gated. |
| #106 cap-hit hints | Unblocked once #107 lands — caps can now be set, so cap-hit strings become reproducible. |
| #107 guardrail injection | **Premise verified — env vars honored as overridable defaults; leave `spec-first`, no FR.** |
