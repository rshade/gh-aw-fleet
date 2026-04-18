---
name: fleet-upgrade-review
description: Audit pin health and upgrade readiness across the gh-aw-fleet tracked repositories, then guide targeted upgrades. Use this skill whenever the user says "audit the fleet", "which repos need upgrading", "check pin health", "run fleet upgrade audit", "upgrade all repos", "what's out of date", "run gh aw upgrade across the fleet", or asks about the health of deployed agentic workflows across multiple repos. Also trigger on phrases like "is <repo> up to date" or "any repos behind on their pins". The skill runs `fleet upgrade --audit --all`, presents a per-repo dashboard, and handles individual repo upgrades one at a time with dry-run review and gpg-failure handling. Do not trigger for deploying new workflows (fleet-deploy), for refreshing the upstream catalog (fleet-eval-templates), or for onboarding a new repo (fleet-onboard-repo).
---

# Fleet Upgrade Review

Audit pin health and action-versions across the fleet. Guide targeted upgrades when something needs attention.

## Why this skill exists

`gh-aw-fleet` tracks N repos, each pinning workflows to specific refs. Over time, upstream tags bump, actions get new versions, and the installed `gh aw` CLI moves. Drift accumulates silently until a deploy or update surfaces it. This skill is the periodic sweep: "of my fleet, what needs attention?" It wraps `gh aw upgrade --audit --json` which gh-aw provides for exactly this purpose.

The skill also handles the common aftermath: when a repo does need upgrading, it's the same three-turn pattern as `fleet-deploy` — dry-run, review, apply, handle gpg. Rather than duplicate, this skill defers to `fleet-deploy`-style conventions for the apply phase.

## When to use

- User wants a cross-repo health check.
- User suspects a specific repo is behind.
- Before a major coordinated version bump (e.g., gh-aw tagging a new release, agentics shipping breaking changes).

Skip for:
- Adding a fresh workflow not yet in the profile — `fleet-deploy`.
- Source-swapping (e.g., gh-aw → agentics) for existing workflows — see `fleet-sync-force` flow (no dedicated skill yet; run `fleet sync --apply --force <repo>` with standard dry-run-then-apply pattern).
- Refreshing the upstream catalog — `fleet-eval-templates`.

## The flow

### Step 1 — fleet-wide audit

```bash
cd /mnt/c/GitHub/go/src/github.com/rshade/gh-aw && go run . upgrade --audit --all
```

This clones each tracked repo, runs `gh aw upgrade --audit --json` inside, collects the JSON. Output is a compact per-repo summary.

**Fallback**: if the audit can't run — whether from subprocess timeout (>5 min), denied Bash in a restricted subagent environment, or network failure — fall back to reading recent state from conversation context or `fleet.local.json` directly. A partial dashboard with a "source: cached/fallback" note is more useful than no dashboard. Note the fallback reason explicitly so the user can address it (authenticate gh, run the audit locally themselves, etc.).

For each repo, the audit reports (via the underlying `gh aw upgrade --audit`):

- Outdated Go dependencies with available updates.
- Security advisories from GitHub Security Advisory API.
- Dependency maturity (v0.x vs stable).
- A dependency-health score.

Aggregate into a dashboard view. Table columns: repo, pin status (OK / behind N tags / bleeding-edge SHA), action-version drift, any security advisories, recommended action.

### Step 2 — present the dashboard

A good dashboard is compact and actionable:

```
Fleet audit (2026-04-18):

| Repo                           | Pins          | Actions       | Security | Action needed            |
|--------------------------------|---------------|---------------|----------|--------------------------|
| acme/widgets                  | OK (v0.68.3)  | OK            | 0        | none                     |
| HavenTrack/goa-service-shared  | OK (v0.68.3)  | 2 bumps avail | 0        | consider upgrade         |
| rshade/finfocus                | OK (v0.68.3)  | OK            | 1        | review advisory          |
| rshade/gh-aw-fleet             | N/A (example) | N/A           | 0        | skip                     |
```

