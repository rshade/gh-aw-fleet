# Contract: JSON Envelope

**Feature**: 003-cli-output-json
**Spec FRs**: FR-003, FR-004, FR-005, FR-006, FR-007, FR-008, FR-009, FR-010, FR-015, FR-016, FR-020

This contract pins the wire shape of the JSON object emitted on stdout when `--output json` is active. It is the machine-readable contract that downstream consumers depend on. Changes to this contract require a `schema_version` bump.

---

## Top-level shape

```json
{
  "schema_version": 1,
  "command": "deploy",
  "repo": "rshade/gh-aw-fleet",
  "apply": false,
  "result": { "...": "command-specific struct, or null on pre-result failure" },
  "warnings": [
    { "code": "missing_secret", "message": "Actions secret ANTHROPIC_API_KEY is missing...", "fields": { "secret": "ANTHROPIC_API_KEY", "url": "https://..." } }
  ],
  "hints": [
    { "code": "hint", "message": "Workflow uses a property your installed gh aw CLI doesn't recognize...", "fields": { "hint": "Workflow uses a property your installed gh aw CLI doesn't recognize..." } }
  ]
}
```

All output is **compact single-line JSON** (FR-015). No pretty-printing. Consumers pipe through `jq` for formatting.

---

## Pinned top-level keys

All seven keys MUST appear in every envelope (FR-004, SC-005). No `omitempty`. Ordering MAY vary (JSON objects are unordered by spec) but the `encoding/json` emission order is stable Go-field-declaration order in practice — consumers MUST NOT depend on it.

| Key | Type | Nullable | Notes |
|---|---|---|---|
| `schema_version` | integer | no | `1` for this release. Monotonically bumped on breaking changes. |
| `command` | string | no | One of: `"list"`, `"deploy"`, `"sync"`, `"upgrade"`. |
| `repo` | string | no (may be empty string) | `owner/name`; empty for `list` or on pre-result failure before repo parse. |
| `apply` | boolean | no | `true` only when `--apply` was passed AND the command is one that mutates (`deploy`, `sync`, `upgrade`). `false` for `list`, and for dry-runs. |
| `result` | object \| null | yes | Command-specific struct; `null` on pre-result failure (FR-020). |
| `warnings` | array | no (may be `[]`) | Array of `Diagnostic`. Always present. `[]` when no warnings. |
| `hints` | array | no (may be `[]`) | Array of `Diagnostic`. Always present. `[]` when no hints. |

---

## `schema_version` policy

- Integer, initially `1`.
- Bumped (monotonic `+1`) on any breaking change to:
  - top-level envelope keys (add/remove/rename)
  - `Diagnostic` shape
  - `ListResult` / `ListRow` field names or types
  - `DeployResult` / `WorkflowOutcome` / `SyncResult` / `UpgradeResult` JSON field names or types
- Additive changes (new optional fields on existing structs, new `Diagnostic` codes, new nested objects) do NOT bump the version — they are backward-compatible.
- Version history is maintained in `CHANGELOG.md` under the `### JSON envelope schema` sub-section.

**Consumer guidance**: Agents MUST check `schema_version === 1` and fail loudly on any other value. Forward-compatibility is not promised; a bump signals "re-read the contract before trusting fields."

---

## `command`

The canonical form used in the envelope. One of:

| Value | Emitting subcommand |
|---|---|
| `"list"` | `gh-aw-fleet list` |
| `"deploy"` | `gh-aw-fleet deploy <repo>` |
| `"sync"` | `gh-aw-fleet sync <repo>` |
| `"upgrade"` | `gh-aw-fleet upgrade <repo>` or `gh-aw-fleet upgrade --all` (per-repo line) |

**Not emitted**: `"template"`, `"template fetch"`, `"add"`, `"status"` — those subcommands reject `-o json`.

Consumers dispatching on `command` to apply a result-schema can use a stable Go switch / object lookup.

---

## `result`

The type of `result` depends on `command`:

| `command` | `result` type (Go) |
|---|---|
| `"list"` | `*fleet.ListResult` |
| `"deploy"` | `*fleet.DeployResult` |
| `"sync"` | `*fleet.SyncResult` |
| `"upgrade"` | `*fleet.UpgradeResult` |

All result fields use snake_case JSON keys (FR-008). See `data-model.md` for the full field list per struct.

**Null semantics**:
- `result` MUST be `null` when the command failed before building its result struct (FR-020: config parse error, missing tool, repo not in fleet, profile resolution failure). In this case, `warnings[]` or `hints[]` MUST carry enough structured context for the consumer to understand the failure.
- `result` MUST NOT be `null` when the command structurally succeeded — even if the command's logical outcome is "empty" (e.g., a sync run with no drift produces `{"missing":[], "drift":[], "expected":[...]}`, not `null`).

