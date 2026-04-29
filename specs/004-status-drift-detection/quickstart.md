# Quickstart: `gh-aw-fleet status`

**Feature**: `004-status-drift-detection` | **Date**: 2026-04-28

This is the operator-facing quickstart. It assumes you already have `gh-aw-fleet` installed, `gh auth status` clean, and a `fleet.json` (and optionally `fleet.local.json`) in the working directory.

---

## What `status` does

`status` answers one question: **does the actual workflow set in each declared repo match what fleet.json says it should be?** It computes drift in three categories:

| Category | Meaning |
|---|---|
| **Missing** | Workflow declared in fleet.json but not present in the repo. |
| **Extra** | gh-aw-managed workflow present in the repo but not declared in fleet.json. |
| **Drifted** | Workflow present and declared, but pinned to a different ref than fleet.json says. |
| **Unpinned** | Workflow present and declared, but its `.md` lacks a parseable `source:` frontmatter line (deploy bug or hand-edit). |

Each repo rolls up to one of three states: `aligned`, `drifted`, or `errored`.

`status` is **read-only** — it never clones, never opens a PR, never writes anywhere. It only reads from GitHub via `gh api`.

---

## Five-minute walkthrough

### 1. Quick fleet-wide check

```bash
gh-aw-fleet status
```

Reports per-repo drift in a tabwriter table. Exits `0` if every repo is aligned, `1` otherwise. Typical run for a 10-repo fleet finishes in under 20 seconds.

Sample output (mixed-state fleet):

```text
REPO                           STATE     MISSING  EXTRA  DRIFTED  UNPINNED
rshade/cronai                  aligned   0        0      0        0
rshade/gh-aw-fleet             drifted   0        0      1        0
rshade/private-thing           errored   -        -      -        -

rshade/gh-aw-fleet drift detail:
  drifted: audit          v0.68.3 → v0.67.0

rshade/private-thing error:
  HTTP 404: repository not found or inaccessible
```

### 2. Drill in on one repo

```bash
gh-aw-fleet status rshade/gh-aw-fleet
```

Same drift report, scoped to one repo. Useful when CI flagged a single repo and you want a focused view.

### 3. CI gate before deploy

```bash
gh-aw-fleet status && gh-aw-fleet deploy --apply rshade/gh-aw-fleet
```

If status exits `0`, the chain proceeds. If anything is drifted or errored, the chain aborts before any clones or PRs are created. You investigate, fix `fleet.json` or the repo state, then re-run.

### 4. Machine-readable output for dashboards

```bash
gh-aw-fleet status -o json | jq '.result.repos | map(select(.drift_state == "drifted")) | length'
```

Returns just the count of drifted repos — perfect for a Grafana panel or a Slack alert. The full JSON envelope is documented in `contracts/json-envelope.md`.

Other useful jq queries:

```bash
# Names of every drifted repo
gh-aw-fleet status -o json | jq -r '.result.repos | map(select(.drift_state == "drifted")) | .[].repo'

# All workflow-level drifts across the fleet
gh-aw-fleet status -o json | jq -r '.result.repos[] | .repo as $r | .drifted[] | "\($r) \(.name): \(.desired_ref) → \(.actual_ref)"'

# Repos that returned errors (e.g., access issues)
gh-aw-fleet status -o json | jq -r '.result.repos | map(select(.drift_state == "errored")) | .[].repo'
```

### 5. Debug the run itself

```bash
gh-aw-fleet status --log-level debug --log-format console
```

Routes the underlying `gh api` calls and worker-pool dispatch to stderr at debug level. Useful when status is slow, errored unexpectedly, or you want to confirm "no clones happened" by seeing only `gh api` calls in the log.

---

## What status does NOT do

Per spec scope:

