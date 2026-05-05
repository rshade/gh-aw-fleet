# Implementation Plan: Layer 1 Security Scanner (Secrets + Compiled-YAML + Fleet-Structural Rules)

**Branch**: `006-layer1-security-scanner` | **Date**: 2026-04-30 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/006-layer1-security-scanner/spec.md`

## Summary

Add a v1 security-scanner layer between "upstream merged markdown" and "fleet commits markdown into a target repo." Three detectors run on every `deploy`/`sync`/`upgrade` after `gh aw add` produces source `.md` and compiled `.lock.yml` in the work-dir clone, and before the commit gate: (1) embedded-credential detection via `github.com/zricethezav/gitleaks/v8` against the source markdown, (2) frontmatter-structural rules implemented in-tree (six rules: `permissions.write-on-schedule`, `safe-outputs.draft-false`, `safe-outputs.missing-protected-files`, `engine.env.non-allowlist`, `repo-memory.main-branch`, `mcp.non-standard-server`), and (3) compiled-YAML linting via shellout to the optional `actionlint` binary against the `.lock.yml`. Findings are advisory in v1 — they never block the run — and surface on three established channels: stderr `zerolog.Warn` (one line per finding), JSON envelope `warnings[]` (one `Diagnostic` entry per finding), and a new `## Security Findings` section appended to the PR body when `--apply` is used.

The scanner core mirrors the existing `internal/fleet/diagnostics.go` pattern: a small typed value (`Finding`) that projects into the cross-surface `Diagnostic` shape. Three clarifications from the 2026-04-30 session bind the design: (a) credential-scanner matched literals are redacted in **both** stderr and PR body — symmetric, not asymmetric (FR-008a); (b) the `engine.env.non-allowlist` rule emits an INFO finding and skips itself when the workflow's engine is missing or unknown, mirroring FR-007/FR-017 (FR-018); (c) the v1 MCP allowlist is GitHub-hosted-only — `github.com`, `githubusercontent.com`, `raw.githubusercontent.com` — npm and other ecosystems are deliberately excluded as known typosquat / supply-chain channels (FR-019).

