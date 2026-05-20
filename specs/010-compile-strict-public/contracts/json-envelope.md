# Contract: JSON Output Envelope Extensions

**Date**: 2026-05-17
**Applies to**: `gh-aw-fleet deploy --output json`, `gh-aw-fleet upgrade --output json`
**Spec reference**: [spec.md FR-015](../spec.md#functional-requirements), [data-model.md E4](../data-model.md#e4--deployresult--upgraderesult-extension)

This contract documents the additive surface added to the `--output json` envelope for Deploy and Upgrade. The envelope schema version (`cmd.SchemaVersion`) is NOT bumped — both fields default to their zero values and are ignored by consumers that don't know about them.

## Top-level `result` extension

The `result` object emitted by `deploy` and `upgrade` (when invoked with `--output json`) gains two new fields:

```json
{
  "result": {
    "repo": "rshade/gh-aw-fleet",
    "clone_dir": "/tmp/gh-aw-fleet-1234567",
    "added": [...],
    "skipped": [...],
    "failed": [...],
    "init_was_run": true,
    "branch_pushed": "fleet/deploy-2026-05-17-200314",
    "pr_url": "https://github.com/rshade/gh-aw-fleet/pull/123",
    "missing_secret": "",
    "secret_key_url": "",
    "actions_disabled": false,
    "workflow_token_read_only": false,
    "security_findings": null,

    "compile_strict_applied": true,
    "compile_strict_source": "auto-public"
  }
}
```

## Field semantics

### `compile_strict_applied` (`bool`)

| Value | Meaning |
|-------|---------|
| `true` | `gh aw compile --strict` was invoked AND completed successfully in this Deploy/Upgrade run. |
| `false` | Compile was either skipped (Effective resolved to `false`), aborted by the FR-016 probe, aborted by compile failure, or the resolver never ran (early-error path). Consumers MUST cross-reference `compile_strict_source` to disambiguate. |

### `compile_strict_source` (`string`)

| Value | Resolver branch | `applied` implication |
|-------|----------------|----------------------|
| `"explicit"` | `RepoSpec.CompileStrict != nil` → operator override (true or false) | `applied == true` iff `*RepoSpec.CompileStrict == true` AND no probe / compile abort. |
| `"auto-public"` | Visibility lookup returned `"public"` AND no explicit override | `applied == true` iff no probe / compile abort. |
| `"auto-private"` | Visibility lookup returned non-`"public"` AND no explicit override | `applied` is always `false` (compile was not attempted). |
| `"auto-fallback"` | Visibility lookup failed (403, 404, 5xx, network, malformed JSON, missing field) AND no explicit override | `applied == true` iff no probe / compile abort. The `warn`-level FR-007 log line for the same invocation carries the underlying reason. |
| `""` (empty) | Resolver was not invoked (e.g., result returned before reaching the resolver) | Consumers MUST treat as "not applicable to this result." Distinct from any of the four valid values. |

## Consumer gating examples

**Detect fail-secure activation** (the case most operators want to alert on — deploy proceeded with strict ON because visibility lookup failed):

```bash
gh-aw-fleet deploy <repo> --output json | jq '
  select(
    .result.compile_strict_applied == true and
    .result.compile_strict_source == "auto-fallback"
  )
'
```

**Detect that a public repo was deployed with strict OFF** (override active):

```bash
gh-aw-fleet deploy <repo> --output json | jq '
  select(
    .result.compile_strict_applied == false and
    .result.compile_strict_source == "explicit"
  )
'
```

**Detect that the compile step succeeded for any reason**:

```bash
gh-aw-fleet deploy <repo> --output json | jq 'select(.result.compile_strict_applied == true)'
```

## Backwards compatibility

- Pre-feature consumers that read `result` as a flat object and ignore unknown keys: **unaffected**.
- Pre-feature consumers using `jq` paths like `.result.added` or `.result.pr_url`: **unaffected**.
- Pre-feature consumers asserting exact `result` field counts (uncommon): **affected** — they will see two additional fields and should be relaxed to "fields ≥ N."
- The new fields appear in EVERY `deploy` / `upgrade` envelope from this release forward, even when the resolver didn't run (defaults: `false` / `""`).

## Warnings array (`warnings[]`)

When the FR-016 probe aborts the operation, a `Diagnostic` entry is appended to the existing `warnings[]` array with the codes from data-model.md E5:

```json
{
  "warnings": [
    {
      "code": "gh_aw_too_old",
      "message": "Local gh aw version is too old (detected v0.50.0; minimum v0.68.3). Upgrade with `gh extension upgrade aw`. To bypass, set \"compile_strict\": false for repos that don't need strict compile.",
      "fields": {
        "detected_version": "v0.50.0",
        "minimum_version": "v0.68.3",
        "repo": "rshade/gh-aw-fleet"
      }
    }
  ]
}
```

Diagnostic codes added by this feature:

- `compile_strict_failed`
- `gh_aw_too_old`
- `gh_aw_missing`

Each follows the existing `Diagnostic` shape (`code`, `message`, `fields`).