- **No mutation.** No `gh aw add`, no `gh aw upgrade`, no PRs, no commits, no branches, no clones. If you need to fix drift, run `gh-aw-fleet sync --apply` or `gh-aw-fleet deploy --apply` after status confirms.
- **No content comparison.** Status only checks the `source:` ref on each workflow's `.md` file. If two workflows have the same `source:` ref but different content (impossible under normal `gh aw` flow, but theoretically possible via manual edit), status reports them as aligned.
- **No template-catalog comparison.** Whether a workflow in the catalog (`templates.json`) is one you should deploy is a separate question handled by the template-eval flow.
- **No non-default branch checks.** Status reads each repo's default branch only. The follow-up issue **#61** tracks demand for an opt-in `--ref <branch>` flag.
- **No SHA resolution.** A workflow pinned to a SHA that points to the same content as the desired tag is reported as `drifted` (strict string comparison). The follow-up issue **#62** tracks demand for an opt-in resolve mode.

---

## Troubleshooting

### "repo not declared in fleet config"

You passed `gh-aw-fleet status owner/name` but `owner/name` isn't in your `fleet.json` or `fleet.local.json`. Check the loaded config breadcrumb on stderr to confirm which file was loaded; add the repo via `gh-aw-fleet add owner/name` if needed.

### A repo shows `errored: HTTP 404` but the repo exists

Your `gh` token doesn't have access to it. Check `gh auth status` and confirm your token's scopes include `repo` (for private repos). Public repos work with no extra scope.

### A repo shows `errored: API rate limit exceeded`

You've hit GitHub's per-token rate limit (5000 requests/hour authenticated). Wait for the reset (`gh api rate_limit | jq .resources.core`) or rotate to a different token. Status's bounded worker pool (6 concurrent repos) keeps the rate of API calls reasonable, but a very large fleet or rapid re-runs can still exhaust the budget.

### Status shows `drifted` but I just ran `deploy --apply`

Two likely causes:

1. **Pin mismatch**: `gh aw add` (which `deploy` uses) honors fleet.json pins; `gh aw update` (which `upgrade` uses) follows the workflow's own `source:` line. If you ran `upgrade` after editing fleet.json, the upgrade may have moved past the pin. Run `gh-aw-fleet sync --apply --force <repo>` to re-pin to fleet.json.

2. **Branch lag**: `deploy --apply` opens a PR; until it's merged, the default branch (which status reads) still shows the pre-merge state. Wait for the merge, then re-run status.

### Status shows `unpinned` for a workflow that I deployed normally

Open the workflow's `.md` in the repo on GitHub — the YAML frontmatter at the top should have a `source: ...@<ref>` line. If it's missing, something went wrong in the deploy (or someone hand-edited the file). Re-run `gh-aw-fleet deploy --apply --force <repo>` to overwrite it cleanly.

### Status takes longer than 20 seconds for 10 repos

Check `gh api rate_limit` — if you're rate-limited, retries dominate. If not, the bottleneck is per-`gh api` latency. Status's worker pool is 6; on a slow connection or against geographically distant GitHub endpoints, individual calls can dominate. Run with `--log-level debug` to see per-call timings. The follow-up issue **#62** discusses an opt-in mode that would reduce false-positive drift reports but at additional API cost — that's the wrong direction if you're already slow.

---

## When to use status vs sync vs deploy --dry-run

| Question | Best command |
|---|---|
| "Anything to do across the fleet right now?" | `gh-aw-fleet status` |
| "Is repo X aligned with fleet.json?" | `gh-aw-fleet status owner/name` |
| "What WOULD `deploy --apply` do for repo X?" (preview the changes) | `gh-aw-fleet deploy --dry-run owner/name` (clones, runs `gh aw add` against scratch dir) |
| "Reconcile repo X to fleet.json now" | `gh-aw-fleet sync --apply owner/name` |
| "Bump installed workflows to their latest source-tag" | `gh-aw-fleet upgrade --apply owner/name` |
| "Audit pinned refs without acting" | `gh-aw-fleet upgrade --audit owner/name` (also clones) |

Use `status` when you don't want to clone, when you want fast feedback, and when "yes/no does any action need taking" is the question. Use the other commands when you need to actually preview or apply mutations.
