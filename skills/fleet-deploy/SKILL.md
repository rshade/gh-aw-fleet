---
name: fleet-deploy
description: Guide the operator through deploying the fleet.json-declared workflow set to a tracked repository using the gh-aw-fleet CLI. Use this skill whenever the user says "deploy to <repo>", "run fleet deploy for X", "push fleet workflows to X", "sync workflows to X via fleet", "apply default profile to X", or asks to update agentic workflows on a specific tracked repo. Also trigger when the user names one of the tracked repos (in fleet.json or fleet.local.json) in the same breath as deploy/push/apply/ship verbs. The skill handles dry-run review, the --apply go-ahead gate, and gracefully recovers from gpg-signing failures by composing a manual-finish shell paste rather than bypassing commit signing. Do not trigger for template catalog refreshes (that's fleet-eval-templates), onboarding a brand-new repo (fleet-onboard-repo), bumping pin refs (fleet-upgrade-review), or editing fleet.json structure.
---

# Fleet Deploy

Deploy the declared workflow set from `fleet.json` (or `fleet.local.json`) to a target repo via `gh aw add`, then open a PR against the target.

## Why this skill exists

Deploying workflows to a repo is a three-turn conversation: dry-run → review → apply. The turns exist for safety — `--apply` pushes a branch and opens a PR visible to anyone with access to the target repo. The skill codifies the turns so you don't have to re-derive them each time. It also encodes the gpg-signing failure path, which is the most common way `--apply` fails in practice (the Go tool runs git non-interactively; a signing-required git config can't prompt for a passphrase).

## When to use

Use when the user names a tracked repo and wants to deploy / sync / push / ship / apply workflows to it. Tracked repos live in `fleet.json` or `fleet.local.json`; use `go run . list` to confirm.

Skip for:
- Bulk operations across every tracked repo — no automation yet; do one at a time.
- First-time onboarding of a repo not in fleet.json — see `fleet-onboard-repo`.
- Bumping workflow refs to latest — see `fleet-upgrade-review`.
- Refreshing the upstream template catalog — see `fleet-eval-templates`.

## The flow

Three turns. Don't collapse them — the confirmation gates are deliberate.

### Turn 1 — dry-run and report

Verify the repo is tracked:

```bash
cd /mnt/c/GitHub/go/src/github.com/rshade/gh-aw && go run . list
```

Look for an exact match on `<owner>/<repo>` in the output. If not present, stop and tell the user "That repo isn't in fleet.json / fleet.local.json — add it via `fleet add` or edit fleet.local.json, then re-run." Do not proceed.

Then dry-run:

```bash
go run . deploy <owner>/<repo>
```

The output includes (as applicable):

- `gh aw init: ran (repo was not yet initialized)` — the target didn't have the dispatcher agent; init ran in the scratch clone.
- `added: N` — workflows that would be added, each with full `owner/repo/.../workflow@ref` spec.
- `skipped: N` — already-present workflows.
- `failed: N` — each with a line or two of error text, followed by `hint: ...` lines surfaced by `fleet.CollectHints`.

Report all of this back to the user faithfully — don't summarize away the failures, don't drop the hints. If there are failures:

1. Show each failed workflow's exact error text (it came from `gh aw add` via the tool's `condense()`).
2. Show each applicable hint verbatim — the tool's diagnostic layer already knows the remediation (e.g., "pin the source to a tagged release via `fleet sync --apply --force`").
3. Ask how they want to proceed. Typical options: (a) fix the pin in fleet.json + re-dry-run, (b) proceed with `--apply` accepting the failures as deferred, (c) run `fleet sync --apply --force <repo>` first to re-pin, (d) abort.

### Turn 2 — wait for explicit go-ahead

Do not run `--apply` without the user saying "go", "apply", "yes", "proceed", "trust me", or some equivalent clear affirmation. This is the blast-radius gate.

If the user edits fleet.json between turns (for a pin change or new exclude), rerun Turn 1 before Turn 2. The latest dry-run is the source of truth for what the user is approving.

