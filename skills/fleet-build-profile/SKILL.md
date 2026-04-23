---
name: fleet-build-profile
description: Scaffold a new fleet profile — add a `profiles.<name>` entry to fleet.json (or fleet.local.json) with a curated workflow list and pinned source refs, and optionally mirror it to `profiles/<name>.json`. Use this skill whenever the user says "build a profile", "create a profile", "scaffold a profile", "new profile", "add a profile to fleet.json", "make a profiles/<name>.json", "start a docs-plus / security-plus / quality-plus / community-plus profile", or proposes a named profile that doesn't exist yet. Also trigger when `fleet-eval-templates` has surfaced candidates the user now wants materialized into a profile definition. The skill reads `templates.json` for workflow metadata, proposes the profile JSON inline, writes on approval, and for the `default` profile mirrors both files to preserve the documented verbatim-match invariant. Do not trigger for evaluating upstream workflows (that's `fleet-eval-templates`), for deploying an existing profile to a repo (that's `fleet-deploy`), for rotating pins on already-deployed workflows (that's `fleet-upgrade-review`), or for onboarding a repo into the fleet (that's `fleet-onboard-repo`).
---

# Fleet Build Profile

Scaffold a new profile (or materialize candidates from `fleet-eval-templates`) as a `profiles.<name>` entry in `fleet.json` or `fleet.local.json`, with pinned sources and a curated workflow list.

## Why this skill exists

Profiles are the *intent layer* of the fleet. `templates.json` says what exists upstream; `fleet.json` says which curated subset a repo gets, pinned to which ref. The gap between them is judgment — "these eight agentics workflows make a coherent `security-plus` bundle at this ref, tagged so audits don't drift."

`fleet-eval-templates` answers "which workflows deserve inclusion?" and stops at a proposed diff. That stop-point is deliberate: the decision to name a profile, choose pins, and write the file is a separate operator act. This skill is that act. Keeping the two skills split means evaluation stays read-only and profile-creation stays explicit — neither accidentally does the other.

Two other facts shape the flow:

- `fleet.json` is the public, committed declarative state (five profiles today: `default`, `quality-plus`, `security-plus`, `docs-plus`, `community-plus`). `fleet.local.json` layers on top, is gitignored, and is where private / experimental profiles live. The user picks which to write — usually `fleet.local.json` for new or one-off profiles, `fleet.json` only when the profile is stable and meant to be shared.
- `profiles/<name>.json` is a separate, human-readable mirror. Only `profiles/default.json` exists today, and no Go code reads it — it's a documentation convention. CLAUDE.md still requires `profiles/default.json` to match `fleet.json`'s `default` profile verbatim, so any write that touches the `default` profile has to update both files in the same turn.

## When to use

- User has a name in mind for a new profile and a rough workflow list (often the output of `fleet-eval-templates`).
- User wants to fork an existing profile into a specialized variant (e.g., `security-plus-strict` from `security-plus`).
- User is editing `profiles/default.json` as documentation and needs fleet.json kept in sync.

Skip for:
- Evaluating which upstream workflows deserve inclusion — `fleet-eval-templates`.
- Adding workflows to an already-existing profile without changing pins or scope (a small edit — just do it directly, no skill needed).
- Deploying a profile to a repo — `fleet-deploy`.
- Rotating pins on workflows already deployed across the fleet — `fleet-upgrade-review`.
- Onboarding a repo — `fleet-onboard-repo`.

## The flow

### Step 1 — confirm scope

Before touching anything, confirm four things with the user:

1. **Profile name.** e.g., `docs-plus`, `security-plus-strict`, `experimental`. If the name already exists in the target file, this is an edit, not a build — stop and ask whether to merge workflows or rename.
2. **Target file.** `fleet.local.json` (private, experimental, gitignored) or `fleet.json` (shared, committed). Default to `fleet.local.json` unless the user explicitly wants a shared profile.
3. **Mirror into `profiles/<name>.json`?** Only needed if this is the `default` profile (required, per invariant) or if the user wants a standalone example file for humans. Say "no" by default for new profiles — the file has no code consumer.
4. **Workflow source** — either inline list from the user, or "use the candidates from the last `fleet-eval-templates` run." In the second case, re-read `templates.json` to pull metadata; don't rely on memory.

If any of these is underspecified, ask. A mis-named or mis-placed profile is annoying to untangle because `fleet sync --apply --force` would have to re-pin deployed repos.

### Step 2 — pick sources and pins

A profile has a `sources` map: for each upstream repo it draws from, a `ref`. Decide pins by these rules:

- **`githubnext/agentics`**: pin to a commit SHA (preferred) or `main`. `main` is acceptable for agentics — it's a curated library and its tip is generally safer than gh-aw's. Copy the ref from `profiles/default.json` / `fleet.json`'s `default` profile unless the user has a reason to diverge (e.g., a known-good earlier SHA, or pinning to `main` to track the library).
- **`github/gh-aw`**: pin to a tag (`vX.Y.Z`), **never `main`**. CLAUDE.md is explicit: gh-aw's `main` often contains unreleased compiler features (e.g., `mount-as-clis`) that break the installed CLI. If the user says `main`, push back once and propose the latest tag instead.
- **Omit sources this profile doesn't use.** A `docs-plus` profile that draws only from agentics shouldn't carry a `github/gh-aw` pin. Extra pins don't hurt deploys but pollute the diff surface on pin bumps.

Read `templates.json`'s `fetched_at` for context on how old the catalog is — if it's stale (>a week), mention it and suggest running `fleet-eval-templates` first.

### Step 3 — curate the workflow list

For each candidate workflow, verify against `templates.json` before adding:

```bash
jq '.sources["githubnext/agentics"].workflows[] | select(.name == "<name>") | {description, triggers, safe_outputs}' templates.json
```

- If the workflow isn't in `templates.json`, stop — either the name is wrong or the catalog is stale. Don't guess.
- If `triggers` or `safe_outputs` are empty/null, read `.body` before including — the fetch may not have resolved imports, and the workflow's real shape is in the body.
- Cross-check the source: gh-aw workflows need `.github/workflows/` layout, agentics workflows use `workflows/`. `ResolvedWorkflow.Spec()` handles this, but the `source` field in the profile entry must match the `SourceLayout` key exactly (`github/gh-aw` or `githubnext/agentics`).

Name-order inside `workflows: [...]` is conventional: gh-aw entries first (dogfooded items with no agentics counterpart), then agentics entries alphabetical. Match the formatting of `profiles/default.json` — aligned-column `"source":` values are hand-maintained, and diffs stay readable.

### Step 4 — propose the diff inline

Show the full JSON block for the new profile entry before writing. Include a short description field (one sentence — see existing profile descriptions for tone: "Baseline every tracked repo gets. Low-noise, broadly useful."). If `mirror to profiles/<name>.json` was requested, show both blocks.

Call out anything the user should sanity-check:
- Any gh-aw workflow included → flag the source stability note.
- Pin ref differs from `default` profile → mention it and why.
- Any workflow with `safe_outputs: create-pull-request` or `push-to-pull-request-branch` → note the noise profile, since the whole bundle's character is set by its heaviest safe-outputs.

Stop. Wait for explicit approval ("go", "apply", "looks good") before writing.

### Step 5 — write the files

On approval:

- Insert the new profile into the target file's `profiles` map, using a JSON editor or `jq` in-place. Preserve existing formatting; `fleet.json`'s aligned columns are the convention.
- If `name == "default"` **or** the user requested a mirror, write `profiles/<name>.json` with the same shape (top-level `description`, `sources`, `workflows` — no wrapping `version`/`defaults`/`profiles` map).
- For `name == "default"`, both files **must** be written in the same turn — the invariant is docs-only but trips code review and `fleet-eval-templates` follow-ups if out of sync.

Do not run `go run . ...` or `gh aw ...` as part of this step. The skill is declarative-state-only; deployment is the user's follow-up.

### Step 6 — remind about sync

Profile edits don't propagate automatically. Existing `gh aw update` on a deployed repo follows the *workflow's own frontmatter `source:` line*, not fleet.json — so a pin bump or workflow addition in a profile is invisible to already-deployed repos until the user runs:

```bash
go run . sync --apply --force <repo>
```

Call this out in the final summary. For a brand-new profile, there's nothing to sync until the user deploys it via `fleet-deploy` (`go run . deploy --apply --profile <name> <repo>`). Mention that path too.

## Invariants

- **Never default `github/gh-aw` to `main`.** If the user insists, push back once with the `mount-as-clis` example; if they still want it, proceed but log the risk in the profile description.
- **Do not delete or reorder existing profiles.** This skill appends. If the user wants a rename or deletion, that's a separate edit and should be explicit.
- **`default` profile edits require the mirror.** `fleet.json`'s `default` and `profiles/default.json` must match verbatim, including description, source order, workflow order, and column alignment. If they're already out of sync on entry, report it and fix alignment *first* before adding.
- **Don't write to `fleet.local.json` if it doesn't exist** — it's gitignored and absence is meaningful (the user hasn't set up a private fleet yet). Offer to create it, don't silently do so.
- **No git operations.** This skill edits declarative state only. Committing is the user's call, and CLAUDE.md forbids the Bash tool from running `git add`/`commit`/`push`.

