# Phase 1 Data Model: CLI JSON Output Mode

**Feature**: 003-cli-output-json
**Date**: 2026-04-21

This feature has no persistent state (no database, no config file, no cache). The "data model" here describes the runtime and serialization entities that shape the JSON envelope emitted on stdout. All entities live in memory for the lifetime of one CLI invocation.

---

## Entity 1: Envelope

**Purpose**: The top-level JSON object emitted on stdout when `--output json` is active. Every invocation of a JSON-supporting subcommand (`list`, `deploy`, `sync`, `upgrade`) produces exactly one envelope — or N envelopes for `upgrade --all` (one per repo, NDJSON).

**Location**: Defined in `cmd/output.go`.

| Field | JSON key | Type | Presence | Source |
|---|---|---|---|---|
| `SchemaVersion` | `schema_version` | `int` | always | constant `1` for this release |
| `Command` | `command` | `string` | always | one of `"list"`, `"deploy"`, `"sync"`, `"upgrade"` |
| `Repo` | `repo` | `string` | always | target repo (`owner/name`); empty string for `list`; may be empty on pre-result failure if not yet parsed |
| `Apply` | `apply` | `bool` | always | `true` only when `--apply` was passed AND the command would mutate; `false` otherwise (including `list`, which has no apply path) |
| `Result` | `result` | command-specific struct or `null` | always (value may be `null`) | `ListResult` / `DeployResult` / `SyncResult` / `UpgradeResult`, or `null` on pre-result failure (FR-020) |
| `Warnings` | `warnings` | `[]Diagnostic` | always (may be `[]`) | collected at warning sites in each command |
| `Hints` | `hints` | `[]Diagnostic` | always (may be `[]`) | from `fleet.CollectHintDiagnostics` |

**Invariants**:
- `warnings` and `hints` marshal as `[]` when empty, NEVER `null` (FR-006, FR-009). Enforced by `initSlices` helper in envelope writer.
- Every field listed is present on every envelope — FR-004 pins the top-level key set (`schema_version`, `command`, `repo`, `apply`, `result`, `warnings`, `hints`). No `omitempty` on envelope fields.
- `schema_version` is the **only** machine-readable schema contract; bumps are monotonic and signal breaking changes to envelope or result field shapes (FR-005).

**Go struct sketch** (for reference; actual code lives in `cmd/output.go`):

```go
type Envelope struct {
    SchemaVersion int          `json:"schema_version"`
    Command       string       `json:"command"`
    Repo          string       `json:"repo"`
    Apply         bool         `json:"apply"`
    Result        any          `json:"result"` // nil → JSON null
    Warnings      []Diagnostic `json:"warnings"`
    Hints         []Diagnostic `json:"hints"`
}
```

---

## Entity 2: Diagnostic

**Purpose**: Shared shape for entries in `warnings[]` and `hints[]`. A single row that a downstream consumer can machine-parse (`code`) or render to a human (`message`) with structured context for downstream automation (`fields`).

**Location**: Defined in `cmd/output.go`. Constructed at warning sites in command code; constructed by `fleet.CollectHintDiagnostics` for hints.

| Field | JSON key | Type | Presence | Notes |
|---|---|---|---|---|
| `Code` | `code` | `string` | always | stable, snake_case identifier — e.g., `missing_secret`, `drift_detected`, `hint`, `unknown_property`, `http_404`, `gpg_failure` |
| `Message` | `message` | `string` | always | human-readable text; same string that would appear in text-mode `⚠ WARNING:` or `hint:` output |
| `Fields` | `fields` | `map[string]any` | optional (omitted when empty) | structured context: `{secret: "ANTHROPIC_API_KEY", url: "..."}` etc. |

**Go struct sketch**:

```go
type Diagnostic struct {
    Code    string         `json:"code"`
    Message string         `json:"message"`
    Fields  map[string]any `json:"fields,omitempty"`
}
```

