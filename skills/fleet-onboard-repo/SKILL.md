---
name: fleet-onboard-repo
description: Add a new repository to the gh-aw-fleet tracked set and walk it through its first deploy. Use this skill whenever the user says "add repo X to fleet", "track X with gh-aw-fleet", "onboard X", "start managing X", "register X in the fleet", "include X in our fleet", or provides a repo spec that isn't already in fleet.json / fleet.local.json and wants it managed. Also trigger if the user says "deploy to X" but X is not yet tracked — pivot to this skill before attempting deploy. The skill edits fleet.local.json to register the repo (never fleet.json, which is the public example), asks for profile selection and per-repo customization (engine override, exclusions, extras), verifies via `go run . list`, and then hands off to fleet-deploy for the first PR. Do not trigger for already-tracked repos (that's fleet-deploy), for fleet-wide audits (fleet-upgrade-review), or catalog evaluation (fleet-eval-templates).
---

# Fleet Onboard Repo

Register a new target repo in `fleet.local.json`, pick its profile(s), then hand off to `fleet-deploy` for the first PR.

## Why this skill exists

Adding a repo is a small but error-prone decision set: which profile(s) apply, which engine, any excludes, any extras. Mistakes here ripple — the repo gets workflows it didn't want, or misses workflows it did. The skill walks the decisions in order and emits a clean fleet.local.json diff the user can review before the actual deploy.

`fleet.local.json` is the private fleet state (gitignored). `fleet.json` is the committed public example and never edited by this skill. That separation is critical — the public file must not accumulate private repo names.

## When to use

- User names a repo not in `go run . list` output and wants it tracked.
- User says "deploy to X" but X isn't tracked — this skill runs first, then hands off to `fleet-deploy`.

Skip for:
- Already-tracked repos (they go straight to `fleet-deploy`).
- Catalog work — `fleet-eval-templates`.
- Audits — `fleet-upgrade-review`.

## The flow

### Step 1 — verify not already tracked

```bash
cd /mnt/c/GitHub/go/src/github.com/rshade/gh-aw && go run . list
```

If `<owner>/<repo>` already appears in the output, stop and say "already tracked — use `fleet-deploy` to (re)deploy." Do not edit fleet.local.json.

Also verify the repo exists and you can access it:

```bash
gh repo view <owner>/<repo> --json name,visibility,defaultBranchRef
```

If the fetch fails, report and stop.

### Step 2 — gather registration decisions

Ask the user for each of these. Propose defaults based on the repo's shape (language, activity level), but let the user override.

1. **Profile(s)**: one or more of `default`, `quality-plus`, `security-plus`, `docs-plus`, `community-plus`. Default pick: `["default"]`. Propose specialized profiles only when the repo fits (e.g., `docs-plus` if it has a public docs site).
2. **Engine**: optional override of `defaults.engine` (which is `copilot`). Only set if user has a specific reason (e.g., trialing `claude`). Propose: leave unset so the fleet default applies.
3. **Excludes**: workflows from the selected profile(s) to skip. Propose based on repo shape — e.g., exclude `daily-doc-updater` + `docs-noob-tester` on a repo with no public docs; exclude `dependabot-pr-bundler` if Dependabot isn't configured.
4. **Extras**: local workflows (source `local`) or ad-hoc additions not in any profile. Rare for a new repo; skip unless user volunteers.

Don't ask about deploy timing yet — that question makes more sense after the user has seen and approved the actual JSON diff (Step 3). Asking too early forces them to commit to a flow before they know what's being registered.

### Step 3 — dry-run `add` to preview the registration

Run the `add` subcommand in dry-run mode (default) to see what would be registered:

```bash
go run . add <owner>/<repo> --profile default
```

With exclusions or other customizations:

```bash
go run . add <owner>/<repo> \
    --profile default \
    --exclude daily-doc-updater --exclude docs-noob-tester
```

Dry-run prints (stderr) `would add <owner>/<repo> with profiles [default] (N workflows)` plus the workflow list on stdout. No file is written. Show the user the preview, then ask about the follow-up:

> "After registering, deploy immediately via `fleet-deploy`, or just register for now?"

### Step 4 — apply

```bash
go run . add <owner>/<repo> --profile default --apply --yes
```

The command writes `fleet.local.json` atomically and never touches `fleet.json`. If `fleet.local.json` didn't exist, it's synthesized as a minimal file (only `version` + the new repo entry); profiles and defaults continue to resolve from `fleet.json`.

### Step 5 — verify

```bash
go run . list
```