## What "success" looks like

A concise, actionable summary:

```
Profile scaffolded: <name> → <target-file>[, profiles/<name>.json]

Sources:
  - <source>@<ref>   (inherited from default | set by user | bumped from X to Y)

Workflows (N):
  - <name> (<source>)
  ...

Next steps:
  - To deploy to a repo:       go run . deploy --apply --profile <name> <repo>
  - To re-pin deployed repos:  go run . sync --apply --force <repo>
  - Review diff before commit: git diff fleet.json profiles/<name>.json
```

If the user asked for a mirror or touched `default`, explicitly confirm both files were written.

## Examples

### Example 1 — new experimental profile in fleet.local.json

```
User: scaffold a profiles/experimental.json with link-checker and markdown-linter from agentics

You:
  - Confirm scope: name=experimental, target=fleet.local.json (gitignored for experiments),
    mirror to profiles/experimental.json? Ask — user says yes.
  - jq templates.json for both workflows; confirm triggers + safe_outputs.
  - Inherit agentics ref from existing default (96b9d4c...). Omit gh-aw source (not used).
  - Propose JSON block for fleet.local.json's profiles.experimental and profiles/experimental.json.
  - On approval: write both files. Remind about deploy command.
```

### Example 2 — materialize fleet-eval-templates output

```
User: take the docs-plus candidates we just discussed and make a profile

You:
  - Confirm scope: name=docs-plus (check — already exists in fleet.json). This is an edit
    of an existing profile, not a new build. Flag this and ask whether to merge or pick a
    new name (e.g., docs-plus-strict).
  - On user choice (merge into docs-plus): switch out of this skill — it's an append edit,
    not a scaffold. Just do the edit directly.
  - On user choice (new name, docs-plus-strict): continue. Pin agentics to SHA from default.
    Propose JSON block. Write to fleet.json on approval (user wants it shared).
```

### Example 3 — editing the default profile

```
User: add dependency-freshness-scanner to the default profile

You:
  - name=default: this triggers the mirror invariant. Target = fleet.json AND
    profiles/default.json.
  - Verify current alignment: diff fleet.json's profiles.default against profiles/default.json.
    If drift, fix alignment first; flag to the user.
  - Verify workflow in templates.json. Confirm safe_outputs are modest (create-issue only,
    say) — otherwise it's a bad fit for default's low-noise framing.
  - Propose diff for both files. Write both on approval. Remind about
    `sync --apply --force <repo>` for each already-deployed repo, since pin bumps don't
    flow through upgrade.
```
