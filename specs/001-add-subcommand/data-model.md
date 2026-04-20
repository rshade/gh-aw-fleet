# Data Model: `add <owner/repo>` Subcommand

**Phase**: 1 (design & contracts)
**Feature**: `specs/001-add-subcommand`

This document defines the new types introduced by this feature and
reiterates the existing types it composes with. Existing types
(defined elsewhere) are documented here only for interface clarity;
the authoritative definitions remain in `internal/fleet/schema.go`
and `internal/fleet/load.go`.

## New types (introduced by this feature)

### `AddOptions` (in `internal/fleet/add.go`)

The in-memory representation of command-line intent. Populated by
`cmd/add.go` from cobra flags, then passed to `fleet.Add()`.

```go
type AddOptions struct {
    Repo           string   // Normalized "owner/repo" (lowercased, trimmed).
    Profiles       []string // Profile names to assign. Length >= 1 enforced.
    Engine         string   // Optional engine override. "" means no override.
    Excludes       []string // Workflow names to exclude from profile members.
    ExtraWorkflows []string // Raw --extra-workflow flag values (parsed inside Add).
    Apply          bool     // false = dry-run; true = write to fleet.local.json.
    Confirmed      bool     // true if --yes OR interactive prompt accepted.
    Dir            string   // Working dir containing fleet.json (for LoadConfig / SaveLocalConfig).
}
```

**Validation rules** (enforced inside `Add()`, not in cobra):

| Field | Rule | On violation |
|-------|------|--------------|
| `Repo` | Must match `validateSlug` output (two lowercase `[a-z0-9._-]+` halves separated by `/`) | Return error naming the invalid slug and the rule it violated |
| `Profiles` | Length ≥ 1; every entry must exist in merged `cfg.Profiles` | Return error listing available profile names |
| `Engine` | If non-empty, must be a key of `EngineSecrets` | Return error listing accepted engine names |
| `ExtraWorkflows` | Each raw string must parse via `parseExtraWorkflowSpec` | Return error naming the offending spec with an example of the correct form |
| `Apply` + `Confirmed` | If `Apply == true`, `Confirmed` must also be true | Return error `"--apply requires --yes or interactive confirmation"` |

### `AddResult` (in `internal/fleet/add.go`)

The return value from `fleet.Add()`. Carries both the preview
payload (for rendering) and, in `--apply` mode, a record of what
was written.

```go
type AddResult struct {
    Repo         string             // "owner/repo" (normalized)
    Profiles     []string           // Profile names assigned, in order
    Engine       string             // Engine override ("" if none)
    Resolved     []ResolvedWorkflow // Workflow list (from ResolveRepoWorkflows)
    Warnings     []string           // Non-fatal warnings (no-op exclude, shadowed extra, zero-resolved)
    WroteLocal   bool               // true if this invocation created or rewrote fleet.local.json
    SynthesizedLocal bool           // true if fleet.local.json was synthesized from fleet.json-only baseline
    LocalPath    string             // Absolute path to the written/would-be-written file (for logging)
}
```

Notes:

- `Resolved` is the same `ResolvedWorkflow` slice type the deploy
  path uses; we deliberately do not introduce a parallel "preview
  workflow" type. Exercising the same resolver keeps dry-run
  preview and deploy behavior in lockstep.
- `Warnings` is populated for stderr rendering. Empty in the happy
  path. Never carries fatal errors — those are returned via
  `Add()`'s `error` return.
- `WroteLocal == false` in dry-run mode; `LocalPath` is still
  populated so the dry-run message can reference the target file.

### Supporting helper: `parseExtraWorkflowSpec(s string) (ExtraWorkflow, error)`

Pure function. Converts a single `--extra-workflow` flag value into
an `ExtraWorkflow`. See `research.md §7` for the full algorithm and
error cases.

### Supporting helper: `validateSlug(s string) (string, error)`

Pure function. Returns the normalized (lowercased, trimmed)
`owner/repo` form, or a descriptive error. See `research.md §6`.

