@AGENTS.md

## Active Technologies
- Go 1.25.8 (per `go.mod`). (006-layer1-security-scanner)
- N/A — pure read calls against files already in the work-dir clone. No on-disk scanner state, no cache, no baseline file. Findings are transient on result structs (`DeployResult`/`SyncResult`/`UpgradeResult`). (006-layer1-security-scanner)

## Recent Changes
- 006-layer1-security-scanner: Added Go 1.25.8 (per `go.mod`).
