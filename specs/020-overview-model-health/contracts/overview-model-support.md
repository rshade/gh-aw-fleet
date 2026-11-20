# Contract: `overview` Model-Support Output

Additive to the existing `gh-aw-fleet overview` output contract. Nothing below renames, removes, or repurposes an existing field or column (FR-006).

## 1. Table layout — UNCHANGED

The header stays exactly `REPO DRIFT RUNS FAIL NOOP HEALTH AIC COST`. No column is added (spec Assumption: surfacing reuses the detail block + hints, not a new column). A clean fleet's table is byte-identical to today.

## 2. Text detail block (stdout) — additive line

An affected repo's detail block always renders — `needsOverviewDetail` gates on the affected status directly, because the open failure issue can outlive the failing run window and health gating alone could silently drop the annotation. The block gains one line, printed by `printOverviewDetail` after the runs line:

```text
rshade/finfocus (unhealthy):
  runs: 4 (4 failed, 0 no-op) · health 0% · cost -
  model-support: FAILING — offending model(s): claude-sonnet-4.6
```

Rules:

- **Affected, model named** (2-space indented under the repo header): `model-support: FAILING — offending model(s): <comma-joined sorted set>`
- **Affected, model unknown** (variable resolution failed): `model-support: FAILING — offending model(s): unknown`
- **Undetermined**: `model-support: undetermined — model-failure signal unavailable` — printed **only** when the detail block is already open for another reason (drift / unhealthy / runs-unavailable). Never forces a new detail block (FR-011).
- **Clean**: no line. A clean, drift-aligned repo prints no detail block at all (SC-003).

Ordering is deterministic: repos iterate in sorted order (existing behavior); the v1 collector resolves at most one offending model per repo, so `offending_models` is trivially ordered.

## 3. Remediation hint (stderr) — one per flagged repo

Routed through the existing hints channel (`splitOverviewDiags` → `emitOverviewDiagnostics` → `emitHints`). Emitted as a `fleet.Diagnostic{Code: DiagHint, Fields: {repo, hint}}` so it sorts by repo alongside the other hints:

```text
[hint] rshade/finfocus: agentic runs are failing on an unsupported/deprecated Copilot model.
       Set the GH_AW_DEFAULT_MODEL_COPILOT Actions variable on the repo to a model your
       Copilot subscription supports (no recompile required).
```

Rules:

- Exactly one hint per **affected** repo (FR-004, SC-005). No hint for undetermined or clean repos.
- Absent entirely when no repo is flagged (FR-008) — a healthy fleet emits no model hint.

## 4. JSON envelope — additive field

`overview --output json` payload; envelope `schema_version` stays `1` (FR-006, SC-004).

Affected repo (model named):

```json
{
  "repo": "rshade/finfocus",
  "drift_state": "aligned",
  "runs": 4,
  "failures": 4,
  "noops": 0,
  "runs_available": true,
  "model_support": {
    "status": "affected",
    "offending_models": ["claude-sonnet-4.6"]
  }
}
```

Affected repo (model unknown) — `offending_models` omitted:

```json
{ "repo": "…", "model_support": { "status": "affected" } }
```

Undetermined repo:

```json
{ "repo": "…", "model_support": { "status": "undetermined" } }
```

Clean repo — field absent entirely (byte-identical to today):

```json
{ "repo": "…", "drift_state": "aligned", "runs": 5, "failures": 0, "runs_available": true }
```

Rules:

- `model_support` present iff `status ∈ {affected, undetermined}`; omitted when clean.
- `offending_models` present iff non-empty; sorted + deduplicated.
- No existing field changes; `result.total` is unchanged.

## 5. Exit code — UNCHANGED

Gated on drift only: exit `1` iff `total.drifted + total.errored > 0`; otherwise `0`. A model-support failure on an otherwise drift-aligned fleet exits `0` (FR-007, SC-003).

## 6. Degradation

If the failure-issue read fails for a repo, that repo's `model_support.status` is `undetermined`; the rest of the dashboard renders and the command does not error solely because model detection failed (FR-011). A `gh variable list` failure for a flagged repo yields `status: affected` with `offending_models` omitted (unknown) — the flag is never dropped because the model could not be named (FR-002).
