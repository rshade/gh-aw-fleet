@AGENTS.md

## Active Technologies
- Go 1.26.4 local development toolchain + stdlib (`encoding/json`, `os`, `path/filepath`, `time`). **No new direct dependencies** — constitution §Third-Party Dependencies. (011-fleet-manifest)
- JSON files written into target-repo clones (`os.WriteFile`); read in `status` via `ghAPIRaw` (existing seam). (011-fleet-manifest)
- Go 1.26.4 local toolchain (module declares `go 1.25.8` compatibility) + `gopkg.in/yaml.v3` (existing approved direct dep — YAML parse, already used by `internal/fleet/frontmatter`); stdlib `os`, `path/filepath`, `strings`. **No new third-party dependencies.** (013-dependabot-conflict-scanner)
- N/A — pure read of a probed config already present in the work-dir clone; no on-disk scanner state, no cache, no baseline. Findings are transient on `DeployResult`/`SyncResult`/`UpgradeResult`. (013-dependabot-conflict-scanner)

**Language**: Go 1.26.4 for the local development gate; `go.mod` currently declares module compatibility at `go 1.25.8`.

Per-slice dependency and storage deltas:

- (006-layer1-security-scanner) Storage: N/A — pure read calls against files already in the work-dir clone. No on-disk scanner state, no cache, no baseline file. Findings are transient on result structs (`DeployResult`/`SyncResult`/`UpgradeResult`).
- (007-billing-metadata-fields) Deps: no new direct dependencies introduced by this slice (uses `cobra` v1.10.2, `zerolog` v1.35.1, `yaml.v3` v3.0.1, `encoding/json` — all pre-existing). Storage: N/A — pure read/parse of `fleet.json` / `fleet.local.json`. Round-trip serialization remains byte-identical (SC-006).
- (issue #73) Deps: adds `github.com/tailscale/hujson` (BSD-3-Clause, zero transitive deps) as an approved direct dependency under Constitution v1.1.0 §Third-Party Dependencies, for comment-preserving reads/writes of fleet config files. Storage: N/A — read path runs `hujson.Standardize()` before `json.Unmarshal`; write path uses direct AST mutation for `Add` and RFC 6902 patches for `SaveTemplates`. `.hujson` extension probed first; both extensions present is rejected.
- (012-renovate-conflict-scanner) Deps: no new direct dependencies — reuses `github.com/tailscale/hujson` for JWCC-tolerant Renovate-config parsing. Storage: N/A — pure read of a probed Renovate config in the work-dir clone; findings transient on `DeployResult`/`SyncResult`/`UpgradeResult`. Adds the `security_renovate` diag code (additive — no `cmd.SchemaVersion` / `fleet.SchemaVersion` bump).
- (013-dependabot-conflict-scanner) Deps: no new direct dependencies — reuses `gopkg.in/yaml.v3` for YAML parsing of the Dependabot config. Storage: N/A — pure read of a probed `.github/dependabot.yml`/`.yaml` in the work-dir clone; findings transient on `DeployResult`/`SyncResult`/`UpgradeResult`. Adds the `security_dependabot` diag code (additive — no `cmd.SchemaVersion` / `fleet.SchemaVersion` bump).

## Recent Changes
- 013-dependabot-conflict-scanner: fifth advisory scanner in the slice-006 security registry — sibling of the Renovate scanner but with **one** conflict rule (Dependabot ignores by dependency name only, with no `*.lock.yml` file-glob analog): a `LOW` finding per `github-actions` update entry that does not ignore the gh-aw-actions family, whose remedy carries the name-only caveat (FR-004); `INFO` on unparseable YAML; new `security_dependabot` diag code; surfaces on `deploy`/`sync`/`upgrade` via the existing finding pipeline.
- 012-renovate-conflict-scanner: fourth advisory scanner in the slice-006 security registry — `LOW` findings when a repo's Renovate config lacks the gh-aw-actions disable rule or the `.github/workflows/*.lock.yml` exclusion; `INFO` on unparseable config; new `security_renovate` diag code; surfaces on `deploy`/`sync`/`upgrade` via the existing finding pipeline.
- issue-73 (hujson): inline `//`/`/* */` comments and trailing commas accepted in `fleet.json` / `fleet.local.json` / `templates.json` / `profiles/default.json`; `gh-aw-fleet add` now appends to existing `fleet.local.json` instead of overwriting it.
- 006-layer1-security-scanner: Added Go support under the module's compatibility directive.

<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan:
[specs/014-starlight-docs-site/plan.md](./specs/014-starlight-docs-site/plan.md)
<!-- SPECKIT END -->
