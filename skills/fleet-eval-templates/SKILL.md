---
name: fleet-eval-templates
description: Refresh the upstream template catalog (templates.json) and evaluate new or changed workflows against fleet profiles. Use this skill whenever the user says "check for new agentic workflows", "what's new in gh-aw or agentics", "refresh the template catalog", "evaluate new workflows", "run template fetch", "any new workflows worth adding", or "what changed upstream". Also trigger when the user asks about the upstream template catalog in general — additions, removals, or fit assessments. The skill runs `template fetch`, reads the resulting JSON in-chat (no subprocess LLM needed — the catalog stores full frontmatter + body), and recommends which new workflows deserve profile inclusion, which are niche, and which are internal/test noise. Do not trigger for deploying workflows to a repo (that's fleet-deploy), for pin-health auditing (that's fleet-upgrade-review), or for onboarding a new repo (fleet-onboard-repo).
---

# Fleet Eval Templates

Refresh `templates.json` from the upstream catalogs (`github/gh-aw` + `githubnext/agentics`), diff against the previous snapshot, and evaluate what's new or changed.

## Why this skill exists

`gh-aw-fleet` stores a local catalog of every upstream workflow — their full frontmatter, body, triggers, tools, and safe-outputs — in `templates.json`. The design intent is that you (the assistant) can evaluate new workflows *inline* by reading the JSON, without having to re-fetch or spawn a sub-LLM. That makes this skill cheap: the expensive mechanical step (cloning + parsing ~242 markdown files) is already done; the evaluation step is judgment.

The catalog tracks `main` for each source, not the profile's pinned tag. That's deliberate — the catalog answers "what exists upstream" (discovery), separate from pins (deployment). A workflow that shows up in the catalog but isn't yet pinned in any profile is exactly the thing this skill helps triage.

## When to use

- User asks what's new, what's changed, or whether a specific upstream workflow is worth adopting.
- User wants to periodically sweep for library improvements.
- Before bumping a profile's `source.ref` (a new tag or release), to see what's changed since the last pin.

Skip for:
- Deploying a workflow that already exists in a profile to a specific repo — `fleet-deploy`.
- Auditing pin drift / action versions across the fleet — `fleet-upgrade-review`.
- Adding a new repo to the fleet — `fleet-onboard-repo`.

## The flow

### Step 1 — fetch

```bash
cd /mnt/c/GitHub/go/src/github.com/rshade/gh-aw && go run . template fetch
```

The command lists `.github/workflows/*.md` (for gh-aw) and `workflows/*.md` (for agentics) at `main`, downloads each, parses frontmatter, and writes the result to `templates.json`. It also computes a diff against the prior `templates.json` and prints a summary:

```
github/gh-aw @ main
  added:     [...]
  changed:   [...]
  removed:   [...]
  unchanged: N

githubnext/agentics @ main
  added:     [...]
  ...
```

Capture this output. If `added + changed + removed` all empty across both sources, report "no catalog changes since last fetch" and stop.

### Step 2 — read the catalog

`templates.json` is a large JSON file (~3MB). Read it with targeted tools, not `cat`:

- To list all workflows in one source: `jq '.sources["githubnext/agentics"].workflows | map(.name)' templates.json`
- For a specific workflow's metadata: `jq '.sources["githubnext/agentics"].workflows[] | select(.name == "link-checker") | {description, triggers, tools, safe_outputs, lines}' templates.json`
- For the full frontmatter + body: `jq '.sources["githubnext/agentics"].workflows[] | select(.name == "link-checker") | .frontmatter, .body' templates.json` — careful, body can be hundreds of lines.

Focus on the diff's added/changed lists first. Don't scan all 242 entries unless the user asks.

### Step 3 — evaluate

For each new or changed workflow, read at minimum the `description`, `triggers`, and `safe_outputs`. Judge against the profile taxonomy:

- **default** — baseline every tracked repo gets. Low-noise. Event-triggered or lightweight daily. Examples: ci-doctor, pr-fix, issue-arborist.
- **quality-plus** — PR-generating quality agents. Noisier; opt-in. Examples: daily-test-improver, daily-perf-improver.
- **security-plus** — SAST, secret scanning, etc. Layer on `default`. Examples: daily-semgrep-scan, daily-secrets-analysis.
- **docs-plus** — heavier docs maintenance. Requires a real docs site. Examples: glossary-maintainer, link-checker, markdown-linter.
- **community-plus** — command-triggered helpers, dormant when idle. Examples: grumpy-reviewer, pr-nitpick-reviewer, archie, repo-ask.

Also distinguish from **none** — internal/test/smoke workflows (gh-aw itself has ~150 of these: `smoke-*`, `test-*`, `daily-choice-test`, `example-*`, etc.) that should not be in any profile. Flag them once, don't re-evaluate on subsequent runs.

Judgment criteria, in order:

1. **Frontmatter completeness.** If `triggers` or `safe_outputs` are null / empty / missing in `templates.json`, treat the workflow as "needs hands-on review." Read the full body (`jq '... | .body'`) before classifying — don't guess behavior from the name. Common reasons for missing fields: the workflow uses imports whose frontmatter isn't resolved at fetch time, or the upstream is mid-edit. Either way, the summary data isn't enough.
2. **Signal vs. noise.** Does it create PRs / issues / discussions on a schedule? How often? Will it fire on a quiet repo?
3. **Trigger type.** Event-triggered (zero idle cost) ≫ slash-command (user-initiated) ≫ schedule (fires regardless).
4. **Safe-outputs breadth.** `add-comment` is lightest. `create-pull-request` is significant. `push-to-pull-request-branch` is heaviest.
5. **Overlap.** Does it duplicate something already in a profile? (e.g., agentics `code-simplifier` vs gh-aw `code-simplifier`.)
6. **Required repo shape.** Does it assume a docs site? A particular CI setup? Dependabot? If yes, it belongs in a specialized profile, not `default`.
7. **Prior user preferences.** Before recommending, check `templates.json`'s `evaluations` map for an existing verdict — e.g., if `issue-triage` has `recommend: "skip — user uses /roadmap"`, respect that and don't re-suggest it. Evaluations persist across fetch runs exactly to prevent redundant re-litigation.

