# Quickstart: Layer 1 Security Scanner

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-04-30

## Audience

This is the implementer's entry point. It assumes you have read [plan.md](./plan.md) and [research.md](./research.md), and that you intend to follow [contracts/finding.md](./contracts/finding.md), [contracts/scanner-interface.md](./contracts/scanner-interface.md), and [contracts/pr-body-section.md](./contracts/pr-body-section.md).

## TL;DR

1. Add one new sub-package: `internal/fleet/security/`.
2. Add nine new diagnostic codes in `internal/fleet/diagnostics.go` (seven rule codes plus `DiagSecurityActionlint` and `DiagSecurityFrontmatterParseError`).
3. Add `SecurityFindings []security.Finding` field on `DeployResult`/`SyncResult`/`UpgradeResult`.
4. Call `security.Run(ctx, res.CloneDir)` once after `addResolvedWorkflows` in each of `Deploy`/`Sync`/`Upgrade` — engine for the structural rule is read per-workflow from frontmatter (FR-018), not passed in.
5. Add `securityFindingsSection(*DeployResult) string` composer in `internal/fleet/deploy.go`; call it from `prBody` after `setupRequiredSection`.
6. In `cmd/deploy.go`/`sync.go`/`upgrade.go`: emit one `zerolog.Warn` per finding and project each finding into `Diagnostic` for the JSON envelope's `warnings[]`.
7. Tests + fixtures under `internal/fleet/security/testdata/security/`.
8. `make ci` must pass.

## Step-by-step

### Step 1 — Add the dependency

```bash
go get github.com/zricethezav/gitleaks/v8@latest
go mod tidy
```

Pin the version. Record it in `internal/fleet/security/gitleaks.go`'s package doc comment.

Verify the API:

```bash
go doc github.com/zricethezav/gitleaks/v8/detect
```

Confirm `NewDetector`, `DetectBytes`, and `report.Finding` shapes match what's in [research.md R1](./research.md#r1-gitleaks-v8-library-surface--embed-not-shell-out). If the API has drifted, update the adapter design before writing code.

### Step 2 — Create the package skeleton

```bash
mkdir -p internal/fleet/security/testdata/security
```

Create the files in this order (lets you compile early and often):

1. `internal/fleet/security/doc.go` — package comment.
2. `internal/fleet/security/finding.go` — `Severity`, `Finding`, `Scanner` interface, `Run` (scaffolded with TODO returns).
3. `internal/fleet/security/render.go` — `RenderForStderr`, `RenderForPRBody`.
4. `internal/fleet/security/structural.go` — `structuralScanner` with the six-rule table.
5. `internal/fleet/security/adr26919.go` — engine→allowlist map + the regression test fixture.
6. `internal/fleet/security/gitleaks.go` — gitleaks adapter.
7. `internal/fleet/security/actionlint.go` — actionlint adapter.

After each file, `go build ./...` should pass. `go vet ./...` should pass after each commit.

### Step 3 — Fixtures

Create these under `internal/fleet/security/testdata/security/` (per [plan.md](./plan.md) Project Structure):

| Fixture | Triggers |
|---|---|
| `workflow-with-fake-secret.md` | `gitleaks:aws-access-key` (use literal `AKIAIOSFODNN7EXAMPLE` — AWS docs example, NOT a real key) |
| `workflow-with-write-on-schedule.md` | `fleet.permissions.write-on-schedule` |
| `workflow-with-draft-false.md` | `fleet.safe-outputs.draft-false` |
| `workflow-with-missing-protected-files.md` | `fleet.safe-outputs.missing-protected-files` |
| `workflow-with-engine-env-non-allowlist.md` | `fleet.engine.env.non-allowlist` (HIGH form) |
| `workflow-with-missing-engine.md` | `fleet.engine.env.non-allowlist` (INFO form, FR-018) |
| `workflow-with-repo-memory-main.md` | `fleet.repo-memory.main-branch` |
| `workflow-with-mcp-npm-host.md` | `fleet.mcp.non-standard-server` |
| `workflow-with-malformed-frontmatter.md` | `fleet.frontmatter.parse-error` (INFO) |
| `clean-agentics-workflow.md` | Real agentics `ci-doctor.md` content (SC-002 — must produce zero findings) |
| `compiled-with-actionlint-error.lock.yml` | actionlint HIGH (deliberately malformed) |
| `adr-26919-allowlist.json` | Frozen upstream allowlist for regression test |

