# Quickstart: `overview` Model-Support Advisory

Read-only. No `--apply`, no mutation, no three-turn flow.

## Run it

```bash
# Default trailing-7d window; model-support advisory appears for flagged repos.
go run . overview

# Scope to specific repos.
go run . overview rshade/finfocus rshade/hvmesh

# Machine-readable — inspect the additive model_support field.
go run . overview --output json | jq '.result.repos[] | {repo, model_support}'
```

## What you'll see

For a repo whose recent agentic runs failed on an unsupported model:

```text
REPO             DRIFT    RUNS  FAIL  NOOP  HEALTH  AIC  COST
rshade/finfocus  aligned  4     4     0     0%      -    -
...

rshade/finfocus (unhealthy):
  runs: 4 (4 failed, 0 no-op) · health 0% · cost -
  model-support: FAILING — offending model(s): claude-sonnet-4.6
```

On stderr:

```text
[hint] rshade/finfocus: agentic runs are failing on an unsupported/deprecated Copilot model.
       Set the GH_AW_DEFAULT_MODEL_COPILOT Actions variable on the repo to a model your
       Copilot subscription supports (no recompile required).
```

A fully healthy, drift-aligned fleet prints **exactly** what it prints today — no model line, no hint, no JSON field, exit `0`.

## Fix a flagged repo

The effective model resolves at runtime; no recompile needed:

```bash
gh variable set GH_AW_DEFAULT_MODEL_COPILOT --repo rshade/finfocus --body gpt-5.3-codex
```

Re-run `overview` after the open failure issue clears (gh-aw auto-expires it) to confirm the flag drops.

## Verify (maps to Success Criteria)

```bash
make ci   # fmt-check + vet + lint + full test suite (SC-006)
```

Offline fixture assertions (SC-002 — 100% classification, no false positives):

- **model-failure repo** → flagged, offending model named (or `unknown` if the variable read failed).
- **unrelated-failure repo** (only `agent_failure`) → **not** flagged (FR-009).
- **healthy repo** → no model line, no hint, `model_support` absent from JSON (SC-003/SC-004).
- **undetermined repo** (failure-issue read errors) → `model_support.status = "undetermined"` in JSON; text line only if a detail block already renders (FR-011).

JSON contract check (SC-004 — additive, schema unchanged):

```bash
go run . overview --output json | jq '.schema_version'          # still 1
go run . overview --output json | jq '.result.repos[] | select(.model_support == null) | keys'  # clean rows unchanged
```

## Manual signal check (optional, live)

Confirm the upstream signal a flagged repo is reading:

```bash
# The failure-issue marker overview matches on:
gh issue list --repo rshade/finfocus --label agentic-workflows --state open --json number,title,body \
  | jq -r '.[] | select(.body | contains("gh-aw-failure-issue: true")) | .body' \
  | grep -o 'failure_categories:[^-]*'

# The variables overview resolves the offending model from:
gh variable list --repo rshade/finfocus --json name,value \
  | jq '.[] | select(.name | test("COPILOT"))'
```