---

## Slice normalization (FR-006, FR-009)

Every array field in the envelope MUST serialize as `[]` (not `null`) when empty. This applies recursively:

- `warnings`, `hints` at the envelope top level
- Every slice field of every result struct (`added`, `skipped`, `failed` in `DeployResult`; `missing`, `drift`, `expected`, `pruned` in `SyncResult`; `changed_files`, `conflicts` in `UpgradeResult`; `profiles`, `workflows`, `excluded`, `extra` in `ListRow`; `repos` in `ListResult`)
- Future-added slice fields on any of the above

**Enforcement**: `cmd.output.initSlices(result)` reflection helper (see `research.md` R2) walks the result struct and sets all nil slices to empty non-nil slices before `json.Encoder.Encode`. Runs unconditionally on every envelope emission.

**Non-slice-but-nullable fields** (e.g., `SyncResult.Deploy *DeployResult`, `SyncResult.DeployPreflight *DeployResult`) keep their nil semantics and marshal as `null` when unset. The helper skips pointer-to-struct fields intentionally.

---

## Stdout vs stderr routing (FR-010, FR-011, FR-012)

Stdout in JSON mode contains EXACTLY:
- One complete JSON envelope (single-repo commands), OR
- N complete JSON envelopes separated by newlines (NDJSON for `upgrade --all`; see `contracts/ndjson.md`).

Nothing else. No breadcrumbs, no progress, no diagnostic text. `jq -e . < <(cmd -o json)` succeeds for every invocation (SC-002).

Stderr in JSON mode contains:
- Zerolog structured events for warnings, hints, errors (filtered by `--log-level`).
- Cobra's own error output on flag validation failure (plain text, not JSON).
- Any `fmt.Fprintf(cmd.OutOrStderr(), ...)` calls that existed pre-feature (e.g., `(loaded fleet.local.json)` breadcrumb in `list.go`).

Text mode is unaffected — stdout keeps its existing tabwriter content; stderr keeps its existing zerolog events.

---

## `audit_json` nesting (FR-016)

`UpgradeResult.AuditJSON` is `json.RawMessage`. When populated (typical upgrade run), it MUST serialize as a native nested JSON object in the envelope — not as a string-wrapped escape-encoded blob.

Good (expected):
```json
{ "result": { "audit_json": { "version": "1", "findings": [] } } }
```

Bad (regression):
```json
{ "result": { "audit_json": "{\"version\":\"1\",\"findings\":[]}" } }
```

This is the default behavior of `json.RawMessage` with the stdlib `encoding/json` package. No additional code is required. A regression test (`TestEnvelope_AuditJSONNests`) pins this against future type changes.

When `AuditJSON` is unset (e.g., `gh aw audit` was not invoked on this upgrade), it MUST marshal as JSON `null` — the default for a nil `RawMessage`.

---

## Pre-result failure shape (FR-020)

When a command cannot build its result struct, stdout still gets one complete envelope. Example — `deploy nonexistent/repo -o json`:

```json
{
  "schema_version": 1,
  "command": "deploy",
  "repo": "nonexistent/repo",
  "apply": false,
  "result": null,
  "warnings": [],
  "hints": [
    { "code": "hint", "message": "Repo \"nonexistent/repo\" is not declared in fleet.json; add it with `gh-aw-fleet add nonexistent/repo` first.", "fields": { "hint": "..." } }
  ]
}
```

Exit code is non-zero. `jq -e .` succeeds (SC-002). `jq '.result == null'` returns `true`, which is the canonical test for "command did not produce a result."

---

## Exit code contract (FR-021)

JSON mode exit codes mirror text mode exactly. There is no new exit-code contract. Summary:

| Condition | Exit code | Both modes |
|---|---|---|
| Clean success | 0 | ✓ |
| Warnings present, structurally succeeded | 0 (text-mode parity) | ✓ |
| Pre-result failure (config parse, missing tool, repo not in fleet) | non-zero | ✓ |
| `--apply` failure mid-pipeline | non-zero | ✓ |
| Flag validation failure (e.g., `-o yaml`) | non-zero | ✓ |

Agents requiring warning-level gating MUST parse `warnings[]` from the envelope rather than infer from exit code.

---

## Test fixture (pinning)

`cmd/output_test.go` contains a `TestEnvelope_TopLevelKeysPinned` test that marshals a synthetic envelope (empty result struct, empty warnings, empty hints) and asserts the output matches exactly:

```json
{"schema_version":1,"command":"list","repo":"","apply":false,"result":{"loaded_from":"","repos":[]},"warnings":[],"hints":[]}
```

This fails if any top-level key is added, removed, or renamed — including accidental drift during refactor.