Confirm:
- The new repo appears in the output.
- Effective workflow count matches expectations (12 in default minus excludes, etc.).
- Engine shows the expected value.
- `(loaded fleet.local.json)` appears at the top (not `fleet.json` — proving our edit hit the right file).

If anything is off, delete the `fleet.local.json` entry (or the whole file if this was the first `add`) and restart.

### Step 6 — hand off to fleet-deploy

If the user said "deploy immediately":

- Invoke the `fleet-deploy` flow with the newly registered repo.
- Explicit note in the turn: "Repo registered. Running dry-run now."
- Then follow `fleet-deploy`'s three-turn pattern.

If the user said "register only":

- Report "Registered. Run `go run . deploy <owner>/<repo>` when ready."
- Stop.

## Invariants

- **Never edit `fleet.json`** in this skill. That file is the committed public example. If it accidentally contains private repo names, flag that as a bug — they shouldn't be there.
- **Never commit `fleet.local.json`**. It's in `.gitignore` by design. Don't suggest `git add fleet.local.json`.
- **Always verify via `go run . list`** after `--apply`. Syntactic errors would show up as parse errors — catching them before handoff saves a confused deploy run.
- **Never hand-edit `fleet.local.json`** in this flow. Use `go run . add ... --apply --yes`. The tool handles atomic writes and schema correctness; hand-editing risks stray commas or drift from the `RepoSpec` schema.
- **Don't suggest editing `profiles/` or `fleet.json` profile definitions** as part of onboarding. If a new repo needs a workflow that's not in any existing profile, that's a separate discussion (often involving `fleet-eval-templates` to check the upstream catalog first).
- **Scope the gh repo view check** to basic info only. Don't read the target repo's code; that's noise and potential privacy leak.

## Decision templates

Use these as starting points when proposing profile membership. They're not rules — adjust to what you know about the repo.

### Web app / service with public docs

```json
"profiles": ["default", "docs-plus"]
```

### Internal service / API (no public docs)

```json
"profiles": ["default"],
"exclude": ["daily-doc-updater", "docs-noob-tester"]
```

### Library / framework (has docs, community interaction)

```json
"profiles": ["default", "docs-plus", "community-plus"]
```

### Active development (test/perf-heavy)

```json
"profiles": ["default", "quality-plus"]
```

### Security-sensitive (auth, payments, PII)

```json
"profiles": ["default", "security-plus"]
```

### Low-activity (archive, maintenance-only)

```json
"profiles": ["default"],
"exclude": ["code-simplifier", "weekly-issue-summary", "issue-arborist", "sub-issue-closer"]
```

### What "success" looks like

- `fleet.local.json` has the new repo with the decided profile set.
- `go run . list` shows it with the expected workflow count.
- Either a successful `fleet-deploy` handoff (PR created) or a clean "registered, deploy later" handoff.

## Examples

### Example 1 — simple internal service

```
User: add HavenTrack/user-service to the fleet — it's a Go API server, no public docs

You:
  - Run `go run . list`. Confirm not present.
  - Run `gh repo view HavenTrack/user-service`. Repo exists, Go, internal.
  - Propose: profiles=["default"], exclude=["daily-doc-updater", "docs-noob-tester"], engine unset (inherits copilot).
  - Show diff.

User: looks good, deploy immediately

You:
  - Run `go run . add HavenTrack/user-service --profile default --exclude daily-doc-updater --exclude docs-noob-tester --apply --yes`.
  - Verify with `go run . list`.
  - Hand off to fleet-deploy flow. Dry-run three-turn pattern begins.
```

### Example 2 — library with docs

```
User: track rshade/opencommit

You:
  - Verify not tracked, verify exists.
  - Note: README present, has docs/, active community.
  - Propose: profiles=["default", "docs-plus", "community-plus"], no excludes, no engine override.
  - Show the user the workflow count it'll result in. Ask about deploy timing.

User: register only

You:
  - Run `go run . add rshade/opencommit --profile default --profile docs-plus --profile community-plus --apply --yes`.
  - Verify with `go run . list`. Report registered + next-step command.
```

### Example 3 — user said "deploy to X" but X isn't tracked

```
User: deploy to rshade/new-experiment

You:
  - Run `go run . list`. rshade/new-experiment not present.
  - Don't run `fleet-deploy`. Pivot: "That repo isn't tracked yet. Let's onboard it first. What profile(s) and any excludes?"
  - Proceed with fleet-onboard-repo flow.
  - After registration, chain into fleet-deploy.
```

## Extended usage

For machine-readable output, pass `-o json` on `list`/`deploy`/`sync`/`upgrade`. See `specs/003-cli-output-json/quickstart.md`.
