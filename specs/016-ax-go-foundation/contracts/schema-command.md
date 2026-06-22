# CLI Contract: `gh-aw-fleet __schema`

The one new external interface in phase 1. The config-IO swap has **no** external
contract change (internal mechanics; observable behavior is held constant by the
parity tests), so it has no contract document here.

## Command

```text
gh-aw-fleet __schema [--as ax|mcp]
```

- **Source**: `cmd/schema.go`'s `newSchemaCmd(root)` — a hidden command that
  **mirrors** `schema.NewSchemaCommand` (from `github.com/rshade/ax-go/schema`):
  same `--as ax|mcp` switch,
  `schema.BuildSchema(root, schema.WithSchemaVersion(toolVersion()))` for the ax
  tree, and `contract.NewError` on an invalid `--as`. It calls
  `schema.BuildSchema`/`schema.BuildMCPSchema` **directly** (rather than
  delegating to `NewSchemaCommand`) so the `mcp` path can augment each tool with
  its CLI positional arguments — which ax-go's flag-only reflection cannot
  derive. Wired in `cmd/root.go` after the eight subcommands; `Hidden = true`.
- **Purpose**: machine-readable discoverability of the full CLI surface for agents
  and the future control plane (#145/#146).

## Flags

| Flag | Type | Default | Values | Meaning |
|------|------|---------|--------|---------|
| `--as` | string | `ax` | `ax`, `mcp` | Output schema dialect: ax-native reflective tree, or MCP tools adapter. |

(Plus inherited persistent flags `--dir`, `--log-level`, `--log-format`,
`--output` — they do not affect `__schema` output content.)

## Output

- **Stream**: stdout only (FR-017). Minified JSON + trailing newline
  (`contract.WriteJSON`).
- **`--as ax`**: a `schema.Schema` document — `schema_version`, `tool`
  (`gh-aw-fleet`), `version`, `mode_detection`, `command` (recursive tree), and
  `error_envelope`. The `command` tree MUST enumerate all eight subcommands and
  the root persistent flags (FR-013). See `data-model.md` for field-level shape.
- **`--as mcp`**: a `schema.MCPSchema` document — `{ "tools": [ … ] }`.
- The `error_envelope` block is a **forward-declaration** of ax-go's standard
  error contract; phase 1 does not yet emit that envelope (FR-015).

## Exit codes & errors

| Condition | Behavior |
|-----------|----------|
| Success (`ax` or `mcp`) | JSON on stdout, exit 0. |
| Unknown `--as` value | ax-style validation error (`contract.NewError`, `validation_error`) + non-zero exit (`ExitValidation`). This is the only ax error-envelope emission in phase 1, contained to `__schema`'s own surface (Decision 5). |

## Invariants the contract MUST preserve

- **Additive**: no existing command's stdout/stderr/exit code or `--output json`
  envelope changes; `cmd.SchemaVersion` unchanged (FR-016, SC-004, SC-006).
- **Lazy reflection**: the tree is built at invocation, so `__schema` always
  mirrors the live command set — adding/removing a subcommand later needs no
  `__schema` change.
- **Stream separation**: data on stdout, logs on stderr (FR-017).

## Acceptance (maps to spec)

| Check | Spec ref |
|-------|----------|
| `__schema` (no flag) emits JSON enumerating root flags + all 8 subcommands | FR-012, FR-013, SC-004 |
| `__schema --as mcp` emits the MCP tools list | FR-014 |
| Every other command's output is byte-identical before/after | FR-016, SC-004 |
| `error_envelope` present as forward-declaration; documented | FR-015, FR-020 |
| Output on stdout only | FR-017 |