For each fixture, after writing it, run a quick `cat` to confirm the YAML/markdown is valid and the trigger pattern is present.

### Step 4 — Tests

Two test files:

- `internal/fleet/security/security_test.go` — table-driven, one table per detector + a `CollectFindings` end-to-end table. Each entry: fixture filename, expected RuleID, expected severity, expected line.
- `internal/fleet/security/security_integration_test.go` — runs `Run(ctx, "testdata/security")` against the fixtures dir and asserts the union of findings matches a golden expected slice. Includes a `t.Run("actionlint missing", …)` subtest that strips PATH and asserts SC-005 graceful-degradation behavior.

Run continuously while building:

```bash
go test ./internal/fleet/security/...
```

### Step 5 — Wire into deploy/sync/upgrade

In `internal/fleet/deploy.go`:

```go
// Existing line at deploy.go:209
addResolvedWorkflows(ctx, res, resolved, opts, engine)

// Existing lines at deploy.go:212–213
res.MissingSecret, res.SecretKeyURL = checkEngineSecret(ctx, repo, engine)
res.ActionsDisabled, res.WorkflowTokenReadOnly = checkActionsSettings(ctx, repo)

// NEW — add immediately after the existing checks, before the !opts.Apply guard
res.SecurityFindings = security.Run(ctx, res.CloneDir)
```

