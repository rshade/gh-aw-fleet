# Contract: CLI Semantics (deploy / upgrade / add)

**Date**: 2026-05-17
**Applies to**: `gh-aw-fleet deploy`, `gh-aw-fleet upgrade`, `gh-aw-fleet add`
**Spec reference**: [spec.md FR-004 through FR-011, FR-016](../spec.md#functional-requirements)

This contract documents the user-visible CLI behavior changes. No new flags are added — all behavior is config-driven via `compile_strict` in `fleet.json` / `fleet.local.json`.

## `gh-aw-fleet deploy <repo>` — new behavior

### Dry-run mode (default, no `--apply`)

The resolver runs (potentially with a visibility lookup). One info-level log line emits to stderr:

```text
INF compile-strict resolution event=compile_strict_resolved repo=rshade/gh-aw-fleet effective=true source=auto-public
```

The compile step is **NOT** invoked — dry-run does not invoke any subprocess that mutates files in the clone. The operator sees the would-be policy and can confirm before `--apply`.

When the resolver fails (FR-007), one additional warn line emits:

```text
WRN compile-strict visibility lookup failed; defaulting to strict ON event=compile_strict_lookup_failed repo=rshade/gh-aw-fleet reason=HTTP 403
```

### Apply mode (`--apply`)

After the existing `runAdd` loop completes (filling `res.Added` / `res.Skipped` / `res.Failed`) and BEFORE `git add .github/`:

1. The resolver runs (if it hasn't already during preflight) and populates `res.CompileStrictApplied` / `res.CompileStrictSource`.
2. If `Effective == false`, skip to step 5. No probe, no compile.
3. The probe (FR-016) runs: `gh aw compile --help`.
   - `flag-present` → continue to step 4.
   - `flag-absent` → abort. Preserve clone. Emit `DiagGhAwTooOld` diagnostic and structured stderr.
   - `probe-failed` → abort. Preserve clone. Emit `DiagGhAwMissing` diagnostic.
4. `gh aw compile --strict` runs in the clone directory. Output is tee'd to stderr.
   - Exit 0 → `res.CompileStrictApplied = true`. Continue to step 5.
   - Non-zero exit → abort. Preserve clone. Emit `DiagCompileStrictFailed` diagnostic; raw compile stderr included in the error message.
5. Proceed with the existing `git add .github/` → commit → push → PR pipeline.

### Resume from `--work-dir`

When deploy resumes via `--work-dir <path>` (interrupted prior run), the resolver MUST run again (visibility might have changed) and the compile step MUST re-run (idempotent — re-compiling already-strict-compiled lock files produces no diff). This matches the existing `hasStagedOrUnstagedWorkflowChanges` gate's "complete what we started" semantics.

### Error semantics

| Failure point | Exit code | Clone preserved? | PR created? |
|---------------|-----------|------------------|-------------|
| Visibility lookup fails (`auto-fallback`) | Continues — `warn` logged, deploy proceeds with strict ON. | N/A (still healthy) | Yes (assuming downstream success) |
| Probe `flag-absent` (gh aw too old) | Non-zero | Yes | No |
| Probe `probe-failed` (gh aw missing) | Non-zero | Yes | No |
| `gh aw compile --strict` fails | Non-zero | Yes | No |
| Otherwise (existing failure modes) | Non-zero | Yes (existing behavior) | No |

## `gh-aw-fleet upgrade <repo>` — new behavior

Symmetric to `deploy` above. The probe + compile step runs after `runUpgrade` succeeds (and after `runUpdate` if also run, when applicable) and before `git add .github/`. All five failure-mode entries in the table above apply identically.

The `UpgradeResult` envelope (when `--output json`) gains the same two fields as `DeployResult`.

## `gh-aw-fleet sync <repo>` — no direct changes

Sync's `applyDeployOrPrune` path delegates to `Deploy`, which inherits the compile step transitively. No new code in `sync.go`. Sync's drift-only / dry-run paths do not produce `.lock.yml` changes and therefore do not invoke compile.

`SyncResult.Deploy` (the embedded `DeployResult`) gains the two new fields via the underlying `DeployResult` extension. Operators reading `SyncResult` via `--output json` see them at `.result.deploy.compile_strict_applied` / `.result.deploy.compile_strict_source`.

## `gh-aw-fleet add <owner/repo>` — new behavior

After the existing add operation succeeds (config written to `fleet.local.json`), the visibility resolver runs once and prints ONE stdout info line:

```text
public repo: workflows will be compiled with --strict on next deploy (auto-on; override with "compile_strict": false in fleet.local.json)
```

Or, for a private/internal repo:

```text
private repo: workflows will NOT be compiled with --strict on next deploy (auto-off; override with "compile_strict": true in fleet.local.json)
```

When the visibility lookup fails, the info line is omitted entirely. The add itself succeeds — the operator will discover the policy at deploy time via the FR-007 `warn` log.

This info line is on stdout (it's expected behavior, not a warning), distinct from the deploy-time stderr log lines.

## Config file shape

### `fleet.local.json` (or `fleet.json`)

```jsonc
{
  "schema_version": 1,
  "repos": {
    "rshade/gh-aw-fleet": {
      "profiles": ["default"],
      "compile_strict": true        // NEW: explicit override
    },
    "rshade/legacy-migration": {
      "profiles": ["default"],
      "compile_strict": false       // NEW: opt out for a workflow that can't be strict
    },
    "rshade/regular-public-repo": {
      "profiles": ["default"]
                                    // NEW: omitted → auto-detect (public → strict ON)
    }
  }
}
```

### Round-trip semantics

- Absence of `compile_strict` on existing repo specs: byte-identical round-trip via HuJson AST mutation.
- Adding `compile_strict` via hand-edit: HuJson preserves operator comments on adjacent fields.
- Adding `compile_strict` via `gh-aw-fleet add` (future enhancement — not in this slice): would use the existing `Add` AST path; no schema-version bump.

## Help text additions

`gh-aw-fleet deploy --help`, `gh-aw-fleet upgrade --help`, and `gh-aw-fleet add --help` each gain a one-line note in their Long description referencing the `compile_strict` field and the README section for details. No new flags. The man-page-style `--help` output stays under the existing 80-column conventions.
