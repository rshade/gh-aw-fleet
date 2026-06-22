# Phase 1 Data Model: Adopt ax-go as the AX Foundation — Phase 1

## No new persistent entities

This feature introduces **no** new on-disk state, no new config types, and no
change to any existing entity:

- `fleet.json` / `fleet.local.json` / `templates.json` shapes are unchanged; the
  config-IO swap (`config.Parse` / `config.Patch`) changes the *parsing/patching
  primitive*, not the data. `fleet.SchemaVersion` is not bumped (FR-018).
- The `--output json` envelope (`cmd/output.go`, `cmd.SchemaVersion`) is untouched
  (FR-016). No new fields, no version bump.
- gh-aw-fleet declares no new Go struct types of its own. It *consumes* ax-go's
  `config.*` functions and ax-go's `schema.*` types; it does not redefine them.

The only net-new **data artifact** is the `__schema` discoverability output, whose
shape is owned by ax-go (package `schema`), not by this repo. It is documented
here because it is the contract agents and the future control plane will parse.

## Entity: `__schema` discoverability output (owned by `ax-go/schema`)

Emitted on stdout by `gh-aw-fleet __schema`. Two formats, selected by `--as`.

### `--as ax` (default) → `schema.Schema`

| Field | Type | Meaning |
|-------|------|---------|
| `schema_version` | string | ax-native schema SemVer — a fixed ax-go-owned constant (`schema.SchemaVersion`); **not** set by `WithSchemaVersion`. |
| `tool` | string | Root command name — `"gh-aw-fleet"` (from `root.Name()`). |
| `version` | string | Tool version from `runtime/debug.ReadBuildInfo`, `"dev"` fallback. Despite its name, `schema.WithSchemaVersion(v)` populates **this** field, not `schema_version` (Decision 4). |
| `mode_detection` | string | ax-go's mode-detection rule description (ax-go-owned constant). |
| `command` | `CommandSchema` | The reflected root command tree (see below). |
| `error_envelope` | `ErrorSchemaInfo` | ax-go's standard error-envelope description — a **forward-declaration** in phase 1 (FR-015); the tool does not yet emit this envelope. |

**`CommandSchema`** (recursive): `use`, `short`, `long`, `example`, `flags[]`,
`commands[]`. The root's `commands[]` MUST include all eight subcommands —
`list`, `status`, `add`, `template`, `deploy`, `sync`, `upgrade`, `consumption`
(FR-013) — each with its flags; the root carries persistent flags `--dir`,
`--log-level`, `--log-format`, `--output`.

**`FlagSchema`**: `name`, `shorthand`, `type`, `default`, `usage`, `required`.

**`ErrorSchemaInfo`**: `schema_version`, `required[]` (`error_code`, `message`,
`trace_id`, `tool`, `version`, `schema_version`), `optional[]` (`actionable_fix`,
`context`, `suggestions`). See the FR-015 forward-declaration caveat.

### `--as mcp` → `schema.MCPSchema`

| Field | Type | Meaning |
|-------|------|---------|
| `tools` | `MCPTool[]` | One entry per command, adapted to the MCP tools shape. |

**`MCPTool`**: `name`, `description`, `inputSchema` (a JSON-schema-shaped
`map[string]any`).

### Validation / invariants

- Output is valid JSON on **stdout** only (FR-017); minified + trailing newline
  (ax-go's `contract.WriteJSON`).
- `__schema` is registered `Hidden` (out of human `--help`) but fully invokable
  (Decision 5).
- An unknown `--as` value yields an ax-style validation error + non-zero exit —
  the single contained ax-error-envelope emission in phase 1 (Decision 5).
- The schema is reflected **lazily** at invocation, so it always mirrors the
  current command tree — there is no second hand-maintained source of truth to
  drift.

## Touched code surfaces (not data, but the change map)

| Surface | Change | Data effect |
|---------|--------|-------------|
| `internal/fleet/load.go` `loadConfigFile`, `LoadTemplates` | read via `config.Parse`/`ParseFile` | none — same `Config`/`Templates` values |
| `internal/fleet/load.go` `SaveTemplates` | patch via `config.Patch` + `atomicWrite` | none — same on-disk JSON (modulo canonical whitespace), comments preserved |
| `internal/fleet/load.go` `probeConfigPath`, `mergeConfigs`, version check | **unchanged** | none |
| `cmd/root.go` + new `cmd/schema.go` | add `__schema` | net-new discoverability artifact (above) |
