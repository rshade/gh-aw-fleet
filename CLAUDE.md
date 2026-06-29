@AGENTS.md

## Common commands

```bash
go run . overview  # Read-only drift, run-health, no-op, and AIC/cost dashboard.
```

## Active Technologies
- Go 1.26.4 local development toolchain + stdlib (`encoding/json`, `os`, `path/filepath`, `time`). **No new direct dependencies** — constitution §Third-Party Dependencies. (011-fleet-manifest)
- JSON files written into target-repo clones (`os.WriteFile`); read in `status` via `ghAPIRaw` (existing seam). (011-fleet-manifest)
- Go 1.26.4 local toolchain (module declares `go 1.26.4` compatibility) + `gopkg.in/yaml.v3` (existing approved direct dep — YAML parse, already used by `internal/fleet/frontmatter`); stdlib `os`, `path/filepath`, `strings`. **No new third-party dependencies.** (013-dependabot-conflict-scanner)
- N/A — pure read of a probed config already present in the work-dir clone; no on-disk scanner state, no cache, no baseline. Findings are transient on `DeployResult`/`SyncResult`/`UpgradeResult`. (013-dependabot-conflict-scanner)
- Go 1.26.4 local toolchain + existing `github.com/spf13/cobra` and `github.com/rs/zerolog`; stdlib `encoding/json`, `errors`, `fmt`, `math`, `sort`, `sync`, `time`. No new direct dependencies. (018-overview-subcommand)
- N/A — pure read dashboard; no persisted state or cache. Output is transient to stdout/stderr or the standard JSON envelope. (018-overview-subcommand)

**Language**: Go 1.26.4 for the local development gate; `go.mod` currently declares module compatibility at `go 1.26.4`.

## Architecture big-picture

`pkg/fleet` is the module's first public surface and the single canonical home of the `fleet.json` wire contract. `internal/fleet` aliases those config types while keeping load, merge, resolve, and deploy logic internal.

`overview` joins drift from `Status()` with run health, no-op counts, AIC, and
cost from the shared `collectRepoRuns` logs fan-out. It defaults to trailing 7
days, gates exit code on drift only, and clamps aggregate `safeoutputs/noop`
usage to successful runs.

Per-slice dependency and storage deltas:

- (006-layer1-security-scanner) Storage: N/A — pure read calls against files already in the work-dir clone. No on-disk scanner state, no cache, no baseline file. Findings are transient on result structs (`DeployResult`/`SyncResult`/`UpgradeResult`).
- (007-billing-metadata-fields) Deps: no new direct dependencies introduced by this slice (uses `cobra` v1.10.2, `zerolog` v1.35.1, `yaml.v3` v3.0.1, `encoding/json` — all pre-existing). Storage: N/A — pure read/parse of `fleet.json` / `fleet.local.json`. Round-trip serialization remains byte-identical (SC-006).
- (issue #73) Deps: retains `github.com/tailscale/hujson` (BSD-3-Clause, zero transitive deps) as an approved direct dependency under Constitution §Third-Party Dependencies. After slice 016, `internal/fleet/load.go` reads and patches through `github.com/rshade/ax-go/config`; hujson remains direct for the `Add` AST append path and the Renovate scanner. `.hujson` extension probing remains unchanged and both extensions present is rejected.
- (016-ax-go-foundation) Deps: adds `github.com/rshade/ax-go v0.2.0` as an approved direct dependency under Constitution v1.2.0 §Third-Party Dependencies and raises the module directive to `go 1.26.4`. Import only `github.com/rshade/ax-go/config`, `github.com/rshade/ax-go/schema`, and the transitive stdlib-only `contract` package; never import root `package ax`, which would pull OTel/gRPC/protobuf into the build. Storage: N/A — config file shapes are unchanged. `__schema` is additive and hidden; its `error_envelope` block is a forward declaration only, so consuming agents must not parse today's non-`__schema` errors as ax envelopes until the deferred error-envelope phase lands. Follow-up phases: error-envelope adoption, `--output json` payload alignment, logger convergence, and idempotency/mode/dry-run context.
- (012-renovate-conflict-scanner) Deps: no new direct dependencies — reuses `github.com/tailscale/hujson` for JWCC-tolerant Renovate-config parsing. Storage: N/A — pure read of a probed Renovate config in the work-dir clone; findings transient on `DeployResult`/`SyncResult`/`UpgradeResult`. Adds the `security_renovate` diag code (additive — no `cmd.SchemaVersion` / `fleet.SchemaVersion` bump).
- (013-dependabot-conflict-scanner) Deps: no new direct dependencies — reuses `gopkg.in/yaml.v3` for YAML parsing of the Dependabot config. Storage: N/A — pure read of a probed `.github/dependabot.yml`/`.yaml` in the work-dir clone; findings transient on `DeployResult`/`SyncResult`/`UpgradeResult`. Adds the `security_dependabot` diag code (additive — no `cmd.SchemaVersion` / `fleet.SchemaVersion` bump).
- (018-overview-subcommand) Deps: no new direct dependencies — reuses cobra, zerolog, Status' fetcher seam, and the `gh aw logs` seams. Storage: N/A — read-only dashboard, no cache, no baseline. Adds a JSON payload under the existing envelope without bumping `cmd.SchemaVersion`.

## Recent Changes
- 016-ax-go-foundation: adopted `github.com/rshade/ax-go v0.2.0` the constitutional way; `internal/fleet/load.go` now uses import-isolated `config.ParseFile` / `config.Patch`, `cmd` exposes a hidden additive `__schema` command built on `schema.BuildSchema`/`schema.BuildMCPSchema` (mirroring `schema.NewSchemaCommand`, with MCP positional-argument augmentation), and `go.mod` now declares `go 1.26.4`. Import boundary is `config`/`schema`/`contract` only; no root `package ax`.
- 013-dependabot-conflict-scanner: fifth advisory scanner in the slice-006 security registry — sibling of the Renovate scanner but with **one** conflict rule (Dependabot ignores by dependency name only, with no `*.lock.yml` file-glob analog): a `LOW` finding per `github-actions` update entry that does not ignore the gh-aw-actions family, whose remedy carries the name-only caveat (FR-004); `INFO` on unparseable YAML; new `security_dependabot` diag code; surfaces on `deploy`/`sync`/`upgrade` via the existing finding pipeline.
- 012-renovate-conflict-scanner: fourth advisory scanner in the slice-006 security registry — `LOW` findings when a repo's Renovate config lacks the gh-aw-actions disable rule or the `.github/workflows/*.lock.yml` exclusion; `INFO` on unparseable config; new `security_renovate` diag code; surfaces on `deploy`/`sync`/`upgrade` via the existing finding pipeline.
- issue-73 (hujson): inline `//`/`/* */` comments and trailing commas accepted in `fleet.json` / `fleet.local.json` / `templates.json` / `profiles/default.json`; `gh-aw-fleet add` now appends to existing `fleet.local.json` instead of overwriting it.
- 006-layer1-security-scanner: Added Go support under the module's compatibility directive.

<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan:
[specs/018-overview-subcommand/plan.md](./specs/018-overview-subcommand/plan.md)
<!-- SPECKIT END -->
