# Quickstart: Billing Metadata Fields

**Feature**: 007-billing-metadata-fields
**Audience**: Fleet operators after this feature lands in a release.

This walks through the three operator tasks the new fields unlock: annotating a profile with a cost tier, attributing a repo to a cost center, and reading both back.

## Prerequisites

- A working `gh-aw-fleet` install at or after the release whose `feat(billing): ...` commit lands on `main`.
- A fleet configuration on disk — either `fleet.json` (the public, committed example) or `fleet.local.json` (the private, gitignored real fleet).
- Editor access to those files.

## Task 1: Annotate a profile with a cost tier

Edit `profiles/default.json` (the canonical default profile) or any profile inside `fleet.json` / `fleet.local.json`:

```json
{
  "description": "Baseline profile every fleet repo uses.",
  "tier": "standard",
  "sources": { ... },
  "workflows": [ ... ]
}
```

Recommended tier values:

| Value | When to use |
| ----- | ----------- |
| `minimal` | Completion-only or read-only profiles — cheap by construction. |
| `standard` | Foundational profiles with one or two PR-touching workflows. |
| `premium` | PR-generating agentic loops, multi-model reasoning, daily LLM cron jobs. |

The vocabulary is **advisory** — the tool does not enforce a closed set. If your org has a different cost-tier vocabulary (e.g., `dev | prod`), use that — values are preserved verbatim.

**Important**: if you edit `profiles/default.json`, you must mirror the same change into `fleet.json`'s `profiles.default` block — the project hard invariant keeps those two byte-identical. The tool does not enforce this; the AGENTS.md note is the source of the rule.

## Task 2: Attribute a repo to a cost center

Edit `fleet.local.json` (the private file — keep cost-center values out of the public committed `fleet.json` unless your cost-center names are public-safe):

```json
{
  "repos": {
    "your-org/your-repo": {
      "profiles": ["default", "security-plus"],
      "cost_center": "platform-eng",
      "engine": "copilot"
    }
  }
}
```

The value is a **free-form string**. It SHOULD match the cost-center name configured in your GitHub org's billing UI, but the tool does not validate that — typos surface as "no rollup for that center" in the future `consumption` command, not as load-time errors.

## Task 3: Read both fields back

### Text mode (default)

```sh
$ gh-aw-fleet list
  (loaded fleet.json + fleet.local.json)
REPO                  PROFILES                    TIERS                 ENGINE   WORKFLOWS  EXCLUDED  EXTRA  COST_CENTER
your-org/your-repo    [default security-plus]     [standard premium]    copilot  4          []        0      platform-eng
```

- The new `TIERS` column appears between `PROFILES` and `ENGINE`. Slice positions correspond 1:1 with `PROFILES`: `default` → `standard`, `security-plus` → `premium`.
- A profile with no tier renders as `-` in the corresponding `TIERS` slot. When every profile in the row is untiered, the cell renders `[]` (matching the existing slice-empty convention).
- The new `COST_CENTER` column appears at the end. Shows the value, or `-` when unset.

### JSON mode

```sh
$ gh-aw-fleet list --output json | jq '.result.repos[0]'
{
  "repo": "your-org/your-repo",
  "profiles": ["default", "security-plus"],
  "profile_tiers": {
    "default": "standard",
    "security-plus": "premium"
  },
  "engine": "copilot",
  "workflows": [...],
  "excluded": [],
  "extra": [],
  "cost_center": "platform-eng"
}
```

- `profile_tiers` is keyed by profile name. Profiles without tiers are simply absent from the map. An empty map (`{}`) means no profiles on this row have tiers.
- `cost_center` is always present as a string. Empty string means unset.

## Recognizing unset cases

| Field | Set in config | Text output | JSON output |
| ----- | ------------- | ----------- | ----------- |
| `tier` on a profile | Yes | `TIERS` slot shows the value (e.g., `standard`) | profile key present in `profile_tiers` |
| `tier` on a profile | No (mixed row) | `TIERS` slot shows `-` | profile key absent from `profile_tiers` |
| All profiles untiered | — | `TIERS` cell renders `[]` | `"profile_tiers": {}` |
| `cost_center` on a repo | Yes | column shows value | `"cost_center": "platform-eng"` |
| `cost_center` on a repo | No | column shows `-` | `"cost_center": ""` |

## Verifying round-trip safety

If you want to confirm the tool preserves your annotations through a load/save cycle (for example, before bumping the schema), the `consumption` subcommand once it ships will exercise this implicitly. Today the round-trip is covered by tests in `internal/fleet/load_test.go`; no operator action is required.

## What this feature does NOT do

- It does NOT enforce a closed set of tier values. Misspellings (`stnadard`) load and surface verbatim.
- It does NOT validate that the named `cost_center` exists in your GitHub billing UI.
- It does NOT block `cost_center` from appearing in the public `fleet.json`. Operator self-polices via the existing public/private file split (FR-016).
- It does NOT bump `fleet.SchemaVersion` or the `--output json` envelope's `schema_version` — both fields are additive.
- It does NOT change any mutating command's behavior. `list` remains read-only.

## Next steps

The `gh-aw-fleet consumption` subcommand (filed as #57, planned for v0.3 alongside this feature) will use both fields as group-by keys:

```sh
gh-aw-fleet consumption --by tier        # rolls up spend per tier
gh-aw-fleet consumption --by cost-center # rolls up spend per cost center
```

Until that lands, the fields are documentation only — but they are documentation that the future command already knows how to read, with no additional schema work.