### Step 4 — recommend

For each new/changed workflow, produce a terse one-line verdict like:

- `link-checker` → **docs-plus candidate** — scheduled broken-link sweep; only useful with a docs site.
- `daily-efficiency-improver` → **skip** — too opinionated + high churn; would fight other quality-plus agents.
- `dependabot-pr-bundler` → **already in default** — this is the changed version; read diff to decide if we should bump the pin.
- `contribution-guidelines-checker` → **community-plus candidate** — PR-event-triggered, low idle cost.
- `smoke-codex` → **none (gh-aw internal test)** — not for fleets.

Group by verdict for readability. Show the user 5-10 top signals, not 40 lukewarm ones.

### Step 5 — (optional) propose a profile edit

If any candidates look worth adding, draft the specific fleet.json diff — which profile gets the workflow, which source/ref, any `exclude` implications per repo. Don't apply; show the diff and wait for user approval.

### Step 6 — (only if user asks) persist evaluations

Persistence is **not** part of the default flow. Steps 1-5 are read-only evaluation — you present findings, user decides. Do this step **only** when the user explicitly says "save these", "persist this", "write the evaluations back", or similar direct ask.

The `templates.json` schema has an `Evaluations` map keyed by workflow name. Write via `jq` so future fetch runs surface the prior verdict:

```bash
jq '.evaluations["some-workflow"] = {
  "evaluated_at": "2026-04-18T00:00:00Z",
  "summary": "docs-plus candidate - scheduled link checker",
  "fit_profiles": ["docs-plus"],
  "recommend": "add to docs-plus on next pin bump"
}' templates.json > templates.json.tmp && mv templates.json.tmp templates.json
```

Persist only for **stable** verdicts — "skip" for identified noise, "default candidate" with a clear reason, "user uses /roadmap instead" for decisions that won't change. Don't persist "needs review" or "unclear" — ambiguity shouldn't harden into stored state.

## Invariants

- **Don't modify `fleet.json` or `profiles/default.json`** as part of evaluation. This skill is read-only against declarative state; user approves any profile change in a follow-up turn.
- **Never hallucinate workflows.** If you can't find a workflow in `templates.json`, say so — don't guess at its behavior from its name.
- **Respect the framing.** `github/gh-aw` = compiler + dogfooding. `githubnext/agentics` = curated library. When recommending, weight toward agentics; flag gh-aw-sourced candidates with a note about upstream stability (gh-aw's main can contain unreleased features like `mount-as-clis` that break the installed CLI — see the `fleet.CollectHints` hint for context).
- **Don't run `gh aw` directly** as part of evaluation. The skill operates entirely on `templates.json` after the initial `fetch`.

## What "success" looks like

A concise, actionable summary organized like:

```
Catalog fetch: X new, Y changed, Z removed.

Default profile candidates:
  - <name>: <reason> (<source>@<ref>)

Specialized profile candidates:
  - quality-plus: <name> (<reason>)
  - docs-plus: <name> (<reason>)
  - community-plus: <name> (<reason>)

Skip / noise:
  - <name>: <short reason>

Significant changes to already-pinned workflows:
  - <name>: <what changed in the frontmatter or body> — recommend <action>
```

If the user asks for a deep read on any specific workflow, drop into details from `templates.json`.

## Examples

### Example 1 — periodic sweep, nothing new

```
User: check for new agentic workflows

You:
  - Run `go run . template fetch`.
  - Output: "added: -, changed: -, removed: -, unchanged: 242" for both sources.
  - Report: "No catalog changes since last fetch (previous: <date from FetchedAt>)."
```

### Example 2 — new library workflow

```
User: any new workflows worth adopting?

You:
  - Run fetch. Output shows `githubnext/agentics @ main` added `dependency-freshness-scanner`.
  - jq the frontmatter + description from templates.json.
  - Read body first ~50 lines via jq.
  - Evaluate: scheduled weekly, creates issues only (safe-outputs: create-issue), applies to any repo with a package manifest.
  - Verdict: `default` candidate (low noise, broadly applicable).
  - Propose fleet.json diff: add to `default.workflows`. No repo exclusions needed.
  - Show diff. Stop — wait for user.
```

### Example 3 — existing workflow materially changed

```
User: what changed upstream?

You:
  - Fetch. `githubnext/agentics` shows `pr-fix` in changed.
  - Read before/after frontmatter (previous version from git-history-of-templates.json isn't retained; rely on the new fetched content).
  - Note: new `safe-outputs: threat-detection` entry not in our pinned version.
  - Verdict: worth pin-bumping the agentics source in `default`. Flag that `threat-detection` is a new safe-output that might require opt-in at the gh-aw level.
  - Recommend: test on one repo first (finfocus) before bumping for all.
```

## Extended usage

For machine-readable output, pass `-o json` on `list`/`deploy`/`sync`/`upgrade`. See `specs/003-cli-output-json/quickstart.md`.