(No engine argument: `security.Run` walks `<cloneDir>/.github/workflows/*.md` and the structural scanner reads each workflow's `engine:` from its own frontmatter — FR-018.)

Add `import "github.com/rshade/gh-aw-fleet/internal/fleet/security"` at the top.

In `prBody` (deploy.go:814–820):

```go
func prBody(res *DeployResult, repo string, addedCount int) string {
    var b strings.Builder
    // ... existing header ...
    if section := setupRequiredSection(res); section != "" {
        b.WriteString(section)
    }
    if section := securityFindingsSection(res); section != "" {  // NEW
        b.WriteString(section)
    }
    // ... existing footer ...
    return b.String()
}
```

Mirror these changes in `internal/fleet/sync.go` and `internal/fleet/upgrade.go` at the equivalent integration points.

### Step 6 — Wire into command-layer warnings

In `cmd/deploy.go`'s `emitDeployWarnings` (or equivalent — check the existing function for `MissingSecret`/`ActionsDisabled` warnings):

```go
for _, f := range res.SecurityFindings {
    log.Warn().
        Str("rule_id", f.RuleID).
        Str("severity", f.Severity.String()).
        Str("file", f.File).
        Int("line", f.Line).
        Str("remedy", f.Remedy).
        Msg(f.Message)
}
```

In `emitDeployEnvelope` (or equivalent JSON-envelope path):

```go
for _, f := range res.SecurityFindings {
    envelope.Warnings = append(envelope.Warnings, f.ToDiagnostic())
}
```

Mirror in `cmd/sync.go` and `cmd/upgrade.go`.

### Step 7 — Diagnostic codes

In `internal/fleet/diagnostics.go`, append to the existing `const ( … )` block:

```go
const (
    // ... existing codes ...
    DiagSecurityCredential               = "security_credential"
    DiagSecurityWriteOnSchedule          = "security_write_on_schedule"
    DiagSecurityDraftFalse               = "security_draft_false"
    DiagSecurityMissingProtectedFiles    = "security_missing_protected_files"
    DiagSecurityEngineEnvNonAllowlist    = "security_engine_env_non_allowlist"
    DiagSecurityRepoMemoryMain           = "security_repo_memory_main"
    DiagSecurityMCPNonStandardHost       = "security_mcp_non_standard_host"
    DiagSecurityActionlint               = "security_actionlint"
    DiagSecurityFrontmatterParseError    = "security_frontmatter_parse_error"
)
```

(Nine codes total — see [contracts/scanner-interface.md](./contracts/scanner-interface.md) `diagCodeForRuleID`.)

### Step 8 — Local verification

Before claiming the task complete, run the full local gate:

```bash
make fmt          # apply gofmt in place
make lint         # golangci-lint — may exceed 5 minutes locally
make test         # full test suite
# or in one shot:
make ci
```

`make ci` must pass. Per CLAUDE.md: build+vet alone is insufficient; CI runs stricter checks.

### Step 9 — Real-world dry-run

Per Constitution Principle II, run the actual deploy against a scratch repo with at least one synthetic finding:

```bash
# Set up: a local clone with a fake secret in one workflow
go run . deploy <some-test-repo>

# Expected:
# - stderr emits the security finding zerolog warning
# - dry-run output prints normally
# - JSON output (with --output json) has the finding in warnings[]
# - run completes with exit 0 (advisory, not blocking)
```

If that all looks right, follow the three-turn pattern: confirm with the user, then `--apply` to a real test repo and inspect the opened PR for the `## Security Findings` section.

### Step 10 — Update SKILL.md

Update `skills/fleet-deploy/SKILL.md` with one paragraph: "The deploy pipeline now scans fetched workflow markdown for embedded credentials, fleet-structural anti-patterns, and (when actionlint is installed) compiled-YAML lint issues. Findings appear on stderr during dry-run and apply, and in a `## Security Findings` section in the opened PR body. Findings are advisory — they do not block the deploy. Review the section before merging the PR."

## Success criteria checklist (mirrors spec.md SCs)

- [ ] **SC-001**: Fixture set produces 100% detection of canonical anti-patterns.
- [ ] **SC-002**: `clean-agentics-workflow.md` produces zero findings.
- [ ] **SC-003**: 10-workflow scanner overhead < 2s on commodity hardware (manually benchmarked once at implementation time; recorded in PR description).
- [ ] **SC-004**: Stderr findings == JSON envelope warnings findings (asserted by integration test).
- [ ] **SC-005**: PATH-stripped run produces exactly one INFO finding for actionlint.
- [ ] **SC-006**: Two consecutive runs produce byte-identical sorted findings.
- [ ] **SC-007**: PR body contains all stderr findings (manually verified once on first --apply).

## Common pitfalls

- **Don't recreate the gitleaks Detector per workflow.** Cold start is 300–500ms × N — destroys SC-003. Build it once in `Run`, pass to the adapter.
- **Don't propagate `gleak.Secret` into `Finding.Message`.** FR-008a invariant. Test asserts redaction; failing this test is a security regression, not a stylistic bug.
- **Don't forget `omitempty` on the `SecurityFindings` JSON tag.** Backwards compatibility for the JSON envelope output when nil.
- **Don't run actionlint on `.md` files.** It's a YAML linter — point it at `.lock.yml` only.
- **Don't sort with `sort.Slice`.** Use `sort.SliceStable` (FR-011, SC-006).
- **Don't change the section heading text.** `## Security Findings` exact — see contracts/pr-body-section.md.
- **Don't use `gh aw add` to fetch test fixtures.** The fixtures are checked-in literal markdown; the scanner reads from disk. Tests use `testdata/security/` directly.

## When you're done

1. `make ci` passes.
2. PR description includes the SC-003 benchmark number you measured.
3. CHANGELOG is automatically generated by release-please from the commit message — write a `feat(security):` subject describing what landed (per CLAUDE.md: "the commit message IS the changelog entry").
4. Close issue #37, link to the parent epic #36.