**Diagnostic code catalogue** (initial):

| Code | Source | Fields |
|---|---|---|
| `missing_secret` | `cmd/deploy.go` MissingSecret detection | `secret` (name), `url` (key-retrieval URL) |
| `drift_detected` | `cmd/sync.go` drift detection | `drift` (array of workflow filenames) |
| `hint` | `fleet.CollectHintDiagnostics` wrapper — every hint from `fleet/diagnostics.go` | `hint` (full hint text, duplicated for `jq '.fields.hint'` filter convenience) |
| `unknown_property` | hint classifier | `pattern` (the offending property name, when extractable) |
| `http_404` | hint classifier | `pattern` (the URL or path that 404'd, when extractable) |
| `gpg_failure` | hint classifier | (no additional fields — message is self-explanatory) |

Adding a new diagnostic code:
1. Define a package-level constant in `cmd/output.go` (e.g., `DiagMissingSecret = "missing_secret"`).
2. At the warning site, build `Diagnostic{Code: DiagMissingSecret, Message: "...", Fields: map[string]any{...}}`.
3. Append to the local `[]Diagnostic` slice that the command threads into the envelope.

---

## Entity 3: ListResult (NEW)

**Purpose**: Machine-readable form of the data `cmd/list.go` currently prints as a tabwriter table. The existing list command builds rows inline and flushes them; this feature extracts the data into a typed struct so it can be embedded in the envelope.

**Location**: Defined in `internal/fleet/list_result.go` (new file).

| Field | JSON key | Type | Source |
|---|---|---|---|
| `LoadedFrom` | `loaded_from` | `string` | `Config.LoadedFrom` — one of `"fleet.local.json"` or `"fleet.json"` |
| `Repos` | `repos` | `[]ListRow` | sorted by repo name (same ordering as existing tabwriter output) |

```go
type ListResult struct {
    LoadedFrom string    `json:"loaded_from"`
    Repos      []ListRow `json:"repos"`
}
```

### ListRow (nested inside ListResult.Repos)

| Field | JSON key | Type | Source |
|---|---|---|---|
| `Repo` | `repo` | `string` | map key from `Config.Repos` (e.g., `"rshade/gh-aw-fleet"`) |
| `Profiles` | `profiles` | `[]string` | `Config.Repos[repo].Profiles` |
| `Engine` | `engine` | `string` | `Config.EffectiveEngine(repo)` — empty string renders as `""`, NOT the text-mode `"-"` placeholder (that's a human-readability hack) |
| `Workflows` | `workflows` | `[]string` | names of workflows resolved from `Config.ResolveRepoWorkflows(repo)` |
| `Excluded` | `excluded` | `[]string` | `Config.Repos[repo].ExcludeFromProfiles`; `[]` when absent |
| `Extra` | `extra` | `[]string` | names from `Config.Repos[repo].ExtraWorkflows` |

```go
type ListRow struct {
    Repo      string   `json:"repo"`
    Profiles  []string `json:"profiles"`
    Engine    string   `json:"engine"`
    Workflows []string `json:"workflows"`
    Excluded  []string `json:"excluded"`
    Extra     []string `json:"extra"`
}
```

**Key shape difference from text mode**: Text mode renders empty engine as `"-"` (via `orDash` in `cmd/list.go`). JSON mode renders it as `""` (empty string). This is intentional — `"-"` is a human placeholder; machine consumers use truthiness on the empty string.

**Construction**: A new `fleet.BuildListResult(cfg *Config) (*ListResult, error)` function walks `cfg.Repos`, sorts keys, resolves workflows per repo, and returns the populated struct. `cmd/list.go`'s JSON branch calls this; the text-mode branch keeps its existing inline tabwriter code.

---

## Entity 4: DeployResult (EXISTING — JSON tags added)

**Location**: `internal/fleet/deploy.go`. Struct shape unchanged; only JSON tags added.

| Go field | JSON key | Type | Notes |
|---|---|---|---|
| `Repo` | `repo` | `string` | |
| `CloneDir` | `clone_dir` | `string` | `/tmp/gh-aw-fleet-*` path; preserved on `--apply` failure |
| `Added` | `added` | `[]WorkflowOutcome` | MUST be `[]` when empty (FR-009) |
| `Skipped` | `skipped` | `[]WorkflowOutcome` | MUST be `[]` when empty |
| `Failed` | `failed` | `[]WorkflowOutcome` | MUST be `[]` when empty |
| `InitWasRun` | `init_was_run` | `bool` | `true` when `gh aw init` was invoked during deploy |
| `BranchPushed` | `branch_pushed` | `string` | branch name pushed to remote, empty on dry-run |
| `PRURL` | `pr_url` | `string` | URL returned by `gh pr create`, empty on dry-run or on push failure |
| `MissingSecret` | `missing_secret` | `string` | secret name (e.g., `ANTHROPIC_API_KEY`) when absent; empty when present or unchecked |
| `SecretKeyURL` | `secret_key_url` | `string` | URL to retrieve the missing secret key |

### WorkflowOutcome (nested inside Added/Skipped/Failed)

| Go field | JSON key | Type | Notes |
|---|---|---|---|
| `Name` | `name` | `string` | workflow filename without `.md` |
| `Spec` | `spec` | `string` | `owner/repo/path@ref` form used by `gh aw add` |
| `Reason` | `reason` | `string` | human-readable classification (e.g., `"added"`, `"already present"`, `"compile failed"`) |
| `Error` | `error` | `string` | error text from `gh aw` / `git`; empty when outcome was not an error |

---

## Entity 5: SyncResult (EXISTING — JSON tags added)

**Location**: `internal/fleet/sync.go`.

| Go field | JSON key | Type | Notes |
|---|---|---|---|
| `Repo` | `repo` | `string` | |
| `CloneDir` | `clone_dir` | `string` | |
| `Missing` | `missing` | `[]string` | workflow names declared in fleet.json but absent from the repo; MUST be `[]` when empty |
| `Drift` | `drift` | `[]string` | workflow files present in the repo but not declared; MUST be `[]` when empty |
| `Expected` | `expected` | `[]string` | informational: workflow names matching fleet.json |
| `Deploy` | `deploy` | `*DeployResult` (pointer, may be nil) | set when `Apply==true` AND a deploy was triggered; MUST marshal as `null` when unset |
| `Pruned` | `pruned` | `[]string` | files removed by `--prune --apply`; MUST be `[]` when empty |
| `DeployPreflight` | `deploy_preflight` | `*DeployResult` (pointer, may be nil) | set on dry-run (`Apply==false`) to surface compilation failures; MUST marshal as `null` when unset |

**Key rule for pointer fields**: `Deploy` and `DeployPreflight` are pointers. A nil pointer marshals as JSON `null` (stdlib default). No `initSlices`-style pre-processing needed — pointers and their nil semantics map cleanly.

---

## Entity 6: UpgradeResult (EXISTING — JSON tags added)

**Location**: `internal/fleet/upgrade.go`.

| Go field | JSON key | Type | Notes |
|---|---|---|---|
| `Repo` | `repo` | `string` | |
| `CloneDir` | `clone_dir` | `string` | |
| `UpgradeOK` | `upgrade_ok` | `bool` | |
| `UpdateOK` | `update_ok` | `bool` | |
| `ChangedFiles` | `changed_files` | `[]string` | MUST be `[]` when empty |
| `Conflicts` | `conflicts` | `[]string` | MUST be `[]` when empty |
| `NoChanges` | `no_changes` | `bool` | |
| `BranchPushed` | `branch_pushed` | `string` | |
| `PRURL` | `pr_url` | `string` | |
| `AuditJSON` | `audit_json` | `json.RawMessage` | nested native JSON object when non-nil (FR-016); `null` when nil |
| `OutputLog` | `output_log` | `string` | combined stdout+stderr from `gh aw upgrade/update`; used by `CollectHints` |

**Note on `output_log`**: This field today contains raw `gh aw` stderr that may include non-printable characters, ANSI color codes, and multi-MB payloads on large repos. Embedding it inline in the envelope preserves the existing data fidelity but may bloat the JSON. This is accepted — consumers who don't need it can drop the field with `jq 'del(.result.output_log)'`. A future feature may add a `--output-log=false` flag to suppress it; out of scope here.

---

## Relationships

```text
Envelope
├── Result: any (one of)
│   ├── *ListResult
│   │   └── Repos: []ListRow
│   ├── *DeployResult
│   │   ├── Added:   []WorkflowOutcome
│   │   ├── Skipped: []WorkflowOutcome
│   │   └── Failed:  []WorkflowOutcome
│   ├── *SyncResult
│   │   ├── Deploy:          *DeployResult (nullable)
│   │   └── DeployPreflight: *DeployResult (nullable)
│   └── *UpgradeResult
│       └── AuditJSON: json.RawMessage (nested JSON)
├── Warnings: []Diagnostic
└── Hints:    []Diagnostic
```

No circular references. No mutually recursive types. All serialization is a single downward walk from the envelope root.

---

## Validation rules

Enforced at envelope-writer boundary (`writeEnvelope` in `cmd/output.go`):

1. **Command-tag agreement** (defensive): reject at compile time — each subcommand calls `writeEnvelope(cmd, result, ...)` with its own hardcoded `cmd` string (`"list"`, `"deploy"`, `"sync"`, `"upgrade"`). No runtime validation needed.
2. **Schema version pin**: `Envelope.SchemaVersion = 1` set by `writeEnvelope`; individual subcommands never set this directly.
3. **Empty-slice normalization**: `initSlices(result)` runs unconditionally before `json.Encoder.Encode`. Walks nested structs. Leaves pointer-to-struct fields alone (nil pointers stay nil for JSON `null` semantics; non-nil get walked).
4. **Pre-result failure shape**: when a command cannot build its result (config parse error, missing tool, repo not in fleet), the command returns early from its RunE with a `writeEnvelope(cmd, nil, warnings, hints)` call. The `result: null` rendering is handled by `Envelope.Result == nil` marshaling to JSON null.
5. **No `omitempty` on envelope top-level fields**: ensures all 7 keys are always present (FR-004, SC-005). `omitempty` IS used on `Diagnostic.Fields` only (so an empty-fields diagnostic doesn't emit `"fields":{}`).

---

## Lifecycle summary

```text
CLI invocation (`cmd -o json`)
  │
  ├─ PersistentPreRunE (root.go)
  │    └─ validateOutputMode(flag) — rejects yaml/JSON/etc before subcommand RunE
  │
  ├─ Subcommand RunE
  │    ├─ Compute result (existing business logic — untouched)
  │    ├─ Collect []Diagnostic warnings at existing zlog.Warn() sites
  │    ├─ Call fleet.CollectHintDiagnostics(...) for hints (instead of CollectHints)
  │    └─ Branch on outputMode():
  │        ├─ "text" → existing printXxxReport path (untouched)
  │        └─ "json" → writeEnvelope(cmd, result, warnings, hints)
  │
  └─ writeEnvelope (cmd/output.go)
       ├─ initSlices(result) — ensure empty slices, not nil
       ├─ Build Envelope{SchemaVersion: 1, Command, Repo, Apply, Result, Warnings, Hints}
       └─ json.NewEncoder(stdout).Encode(env) — emits compact JSON + newline
```

For `upgrade --all`: the subcommand iterates repos, calls `writeEnvelope` once per repo, and flushes between. No aggregate envelope is built.