### Supporting helper: `BuildMinimalLocalConfig(repo string, spec RepoSpec) *Config`

Constructs a `Config` with `Version = SchemaVersion`,
`Repos = map[string]RepoSpec{repo: spec}`, and everything else zero-
valued. Exists specifically to make the FR-015 "minimal local file"
contract directly testable (unit test asserts the resulting JSON
has exactly two top-level keys: `version` and `repos`).

## Modified types

None. `Config`, `RepoSpec`, `ExtraWorkflow`, `ResolvedWorkflow`,
and `Profile` (all in `internal/fleet/schema.go`) are consumed as-is.

## Existing types consumed (for reference)

### `Config` (`internal/fleet/schema.go:10-16`)

Already supports everything needed. `Add` mutates `Config.Repos` by
inserting the new key.

### `RepoSpec` (`internal/fleet/schema.go:56-62`)

Fields populated by `Add`:

| Field | Source |
|-------|--------|
| `Profiles` | `AddOptions.Profiles` verbatim |
| `Engine` | `AddOptions.Engine` (empty string omitted via `omitempty`) |
| `ExtraWorkflows` | Result of parsing each `AddOptions.ExtraWorkflows` entry |
| `ExcludeFromProfiles` | `AddOptions.Excludes` verbatim |
| `Overrides` | Always nil in v1 (out of scope per Assumptions) |

### `ExtraWorkflow` (`internal/fleet/schema.go:66-71`)

All four fields (`Name`, `Source`, `Ref`, `Path`) populated by
`parseExtraWorkflowSpec`.

## Lifecycle (dry-run → apply)

```text
cmd/add.go parses flags
    │
    ├── validateSlug(args[0])                          ← FR-004
    ├── check Apply + Confirmed (TTY/--yes logic)      ← FR-003
    └── build AddOptions
            │
            ▼
    fleet.Add(cfg, opts) in internal/fleet/add.go
            │
            ├── parse each --extra-workflow             ← FR-008
            ├── validate profiles exist in cfg          ← FR-011
            ├── validate engine in EngineSecrets        ← FR-006
            ├── check no duplicate in cfg.Repos         ← FR-010
            ├── build candidate RepoSpec
            ├── set cfg.Repos[repo] = candidate
            │    (IN-MEMORY ONLY — used for resolver call)
            ├── cfg.ResolveRepoWorkflows(repo)          ← FR-012
            ├── collect warnings (no-op excludes,       ← research.md §3
            │   shadowed extras, zero-resolved)
            └── build AddResult
                    │
                    ▼
            if opts.Apply:
                BuildMinimalLocalConfig(repo, candidate) ← FR-015
                SaveLocalConfig(opts.Dir, minimal)       ← FR-014 + FR-016
                set WroteLocal = true
            return result, nil
                    │
                    ▼
    cmd/add.go renders AddResult
        stderr: header + warnings + next-step hint     ← FR-013
        stdout: workflow list (one "- <name>" per line)
```

**Important invariant**: The in-memory `cfg.Repos[repo] = candidate`
mutation is temporary — it exists only so `ResolveRepoWorkflows`
can treat the candidate as though it were part of the merged view.
When `--apply` writes, it writes a **minimal** `Config` that does
NOT carry the rest of `cfg`'s contents (FR-015). The minimal file
is constructed fresh via `BuildMinimalLocalConfig`, not derived
from the mutated `cfg`.

## State transitions

`add` is stateless per-invocation. Between invocations, the state
machine is:

```text
(no fleet.local.json, fleet.json present)
     │
     │ add owner/repo --profile X --apply --yes
     ▼
(fleet.local.json with {version, repos:{owner/repo: ...}})
     │
     │ add owner2/repo2 --profile Y --apply --yes
     ▼
(fleet.local.json with {version, repos:{owner/repo, owner2/repo2}})
```

No intermediate "partially-written" state is possible because
writes are atomic (temp file + rename, per FR-016).

Fatal-error transitions exit non-zero before any mutation. The
only way `fleet.local.json` changes is the successful path above.
