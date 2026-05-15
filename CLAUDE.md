@AGENTS.md

## Active Technologies

**Language**: Go 1.25.8 (per `go.mod`) across all slices below.

Per-slice dependency and storage deltas:

- (006-layer1-security-scanner) Storage: N/A — pure read calls against files already in the work-dir clone. No on-disk scanner state, no cache, no baseline file. Findings are transient on result structs (`DeployResult`/`SyncResult`/`UpgradeResult`).
- (007-billing-metadata-fields) Deps: no new direct dependencies introduced by this slice (uses `cobra` v1.10.2, `zerolog` v1.35.1, `yaml.v3` v3.0.1, `encoding/json` — all pre-existing). Storage: N/A — pure read/parse of `fleet.json` / `fleet.local.json`. Round-trip serialization remains byte-identical (SC-006).
- (issue #73) Deps: adds `github.com/tailscale/hujson` (BSD-3-Clause, zero transitive deps) as an approved direct dependency under Constitution v1.1.0 §Third-Party Dependencies, for comment-preserving reads/writes of fleet config files. Storage: N/A — read path runs `hujson.Standardize()` before `json.Unmarshal`; write path uses direct AST mutation for `Add` and RFC 6902 patches for `SaveTemplates`. `.hujson` extension probed first; both extensions present is rejected.

## Recent Changes
- issue-73 (hujson): inline `//`/`/* */` comments and trailing commas accepted in `fleet.json` / `fleet.local.json` / `templates.json` / `profiles/default.json`; `gh-aw-fleet add` now appends to existing `fleet.local.json` instead of overwriting it.
- 006-layer1-security-scanner: Added Go 1.25.8 (per `go.mod`).

<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan:
[specs/009-consumption-subcommand/plan.md](./specs/009-consumption-subcommand/plan.md)
<!-- SPECKIT END -->