### Turn 3 — apply

```bash
go run . deploy <owner>/<repo> --apply
```

Expected outcomes:

- **Clean success**: output ends with `PR: https://github.com/...`. Report the URL to the user; declare the deploy complete.
- **gpg signing failure**: exit status 1, output contains `gpg failed to sign the data` or `gpg: signing failed`. The tool preserves the clone dir (`/tmp/gh-aw-fleet-<owner>-<repo>-<timestamp>`) with all files staged on the deploy branch. Compose a manual-finish paste (next section).
- **Other failures**: report honestly — don't auto-retry.

## The gpg-failure manual-finish paste

When `--apply` fails on gpg signing, the deploy has already done the hard work: cloned the target, run `gh aw init` (if needed), run `gh aw add` for each workflow, created the deploy branch, staged all changes. Only the commit is blocked, because the git signing operation couldn't prompt for a passphrase.

The fix is to hand the user a copy-paste block they run in their own shell, where gpg-agent can prompt them interactively. **Never** add `--no-gpg-sign` or `-c commit.gpgsign=false` to any git invocation — CLAUDE.md forbids it, and the rule exists because signing bypasses can mask real security incidents.

### How to compose the paste

Gather the values from the failed `--apply` output:

1. Clone path: `ls -td /tmp/gh-aw-fleet-<owner>-<repo>-* | head -1`.
2. Branch name: read from the output (`fleet/deploy-<YYYY-MM-DD-HHMMSS>`), or run `git branch --show-current` inside the clone.
3. Workflow count: count the `+ <name>` lines under `added:` in the Turn 3 output.
4. Workflow names + specs: same place.
5. Skipped names: from `skipped:` in the output (if any).
6. Init status: check the `gh aw init: ran` line (present or absent).

### The template

```bash
CLONE=/tmp/gh-aw-fleet-<owner>-<repo>-<timestamp>
cd "$CLONE"

git commit -m 'ci(workflows): add <N> agentic workflows via gh-aw-fleet

Deployed via gh-aw-fleet:

- <workflow-1>
- <workflow-2>
- ...'

git push -u origin <branch>

gh pr create \
  --title "ci(workflows): add <N> agentic workflows via gh-aw-fleet" \
  --body "$(cat <<'EOF'
Deploys <N> agentic workflows to \`<owner>/<repo>\` via [gh-aw-fleet](https://github.com/rshade/gh-aw-fleet).

<(optional) This repo was not yet initialized for gh-aw; \`gh aw init\` was run as part of this PR.>

## Added

- \`<spec-1>\`
- \`<spec-2>\`
- ...

<(optional) ## Already present (skipped)

- \`<skipped-1>\`
- ...>

Each workflow is pinned via its frontmatter \`source:\` field. Use \`gh aw update\` to pull upstream changes.
EOF
)"
```

Substitute every `<angle-bracketed>` placeholder with the concrete value. Omit the "not yet initialized" line if the init didn't run. Omit the "Already present" section if nothing was skipped.

The commit subject and PR title must be **identical** and in Conventional Commits format:

```
ci(workflows): add <N> agentic workflows via gh-aw-fleet
```

No scope abbreviations. No trailing period. Under 72 chars (this template is 54 chars + `<N>` digits). `ci` is the type (GH Actions workflows live in `.github/workflows/`); `workflows` is the scope.

### After handing the paste

Stop. Wait for the user to report the PR URL or the merge. Don't try to run git commands yourself — the user's shell is where gpg-agent lives.

## Invariants (non-negotiable)

These apply regardless of how the user phrases the request:

