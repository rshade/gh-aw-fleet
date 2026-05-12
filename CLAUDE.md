@AGENTS.md

## Active Technologies
- Go 1.25.8 (per `go.mod`). (006-layer1-security-scanner)
- N/A — pure read calls against files already in the work-dir clone. No on-disk scanner state, no cache, no baseline file. Findings are transient on result structs (`DeployResult`/`SyncResult`/`UpgradeResult`). (006-layer1-security-scanner)
- Go 1.25.8 (per `go.mod`). + `github.com/spf13/cobra` v1.10.2 (CLI), `github.com/rs/zerolog` v1.35.1 (stderr structured logging), `gopkg.in/yaml.v3` v3.0.1 (frontmatter parsing — unchanged on this feature path), `encoding/json` (stdlib). **No new third-party dependencies** — within the approved set under Constitution v1.1.0 § Third-Party Dependencies. (007-billing-metadata-fields)
- N/A — pure read/parse of `fleet.json` / `fleet.local.json`. No persistent state outside the existing JSON files. Round-trip serialization must remain byte-identical (SC-006). (007-billing-metadata-fields)
- Go 1.25.8 (per `go.mod`). + `github.com/tailscale/hujson` (BSD-3-Clause, zero transitive deps) for comment-preserving reads/writes of fleet config files. Approved direct dependency under Constitution v1.1.0 §Third-Party Dependencies. (issue #73)
- N/A — read path runs `hujson.Standardize()` before `json.Unmarshal`; write path uses direct AST mutation for `Add` and RFC 6902 patches for `SaveTemplates`. `.hujson` extension probed first; both extensions present is rejected. (issue #73)

## Recent Changes
- issue-73 (hujson): inline `//`/`/* */` comments and trailing commas accepted in `fleet.json` / `fleet.local.json` / `templates.json` / `profiles/default.json`; `gh-aw-fleet add` now appends to existing `fleet.local.json` instead of overwriting it.
- 006-layer1-security-scanner: Added Go 1.25.8 (per `go.mod`).