The new code is one new package (`internal/fleet/security/`) holding ~5 files (~500 LOC inclusive), one new top-level helper on `internal/fleet/deploy.go` to invoke the scanner once after `addResolvedWorkflows`, three new fields on `DeployResult` (paralleled on `SyncResult`/`UpgradeResult`), one new PR-body composer (`securityFindingsSection`), nine new diagnostic codes in `diagnostics.go` (seven rule codes plus `DiagSecurityActionlint` and `DiagSecurityFrontmatterParseError` for the helper paths), and table-driven tests with golden fixtures under `testdata/security/`. One new third-party dependency: `github.com/zricethezav/gitleaks/v8` (justified below). No new packages on `cmd/`. No `cmd.SchemaVersion` bump (the JSON envelope's `warnings[]` is open-ended by design — adding new diagnostic codes does not break the envelope contract).

## Technical Context

**Language/Version**: Go 1.25.8 (per `go.mod`).
**Primary Dependencies**:

- Existing: `github.com/spf13/cobra` v1.10.2 (CLI), `github.com/rs/zerolog` v1.x (stderr structured logging), `gopkg.in/yaml.v3` (frontmatter parsing — already in use via `internal/fleet/frontmatter.go`), `encoding/json` (stdlib, JSON envelope), `os/exec` (stdlib, `actionlint` shellout).
- **NEW**: `github.com/zricethezav/gitleaks/v8` (the canonical embedded-credential regex library; ~200 default rules; used by GitHub Advanced Security, trufflehog, and pre-commit-hooks). One new dep; justified in Constitution Check.
- External binary (optional, runtime-only): `actionlint` — shellout, not a build-time dep. Graceful degradation per FR-007: missing binary → one INFO finding, scanner skips, run continues.

**Storage**: N/A — pure read calls against files already in the work-dir clone. No on-disk scanner state, no cache, no baseline file. Findings are transient on result structs (`DeployResult`/`SyncResult`/`UpgradeResult`).

**Testing**: `go test ./...`. New table-driven unit tests in `internal/fleet/security/security_test.go` (one table per detector, plus a CollectFindings end-to-end table). Fixtures under `internal/fleet/security/testdata/security/`: `workflow-with-fake-secret.md`, `workflow-with-write-on-schedule.md`, `workflow-with-draft-false.md`, `workflow-with-engine-env-non-allowlist.md`, `workflow-with-repo-memory-main.md`, `workflow-with-mcp-npm-host.md`, `workflow-with-malformed-frontmatter.md`, `workflow-with-missing-engine.md`, `clean-agentics-workflow.md` (real `ci-doctor.md` content for SC-002). Integration test in `internal/fleet/security/security_integration_test.go` exercises `CollectFindings` end-to-end against all fixtures and asserts the expected severity counts. `actionlint` integration test uses `t.Skip` if binary missing-from-PATH (matches the FR-007 graceful-degradation contract — the test itself proves SC-005 by stripping PATH and re-running).

**Target Platform**: Linux/macOS/WSL — wherever the `gh-aw-fleet` Go binary already runs. No platform-specific code paths in the scanner; gitleaks is pure Go and cross-compiles cleanly. `actionlint` is detected via `exec.LookPath` so absence on any platform is a v1 graceful-skip, not an error.

**Project Type**: CLI tool (single Go module, `cmd/` + `internal/fleet/` layered as elsewhere). Adds one sub-package: `internal/fleet/security/`.

**Performance Goals**: SC-003 budgets total scanner overhead at <2s for a 10-workflow profile. Concretely: gitleaks `*Detector` cold-start is ~300–500ms (compile ~200 default regex rules); per-workflow `DetectBytes` on a 1–4 KB markdown file is sub-millisecond. Initialize the detector **once per run** (not per workflow) to keep the cold-start cost amortized. actionlint shellout per `.lock.yml` adds ~50–150ms (one process spawn each); 10 workflows ≈ 500ms–1.5s. Structural-rule evaluation parses already-decoded frontmatter (sub-ms per workflow). Comfortably under the 2s budget at 10 workflows; well under Constitution Principle IV's 5-min ceiling.

**Constraints**:

- Read-only against the workflow files (never modifies markdown, never modifies the lock YAML, never re-fetches upstream content).
- Symmetric secret redaction (FR-008a) — both stderr and PR-body messages identify only the rule and `<redacted>` placeholder; matched literal MUST NOT appear in either.
- Advisory-only in v1 (FR-006) — findings change zero exit-code behavior; no run is ever blocked by a finding. `--strict` is explicitly future work.
- One scanner's failure does not abort the run or other scanners (FR-016) — internal scanner errors are logged at `warn` and the run continues.
- Stable sort order: severity desc → file path asc → line number asc (FR-011).
- No new packages on `cmd/` — wiring lives entirely on `internal/fleet/deploy.go`/`sync.go`/`upgrade.go` and the existing `cmd/output.go` envelope path picks up new `Diagnostic` entries automatically via `warnings[]`.
- The `engine.env` allowlist tracks upstream `github/gh-aw` ADR-26919 Part 2 Normative Specification. ADR-26919 itself does **not** transcribe the allowlist; it specifies that conformant codemods MUST call `getSecretRequirementsForEngine(engine, includeSystemSecrets=false, includeOptional=false)`. The actual per-engine secret-name data lives in `pkg/constants/engine_constants.go` (`EngineOptions` table). Six engines: `claude`, `codex`, `copilot`, `gemini`, `opencode`, `crush`. Implementation pins to the specific upstream commit SHA of `engine_constants.go` (not the ADR file) in a comment and adds a regression test that reads a frozen JSON fixture under `testdata/security/adr-26919-allowlist.json` derived from that file's `EngineOptions.SecretName` + `AlternativeSecrets` per engine. Drift surfaces as test failure.

**Scale/Scope**: One scanner invocation per `Deploy`/`Sync`/`Upgrade` call. Each invocation processes the union of `.md` files added/skipped/failed during this run (≤20 workflows per repo today; ≤100 if a profile mass-deploys). Scanner is per-repo and not parallelized in v1 (Constitution Principle IV's parallelism guidance applies to *I/O-bound network* operations; scanner is local-disk + in-process, sub-2s already).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Compliance | Evidence |
|---|---|---|
| **I. Thin-Orchestrator Code Quality** | ✅ PASS WITH JUSTIFIED EXCEPTION | The credential scanner wraps `github.com/zricethezav/gitleaks/v8` (a library — same wrap-don't-reimplement principle as cobra/zerolog/yaml.v3); the YAML linter shells out to `actionlint` (canonical wrap pattern). The structural-rule scanner is **new in-tree logic** because it encodes fleet-specific knowledge that no upstream tool has — this is the exception. The exception is bounded: rules are table-driven (one entry per rule in a `[]Rule` slice in `structural.go`), each rule is ≤30 LOC of frontmatter walking, and the table itself documents what fleet-specific means here. Comments on each rule explain WHY (e.g. "write-on-schedule is the operational shape of a supply-chain compromise"). New file budget per Principle I: `security.go` ~120 LOC, `gitleaks.go` ~80 LOC, `actionlint.go` ~100 LOC, `structural.go` ~250 LOC, `security_test.go` ~400 LOC — all well under the 300-LOC-without-reason guideline. **One new third-party dep** (`gitleaks/v8`) — justified: re-implementing ~200 regex patterns from the gitleaks default ruleset would duplicate the canonical credential-detection corpus used by GitHub Advanced Security, trufflehog, and pre-commit-hooks; we'd inherit the maintenance burden indefinitely. The library is a stable choice: Apache-2.0, active maintenance, no transitive net-new system deps. |
| **II. Testing Standards** | ✅ PASS | `go build`/`go vet` clean enforced. `make ci` (fmt-check + vet + lint + test) is the ship gate. Real-world dry-run: the scanner is invoked on every `deploy --apply` against a real scratch clone — the constitution's "dry-run runs the actual `gh aw add`" requirement extends transparently to the scanner since it runs after `addResolvedWorkflows`. Unit tests are mandatory here (constitution permits; spec mandates via SC-001/SC-002/SC-005/SC-006). Table-driven coverage: one fixture per structural rule (positive + negative cases), one fixture per integration scenario (clean, secret, malformed, missing-engine, npm-MCP), and one ADR-26919 regression fixture. `actionlint` graceful-skip tested by stripping PATH in a subtest. |
| **III. User Experience Consistency** | ✅ PASS | Three-turn pattern preserved: dry-run still shows the plan + findings; `--apply` is still required for mutation; findings never block. The new stderr warnings flow through the same `zerolog.Warn` channel as existing preflight findings (`MissingSecret`, `ActionsDisabled`, `WorkflowTokenReadOnly`). The PR-body section uses the same `## ⚠`-headed composer idiom as `setupRequiredSection`, but lives under its own `## Security Findings` heading (FR-005's "stable header" contract). All three channels (stderr, JSON `warnings[]`, PR body) report the same finding set in the same sort order, mirroring the precedent in 005-actions-preflight. The conventional-commits scope of the *implementation* commit is `feat(security):` per the issue title (constitution scopes commits to `ci(workflows)` for *workflow-touching* commits — implementation of the scanner itself is feature code, not workflow code, so the issue title takes precedence). |
| **IV. Performance Requirements** | ✅ PASS | Scanner overhead is <2s on 10 workflows (SC-003), well under the 5-min ceiling. Gitleaks `*Detector` is initialized **once per run** to amortize the ~300–500ms regex-compile cost — per-workflow `NewDetector()` calls would add 300–500ms × N and miss the budget. Per-workflow scanner work is sub-ms (regex eval) + per-`.lock.yml` actionlint shellout (~100ms). No network calls; no cache (settings are stateless per run); no parallelization in v1 (single-shot for clarity; revisit if the scanner ever exceeds the budget). Constitution Principle IV's parallelism guidance applies to network I/O fanout — scanner is local-disk and in-process. |
| **Declarative Reconcile Invariants** | ✅ PASS | Reads-only against the work-dir clone. Does not invoke git, does not mutate `fleet.json`/`fleet.local.json`, does not bypass gpg signing, does not invoke `git add`/`commit`/`push`. The scanner is orthogonal to the fleet-state reconcile loop — it observes what the upstream-driven workflow content looks like and reports findings; it does not auto-remediate (out of scope per spec; remediation is a separate epic). |
| **Development Workflow** | ✅ PASS | One new dependency justified above (per "added abstractions, new dependencies require a one-line rationale" rule). Skills in `skills/` need a touch only on `fleet-deploy/SKILL.md` (one paragraph: "the deploy now emits security findings; review the `## Security Findings` section in the PR body before merging") — tracked in tasks.md, not in this plan. `CLAUDE.md` needs no architectural-invariant update because the scanner is feature work, not invariant-bearing infrastructure (the new "scan-before-commit" step is a phase in the deploy pipeline, not a new constitutional rule). |

**Result**: All gates pass. **No Complexity Tracking entries required.** The one exception (in-tree structural-rule logic) is bounded, documented, and table-driven — not a violation but a scoped non-orchestrator addition justified by the absence of any upstream tool encoding fleet-specific rules.

## Project Structure

### Documentation (this feature)

```text
specs/006-layer1-security-scanner/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── finding.md           # Finding type, severity model, projection to Diagnostic
│   ├── scanner-interface.md # Scanner contract; gitleaks/structural/actionlint adapter contracts
│   └── pr-body-section.md   # `## Security Findings` rendering contract (stable header, summary line, per-finding format)
├── checklists/
│   └── requirements.md  # Quality checklist (16/16 from /speckit.specify, post-clarify update)
├── spec.md              # Source spec (post-clarify, three Q&As recorded)
└── tasks.md             # Generated by /speckit.tasks (NOT this command)
```

### Source Code (repository root)

This feature touches an existing single-project Go CLI. **One new sub-package** (`internal/fleet/security/`), no new packages on `cmd/`. The structural pattern (one `cmd/{name}.go` per Cobra command + one `internal/fleet/{name}.go` per feature module) is preserved; security is large enough to warrant its own sub-package (5 files vs. one bloated `security.go` that would push past 600 LOC).

```text
cmd/
├── deploy.go                  # MODIFY (small) — emitDeployWarnings() emits one zerolog.Warn() per
│                              #                  finding from res.SecurityFindings;
│                              #                  emitDeployEnvelope() projects each finding into a
│                              #                  Diagnostic in warnings[].
├── deploy_test.go             # MODIFY (small) — printDeploy / envelope tests for security findings
├── sync.go                    # MODIFY (small) — same pattern as deploy.go
├── sync_test.go               # MODIFY (small)
├── upgrade.go                 # MODIFY (small) — same pattern as deploy.go
├── upgrade_test.go            # MODIFY (small)
└── (other files untouched)

internal/fleet/
├── deploy.go                  # MODIFY — DeployResult gains SecurityFindings []security.Finding;
│                              #          new line after addResolvedWorkflows():
│                              #            res.SecurityFindings = security.Run(ctx, res.CloneDir)
│                              #          (engine is read per-workflow from frontmatter inside
│                              #          structural.go — no fleet-level engine arg; FR-018)
│                              #          new prBody composer call: securityFindingsSection(res)
│                              #          appended after setupRequiredSection.
├── deploy_test.go             # MODIFY — TestSecurityFindingsSection (golden render),
│                              #          TestPRBodyAppendsSecurityFindings.
├── sync.go                    # MODIFY (parallel changes)
├── sync_test.go               # MODIFY
├── upgrade.go                 # MODIFY (parallel changes)
├── upgrade_test.go            # MODIFY
├── diagnostics.go             # MODIFY — add nine Diag* constants:
│                              #   DiagSecurityCredential, DiagSecurityWriteOnSchedule,
│                              #   DiagSecurityDraftFalse, DiagSecurityMissingProtectedFiles,
│                              #   DiagSecurityEngineEnvNonAllowlist,
│                              #   DiagSecurityRepoMemoryMain, DiagSecurityMCPNonStandardHost,
│                              #   DiagSecurityActionlint, DiagSecurityFrontmatterParseError.
├── frontmatter.go             # NO CHANGE — security/structural.go reuses ParseFrontmatter via import.
└── security/                  # NEW PACKAGE
    ├── doc.go                 # NEW (~25 LOC) — package godoc.
    ├── finding.go             # NEW (~120 LOC) — Finding, Severity, Scanner interface, Run() entry,
    │                          #                  CollectFindings (sort + project to Diagnostic[]),
    │                          #                  RenderForStderr, RenderForPRBody,
    │                          #                  securityFindingsSection composer.
    ├── gitleaks.go            # NEW (~80 LOC) — gitleaksScanner: NewDetector once,
    │                          #                  DetectBytes per workflow, redact match,
    │                          #                  map report.Finding → Finding (HIGH).
    ├── actionlint.go          # NEW (~100 LOC) — actionlintScanner: exec.LookPath; if missing,
    │                          #                  return one INFO Finding; else exec.Command on
    │                          #                  each .lock.yml with `--format '{{json .}}'`,
    │                          #                  parse JSON, map error→HIGH, warning→MEDIUM.
    ├── structural.go          # NEW (~280 LOC) — structuralScanner with []rule table:
    │                          #                  six rules (write-on-schedule, draft-false,
    │                          #                  missing-protected-files, engine.env.non-allowlist,
    │                          #                  repo-memory.main-branch, mcp.non-standard-host).
    │                          #                  Reuses fleet.ParseFrontmatter; engine for the
    │                          #                  engine.env.non-allowlist rule is resolved
    │                          #                  per-workflow from frontmatter (FR-018), not from
    │                          #                  a constructor parameter; allowlist in adr26919.go.
    ├── adr26919.go            # NEW (~80 LOC) — engine→allowlist port from upstream
    │                          #                 ADR-26919 Part 2 (frozen at upstream commit SHA).
    ├── render.go              # NEW (~60 LOC) — RenderForStderr (terse, plain text — no
    │                          #                 colorization in v1), RenderForPRBody (markdown
    │                          #                 with severity tally line + per-finding bullets).
    ├── security_test.go       # NEW (~500 LOC) — table-driven tests, one table per detector,
    │                          #                  + CollectFindings end-to-end table.
    ├── security_integration_test.go  # NEW (~120 LOC) — runs Run() against fixtures dir,
    │                                  #                 asserts severity counts; PATH-strip subtest.
    └── testdata/security/
        ├── workflow-with-fake-secret.md
        ├── workflow-with-write-on-schedule.md
        ├── workflow-with-draft-false.md
        ├── workflow-with-missing-protected-files.md
        ├── workflow-with-engine-env-non-allowlist.md
        ├── workflow-with-repo-memory-main.md
        ├── workflow-with-mcp-npm-host.md
        ├── workflow-with-malformed-frontmatter.md
        ├── workflow-with-missing-engine.md
        ├── clean-agentics-workflow.md           # real ci-doctor.md content (SC-002)
        ├── compiled-with-actionlint-error.lock.yml
        └── adr-26919-allowlist.json             # frozen upstream allowlist for regression test
```

**Structure Decision**: Single Go module (existing). Add one sub-package `internal/fleet/security/` (5 files + tests + testdata). No reorganization of existing packages. The choice to introduce a sub-package rather than five top-level `security_*.go` files in `internal/fleet/` is driven by: (a) the structural-rule table is large enough to merit isolation; (b) `gitleaks` and `actionlint` are external-tool adapters — they belong together as a cohort; (c) tests + testdata for a security feature are easier to find in their own directory; (d) the sub-package boundary makes import cycles impossible and lets the package's exported API (`Run`, `Finding`, `Severity`, `RenderForPRBody`) be the only surface other code depends on.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

Constitution Check passed with no unjustified violations. The two design choices that warrant note are:

| Choice | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| In-tree structural-rule logic (not a wrapped upstream tool) | No upstream encodes fleet-specific anti-patterns (write-on-schedule, draft-false, engine.env allowlist tied to ADR-26919). This is the unique value the fleet adds over generic GitHub Actions linters. | Wrapping a generic linter would miss every rule in this scope. Building a configurable rule engine (e.g. OPA/Rego) would add a heavyweight dependency for six rules, contradicting Constitution Principle I's "no new abstractions for hypothetical future requirements." Six rules in a flat `[]rule` table is the minimum that ships value. |
| New third-party dependency `github.com/zricethezav/gitleaks/v8` | The default gitleaks ruleset is the canonical credential-pattern corpus (~200 regex rules) used by GitHub Advanced Security, trufflehog, pre-commit-hooks. Re-implementing would duplicate this corpus and inherit indefinite maintenance. | Hand-rolled regex set rejected: 200 patterns × ongoing updates × new credential-format introductions = perpetual maintenance liability for zero unique value. Calling the gitleaks CLI rejected: per-invocation process spawn would balloon scanner overhead beyond SC-003's 2s budget at 10 workflows; the library API gives sub-ms per-workflow scanning after a single ~300ms cold-start. License (Apache-2.0) is compatible. |