- **Never** run `git add`, `git commit`, or `git push` from Claude Code's Bash tool. The Go tool runs these internally via `exec.Command` from a subprocess context the user initiated — that's fine. You, the assistant, don't invoke git directly. (`.claude/settings.json` denies these.)
- **Never** bypass commit signing. `--no-gpg-sign`, `-c commit.gpgsign=false`, `git config commit.gpgsign false` are off-limits. CLAUDE.md is explicit, and the rule exists for auditability.
- **Dry-run is the default**. `--apply` always requires an explicit user turn saying to proceed.
- **Pre-flight failures are data, not errors to hide**. Every failed workflow gets its error text + hint surfaced to the user — never summarize them away. The `hint:` line from `fleet.CollectHints` is the actionable half; it tells the user whether the fix is a CLI upgrade, a pin change, or a source swap.
- **Commit messages are commitlint-valid**. Conventional Commits, `ci(workflows)` scope, subject under 72 chars, no trailing period.
- **Clone dirs on `/tmp` are the user's breadcrumb after a failure** — don't clean them up. Once the user reports the PR merged, they can `rm -rf` themselves.

## Output format — what "success" looks like

Your final message for a completed deploy should include:

- The PR URL (from `--apply` output or from the user confirming they ran the paste).
- One line confirming conventional-commits title is used.
- Clone-dir status: "cleaned up by tool" (clean success) or "preserved at `/tmp/gh-aw-fleet-...` — remove when convenient" (gpg-interrupted path).
- If there were pre-flight failures: list the deferred workflows with a one-line-each suggestion for the next step.

## Examples

### Example 1 — clean deploy, no failures

```
User: deploy to rshade/finfocus

Turn 1 (you):
  - Run `go run . list`. Confirm rshade/finfocus is tracked.
  - Run `go run . deploy rshade/finfocus`.
  - Report: "9 workflows to add, 0 pre-existing, gh aw init will run. No pre-flight failures. Ready to --apply?"

Turn 2 (user): go

Turn 3 (you):
  - Run `go run . deploy rshade/finfocus --apply`.
  - Output ends with `PR: https://github.com/rshade/finfocus/pull/42`.
  - Report PR URL. Done.
```

### Example 2 — gpg-signing failure

```
User: deploy to HavenTrack/goa-service-shared

Turns 1-2 as above.

Turn 3 (you):
  - Run `go run . deploy HavenTrack/goa-service-shared --apply`.
  - Output ends with "gpg failed to sign the data: ..."
  - `ls -td /tmp/gh-aw-fleet-HavenTrack-goa-service-shared-* | head -1` → /tmp/gh-aw-fleet-HavenTrack-goa-service-shared-1234567890
  - Read branch: `fleet/deploy-2026-04-18-130136`
  - Compose the manual-finish block with 8 workflows, PR title "ci(workflows): add 8 agentic workflows via gh-aw-fleet", body including Added section + Skipped section + init-was-run note.
  - Hand to user. Wait for PR URL.

User: merged: https://github.com/HavenTrack/goa-service-shared/pull/330

You: Mark deploy complete. Clone dir at /tmp/gh-aw-fleet-... can be removed when convenient.
```

### Example 3 — pre-flight failure (partial compile)

```
User: deploy to hvmesh/hvmesh

Turn 1 dry-run output includes:
  added:   5
  failed:  4
    ! ci-doctor
      .github/workflows/ci-doctor.md:37:3: error: Unknown property: mount-as-clis. Valid fields are: ...
    ! code-simplifier
      ...
    ! docs-noob-tester
      ...
    ! mergefest
      ...
  hint: Workflow uses `mount-as-clis`, an unreleased gh-aw feature. `gh extension upgrade gh-aw` if your CLI is out of date; if already latest, the upstream is ahead of the release — pin the source to a tagged release (e.g. `@v0.68.3`) via `fleet sync --apply --force`.

You:
  - Show all four failures + the hint verbatim.
  - Ask: "Four workflows would fail — gh-aw upstream is ahead of the CLI release. Options:
    (a) run `fleet sync --apply --force hvmesh/hvmesh` first to re-pin everything to fleet.json tagged refs
    (b) proceed to --apply with these 4 deferred
    (c) abort and bump the fleet.json pin
    Which do you want?"
  - DO NOT proceed without input.
```