Flag urgencies:
- **Security advisories** — highest priority, call out specifically.
- **Major version behind** — call out with "will require `--major` flag".
- **Action bumps** — routine; batch via `fleet upgrade --all --apply` when convenient.
- **Bleeding-edge SHA pins** (workflow's frontmatter `source:` is a SHA, not a tag) — surface the `mount-as-clis`-class risk; recommend `fleet sync --apply --force <repo>` to re-pin to tags.

If everything is OK across the fleet, say so and stop.

### Step 3 — per-repo dry-run

If any repo needs attention, ask the user which to address first (or all, one at a time). For each:

```bash
go run . upgrade <owner>/<repo>
```

Dry-run output shows:
- `gh aw upgrade` log (dispatcher updates, action bumps).
- `gh aw update` log (per-workflow pulls, 3-way merges, compile results).
- Summary: `changed: N file(s)`, each listed with `~`.
- `hint: ...` lines if any compile failures happened.
- `CONFLICTS: ...` if the 3-way merge produced unresolved conflicts.

Report everything to the user. Pay particular attention to:
- **Compile failures** with hints — typical cause is upstream workflow using features the installed CLI doesn't know about. The hint usually says "pin to tagged release via `fleet sync --apply --force`."
- **Conflicts** — files where local modifications collide with upstream changes. Do NOT proceed to `--apply`. Surface the conflict list, recommend opening the clone dir (preserved at `/tmp/gh-aw-fleet-...`) to resolve manually.
- **No changes** — report "no-op, skip."

### Step 4 — wait for explicit go-ahead before --apply

Same blast-radius rule as `fleet-deploy`: `--apply` pushes a branch and opens a PR on the target. Don't run it without "go" / "apply" / "proceed" from the user. Re-dry-run if the user edits fleet.json between turns.

### Step 5 — apply

```bash
go run . upgrade <owner>/<repo> --apply
```

Outcomes match `fleet-deploy`:
- **Clean success**: output ends with `PR: <url>`. Report the URL.
- **gpg signing failure**: same failure mode; generate the manual-finish paste.
- **Conflict (missed in dry-run)**: report, don't retry.

### The gpg-failure manual-finish paste (upgrade variant)

Same mechanics as `fleet-deploy`, with a different subject/body template:

```bash
CLONE=/tmp/gh-aw-fleet-<owner>-<repo>-<timestamp>
cd "$CLONE"

git commit -m 'ci(workflows): upgrade agentic workflows (<N> files changed)

Upgraded via gh aw upgrade + update.'

git push -u origin <branch>

gh pr create \
  --title "ci(workflows): upgrade agentic workflows" \
  --body "$(cat <<'EOF'
Upgrades agentic workflows via \`gh aw upgrade\` + \`gh aw update\` through [gh-aw-fleet](https://github.com/rshade/gh-aw-fleet).

## Changed files

- \`<file-1>\`
- \`<file-2>\`
- ...

<(optional) ## Dispatcher / actions updates

- \`actions/github-script\` → vX
- \`github/gh-aw-actions/setup\` → vY>
EOF
)"
```

Commit subject + PR title identical: `ci(workflows): upgrade agentic workflows (<N> files changed)` / `ci(workflows): upgrade agentic workflows`. Under 72 chars. (The PR title omits the count since it may change if the user edits before merge.)

## Invariants

Same as `fleet-deploy`, repeated here because the skill will be invoked independently:

- **Never** run `git add`, `git commit`, `git push` from Claude Code's Bash tool.
- **Never** bypass commit signing.
- **Dry-run is the default**. Explicit user go-ahead before `--apply`.
- **Conflicts block the pipeline** — do not proceed to `--apply` when `CONFLICTS:` appears. User resolves in the preserved clone, then re-runs `fleet upgrade <repo> --apply --work-dir <clone>` (the resume path is supported).
- **Commit messages are commitlint-valid**. `ci(workflows)` scope.
- **`--major`** is opt-in. Only pass it when the user explicitly requests major-version bumps (the dashboard flagged something requiring it).
- **Clone dirs are breadcrumbs.** Preserve on failure; user cleans up after merge.

## What "success" looks like

- Fleet dashboard presented, either "all OK" or specific repos flagged with reasons.
- Any repos needing upgrades have been walked through individually.
- Each successful upgrade has a PR URL reported.
- Any conflicts have been surfaced with preserved clone paths.

## Examples

### Example 1 — clean fleet

```
User: audit the fleet

You:
  - Run `go run . upgrade --audit --all`.
  - Output: all three repos show "OK".
  - Present dashboard.
  - Report: "All three tracked repos are current. No action needed."
```

### Example 2 — repo behind on actions

```
User: which repos need upgrading?

You:
  - Run audit. goa-service-shared shows 2 action bumps available.
  - Present dashboard. Ask: "goa-service-shared has 2 action bumps. Run dry-run?"

User: yes

You:
  - Run `go run . upgrade HavenTrack/goa-service-shared`.
  - Report: 1 file changed (actions-lock.json), no compile failures, no conflicts.
  - Ask for --apply go-ahead.

User: go

You:
  - Run `--apply`. PR created.
  - Report PR URL.
```

### Example 3 — mount-as-clis drift

```
User: check pin health

You:
  - Run audit. acme flags: "4 workflows use SHA pins to gh-aw main — risk of compile break on update".
  - Dry-run acme to confirm: 4 compile failures with mount-as-clis hint.
  - Report: "acme has 4 workflows pinned to gh-aw main (bleeding edge). Upgrade would fail on compile. The tool's hint recommends `fleet sync --apply --force acme/widgets` to re-pin to tagged v0.68.3. Want to run that instead?"
  - Wait. Do NOT proceed with --apply on upgrade.
```

### Example 4 — conflict during update

```
User: upgrade HavenTrack/goa-service-shared

You:
  - Dry-run. Output includes `CONFLICTS: 2 file(s) need manual merge` + the filenames.
  - Report conflicts. Show the clone path.
  - Say: "Resolve in the clone, then re-run `go run . upgrade HavenTrack/goa-service-shared --apply --work-dir /tmp/gh-aw-fleet-...`. I'll wait."
  - Do NOT attempt --apply.
```
