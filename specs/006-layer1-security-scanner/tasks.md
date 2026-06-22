---
description: "Task list for Layer 1 Security Scanner (issue #37, epic #36)"
---

# Tasks: Layer 1 Security Scanner (Secrets + Compiled-YAML + Fleet-Structural Rules)

**Input**: Design documents from `/specs/006-layer1-security-scanner/`
**Prerequisites**: plan.md (✅), spec.md (✅), research.md (✅), data-model.md (✅), contracts/finding.md (✅), contracts/scanner-interface.md (✅), contracts/pr-body-section.md (✅), quickstart.md (✅)

**Tests**: INCLUDED. The plan and quickstart explicitly require table-driven unit tests + an integration test, and SC-001/SC-002/SC-005/SC-006 are test-based success criteria.

**Organization**: Tasks are grouped by user story (US1→US4 from spec.md) so each story is independently implementable and testable. Foundational phase wires shared infrastructure (Severity/Finding types, Run scaffold, cmd-layer surfaces, DeployResult fields) so each US phase plugs in one scanner end-to-end.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Different files, no dependencies on incomplete tasks
- **[Story]**: Maps task to spec.md user story (US1=P1, US2=P2, US3=P3, US4=P4)
- All paths absolute from repo root `$GOPATH/src/github.com/rshade/gh-aw-fleet/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Add the one new third-party dependency, create the package skeleton, verify the upstream API surface.

- [X] T001 Add `github.com/zricethezav/gitleaks/v8` to `go.mod` via `go get github.com/zricethezav/gitleaks/v8@latest` then `go mod tidy`. Record the exact pinned tag (e.g. `v8.18.x`) in `go.mod`/`go.sum`.
- [X] T002 Verify gitleaks library API by running `go doc github.com/zricethezav/gitleaks/v8/detect` and confirm `NewDetector(cfg config.Config) *Detector` and `(*Detector).DetectBytes([]byte) []report.Finding` match research.md R1. Record the pinned version in a comment to use in `internal/fleet/security/gitleaks.go`'s package doc later.
- [X] T003 Create directory tree `internal/fleet/security/testdata/security/` (with intermediate directories).
- [X] T004 [P] Create `internal/fleet/security/doc.go` with the package comment: one `// Package security` block (1–3 sentences describing the v1 scanner layer's scope and entry point `Run`), per AGENTS.md "Code self-documentation."

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core types, the scanner orchestrator scaffold, the projection to the JSON envelope's `Diagnostic` shape, and the cross-cutting wire-in on `DeployResult`/`SyncResult`/`UpgradeResult` and on `cmd/deploy.go`/`sync.go`/`upgrade.go`. After this phase, `Run` returns nil (no scanners yet) but every surface that *would* show findings is wired and working.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

### Diagnostic codes & types

- [X] T005 Append nine new diagnostic-code constants to `internal/fleet/diagnostics.go` (`DiagSecurityCredential`, `DiagSecurityWriteOnSchedule`, `DiagSecurityDraftFalse`, `DiagSecurityMissingProtectedFiles`, `DiagSecurityEngineEnvNonAllowlist`, `DiagSecurityRepoMemoryMain`, `DiagSecurityMCPNonStandardHost`, `DiagSecurityActionlint`, `DiagSecurityFrontmatterParseError`) with godoc comments per quickstart Step 7 and contracts/scanner-interface.md.
- [X] T006 [P] Create `internal/fleet/security/finding.go` with: `Severity` typed int + `SeverityInfo`/`SeverityLow`/`SeverityMedium`/`SeverityHigh` constants, `(Severity).String()` returning `"INFO"`/`"LOW"`/`"MEDIUM"`/`"HIGH"`, `Finding` struct with all six fields (RuleID, Severity, File, Line, Message, Remedy), `Scanner` interface with the `Scan(ctx, cloneDir) []Finding` method (no engine parameter — engine is per-workflow, read from frontmatter). Per contracts/finding.md.
- [X] T007 In `internal/fleet/security/finding.go`, add `Run(ctx context.Context, cloneDir string) []Finding` scaffold that: (a) builds an empty `[]Scanner` slice (no scanners yet), (b) iterates calling each scanner's `Scan` (no-op now), (c) sorts the combined slice via `sort.SliceStable` per contracts/finding.md (severity desc → file asc → line asc), (d) returns the sorted slice. Wrap each `Scan` call in `defer/recover` per contracts/scanner-interface.md to convert panics into a single INFO finding (`fleet.scanner.panic`) instead of aborting Run.
- [X] T008 In `internal/fleet/security/finding.go`, add `(Finding).ToDiagnostic() fleet.Diagnostic` method per contracts/finding.md. Import `fleet` package. Use `diagCodeForRuleID` helper (T009).
- [X] T009 In `internal/fleet/security/finding.go`, add unexported `diagCodeForRuleID(ruleID string) string` helper using switch/case per contracts/scanner-interface.md (gitleaks: → DiagSecurityCredential, etc., default → fleet.DiagHint).

### Rendering

- [X] T010 [P] Create `internal/fleet/security/render.go` with `RenderForStderr([]Finding) string` (one line per finding: `[SEVERITY] rule_id  file:line  message`; `Line==0` → render only `file`; `File=="" && Line==0` → render slot as `-`). Empty string on empty input. Per contracts/scanner-interface.md.
- [X] T011 In `internal/fleet/security/render.go`, add `RenderForPRBody([]Finding) string`: renders the `**Summary**: …` tally line + per-finding bullets in `- **<SEVERITY>** \`rule_id\` — \`file:line\` — message — remedy` form (em-dashes U+2014). Severity tally omits zero counts; tally order HIGH, MEDIUM, LOW, INFO. Empty string on empty input. Per contracts/pr-body-section.md.

### Result-struct extension and orchestrator integration

- [X] T012 [P] In `internal/fleet/deploy.go`, add `SecurityFindings []security.Finding \`json:"security_findings,omitempty"\`` field on `DeployResult` with godoc per data-model.md Entity 5. Add `import "github.com/rshade/gh-aw-fleet/internal/fleet/security"`.
- [X] T013 [P] In `internal/fleet/sync.go`, add the same `SecurityFindings` field on `SyncResult` with the same godoc + import.
- [X] T014 [P] In `internal/fleet/upgrade.go`, add the same `SecurityFindings` field on `UpgradeResult` with the same godoc + import.
- [X] T015 In `internal/fleet/deploy.go`, insert `res.SecurityFindings = security.Run(ctx, res.CloneDir)` immediately after the `checkActionsSettings`/`checkEngineSecret` calls (~line 213) and BEFORE the `if !opts.Apply { return res, nil }` guard, per research.md R4 and quickstart Step 5. (No engine argument: the structural scanner reads `engine:` per-workflow from frontmatter — FR-018.)
- [X] T016 In `internal/fleet/sync.go`, insert the parallel `security.Run(ctx, res.CloneDir)` call at the equivalent post-add, pre-commit position.
- [X] T017 In `internal/fleet/upgrade.go`, insert the parallel `security.Run(ctx, res.CloneDir)` call at the equivalent post-add, pre-commit position.

### Cmd-layer wiring (stderr + JSON envelope)

- [X] T018 In `cmd/deploy.go`'s `emitDeployWarnings` (or equivalent — locate by existing `MissingSecret`/`ActionsDisabled` warning emission), iterate `res.SecurityFindings` and emit one `log.Warn().Str("rule_id", …).Str("severity", …).Str("file", …).Int("line", …).Str("remedy", …).Msg(f.Message)` per finding. Per quickstart Step 6.
- [X] T019 In `cmd/deploy.go`'s `emitDeployEnvelope` (or equivalent JSON-envelope path), append `f.ToDiagnostic()` for each finding to `envelope.Warnings`.
- [X] T020 [P] In `cmd/sync.go`, mirror T018+T019 for the sync command.
- [X] T021 [P] In `cmd/upgrade.go`, mirror T018+T019 for the upgrade command.

### Foundational tests

- [X] T022 [P] Create `internal/fleet/security/finding_test.go` with: `TestSeverityString` (each constant → expected uppercase string), `TestFindingSortStability` (scrambled input → expected sorted output, asserts `sort.SliceStable` semantics for equal-severity-equal-file findings), `TestToDiagnosticShape` (all six fields project into `Diagnostic.Fields` with expected keys/values; `Code` matches the rule-ID prefix mapping).
- [X] T023 [P] In `internal/fleet/security/render_test.go`, add tests for `RenderForStderr` (empty slice → "", multi-finding → exact line-by-line golden) and for the file-slot rendering corners (`File != ""` with `Line == 0` → no `:0` suffix; `File == "" && Line == 0` → `-`).
- [X] T023a [P] In `internal/fleet/security/finding_test.go`, add `TestRun_ScannerPanicDoesNotAbortRun` (FR-016 acceptance): inject a stub `Scanner` whose `Scan` calls `panic("boom")` ahead of (or alongside) a stub that returns one HIGH finding. Run the orchestrator (use a test-only entry point or seam — e.g. an unexported `runWithScanners(scanners, ...)` helper called by `Run` — added in this task). Assert: (a) the run does NOT panic, (b) exactly one INFO Finding with `RuleID == "fleet.scanner.panic"` (or equivalent rule from T007) is present, (c) the HIGH finding from the surviving scanner is also present. Without this test the defer/recover scaffolding in T007 ships untested.

**Checkpoint**: `go build ./...` and `go vet ./...` pass. `make ci` passes. `Run` returns nil. The cmd-layer surfaces are wired but emit nothing because no scanner is registered yet. User story phases can now proceed in parallel.

---

## Phase 3: User Story 1 - Catch embedded secrets in upstream markdown before commit (Priority: P1) 🎯 MVP

**Goal**: Scan upstream-fetched workflow markdown for embedded credentials (AWS keys, API tokens, private keys, etc.) using the gitleaks default ruleset. Findings are HIGH severity, redacted (no matched literal in message), and surface on stderr + JSON envelope. Deploy continues regardless.

**Independent Test**: With `make ci` green, run `deploy` (dry-run) against a fixture profile pulling `testdata/security/workflow-with-fake-secret.md`. Confirm exactly one HIGH finding emitted on stderr identifying the file, line, rule ID `gitleaks:aws-access-key`, and remediation, AND the dry-run completes normally. Run again against `clean-agentics-workflow.md` — expect zero findings.

### Tests for User Story 1 (write FIRST, ensure they FAIL before implementation) ⚠️

- [X] T024 [P] [US1] Create fixture `internal/fleet/security/testdata/security/workflow-with-fake-secret.md` with valid frontmatter + body containing the literal `AKIAIOSFODNN7EXAMPLE` (AWS docs example, NOT a real key). Per quickstart Step 3.
- [X] T025 [P] [US1] Create fixture `internal/fleet/security/testdata/security/clean-agentics-workflow.md` by copying real `agentics/main` `ci-doctor.md` content verbatim (used for SC-002 zero-false-positive assertion).
- [X] T026 [P] [US1] In `internal/fleet/security/security_test.go`, add `TestGitleaksScanner_FakeSecret` table entry asserting exactly one Finding with `Severity == SeverityHigh`, `RuleID` starting `"gitleaks:"`, `File == ".github/workflows/workflow-with-fake-secret.md"` (relative), `Line` matching the source line, `Remedy` non-empty.
- [X] T027 [P] [US1] Add `TestGitleaksScanner_RedactionEnforced` subtest: assert `!strings.Contains(finding.Message, "AKIAIOSFODNN7EXAMPLE")` AND `strings.Contains(finding.Message, "<redacted>")`. FR-008a invariant; failing this is a security regression, not stylistic.
- [X] T028 [P] [US1] Add `TestGitleaksScanner_CleanAgenticsWorkflow` asserting zero findings against `clean-agentics-workflow.md` (SC-002).

### Implementation for User Story 1

- [X] T029 [US1] Create `internal/fleet/security/gitleaks.go`: package-doc cites the pinned gitleaks tag (T002) + ADR for FR-008a redaction. Define `gitleaksScanner struct { detector *detect.Detector }`. Implement `newGitleaksScanner() *gitleaksScanner` that constructs `*detect.Detector` once via `detect.NewDetectorDefaultConfig()` (or pinned-API equivalent verified in T002). Per research.md R1 — constructor cost amortized, never recreate per workflow.
- [X] T030 [US1] In `internal/fleet/security/gitleaks.go`, implement `(*gitleaksScanner).Scan(ctx, cloneDir) []Finding`: walk `<cloneDir>/.github/workflows/*.md`, for each file `os.ReadFile` → `s.detector.DetectBytes(content)`; for each `report.Finding` produced, construct one `security.Finding` per the contracts/finding.md gitleaks-adapter shape. **MUST NOT** read or propagate `gleak.Secret` — message uses `f.Description + " (<redacted>)"`. `File` is relative-to-cloneDir (strip prefix).
- [X] T031 [US1] In `internal/fleet/security/finding.go`'s `Run`, register `newGitleaksScanner()` as the FIRST entry in the scanner list (per contracts/scanner-interface.md scanner ordering: gitleaks, structural, actionlint).

**Checkpoint**: User Story 1 is fully functional. The gitleaks adapter detects fake AWS keys end-to-end through stderr and JSON envelope. The fleet's primary security value (FR-001 + the spec's "highest-impact failure mode") ships at this checkpoint.

---

## Phase 4: User Story 2 - Catch fleet-structural anti-patterns in workflow frontmatter (Priority: P2)

**Goal**: Evaluate each workflow's YAML frontmatter against the six fleet-specific structural rules (write-on-schedule, draft-false, missing-protected-files, engine.env-non-allowlist, repo-memory-main, mcp-non-standard-host). Each match emits a Finding with the rule's published severity. Malformed frontmatter and unknown engine emit single INFO findings (graceful skip).

**Independent Test**: Run `deploy` (dry-run) against a fixture set with one workflow per anti-pattern + `clean-agentics-workflow.md`. Confirm each rule fires exactly once with the expected severity / file / line / rule ID, and the clean workflow produces zero structural findings.

### Tests for User Story 2 (write FIRST, ensure they FAIL before implementation) ⚠️

- [X] T032 [P] [US2] Create fixture `internal/fleet/security/testdata/security/workflow-with-write-on-schedule.md` (frontmatter declaring `on: schedule` + `permissions: contents: write`).
- [X] T033 [P] [US2] Create fixture `internal/fleet/security/testdata/security/workflow-with-draft-false.md` (frontmatter declaring `safe-outputs.create-pull-request.draft: false`).
- [X] T034 [P] [US2] Create fixture `internal/fleet/security/testdata/security/workflow-with-missing-protected-files.md` (frontmatter has `safe-outputs.create-pull-request:` block with NO `protected-files` key).
- [X] T035 [P] [US2] Create fixture `internal/fleet/security/testdata/security/workflow-with-engine-env-non-allowlist.md` (frontmatter declares `engine: claude` + `engine.env.MY_SECRET: ${{ secrets.MY_SECRET }}` where `MY_SECRET` is NOT in the claude allowlist).
- [X] T036 [P] [US2] Create fixture `internal/fleet/security/testdata/security/workflow-with-missing-engine.md` (frontmatter has `engine.env.SOMETHING: ${{ secrets.X }}` but NO `engine:` key) — triggers FR-018 INFO path.
- [X] T037 [P] [US2] Create fixture `internal/fleet/security/testdata/security/workflow-with-repo-memory-main.md` (frontmatter declares `repo-memory.branch-name: main`).
- [X] T038 [P] [US2] Create fixture `internal/fleet/security/testdata/security/workflow-with-mcp-npm-host.md` (frontmatter declares an MCP server entry referencing `npmjs.com` or similar non-allowlisted host).
- [X] T039 [P] [US2] Create fixture `internal/fleet/security/testdata/security/workflow-with-malformed-frontmatter.md` (frontmatter is syntactically broken YAML — e.g. unclosed string).
- [X] T040 [P] [US2] Create fixture `internal/fleet/security/testdata/security/adr-26919-allowlist.json` with the per-engine allowed-secret allowlist. ADR-26919 itself does NOT transcribe the table — it specifies that conformant codemods MUST call `getSecretRequirementsForEngine(engine, includeSystemSecrets=false, includeOptional=false)`. The actual data lives in `pkg/constants/engine_constants.go` (`EngineOptions` table) in upstream `github/gh-aw`. Six engines: `claude`, `codex`, `copilot`, `gemini`, `opencode`, `crush`. For each engine, record its `SecretName` plus `AlternativeSecrets` as a sorted set in JSON. Capture the current SHA of `pkg/constants/engine_constants.go` (NOT the ADR file) and record it in a top-level `_source_sha` field along with the path `pkg/constants/engine_constants.go`. As of 2026-04-30, the SHA is `b469d2e5bb4340b9ab2e1d93f1bfcaefbbf92109` and the values are: `claude:{ANTHROPIC_API_KEY}`, `codex:{OPENAI_API_KEY,CODEX_API_KEY}`, `copilot:{COPILOT_GITHUB_TOKEN}`, `gemini:{GEMINI_API_KEY}`, `opencode:{COPILOT_GITHUB_TOKEN,ANTHROPIC_API_KEY,GEMINI_API_KEY}`, `crush:{COPILOT_GITHUB_TOKEN,ANTHROPIC_API_KEY,GEMINI_API_KEY}` — verify against current upstream before committing.
- [X] T041 [P] [US2] In `internal/fleet/security/security_test.go`, add `TestStructuralScanner_WriteOnSchedule` asserting exactly one HIGH Finding with `RuleID == "fleet.permissions.write-on-schedule"`, expected file/line.
- [X] T042 [P] [US2] Add `TestStructuralScanner_DraftFalse` asserting one MEDIUM Finding `fleet.safe-outputs.draft-false`.
- [X] T043 [P] [US2] Add `TestStructuralScanner_MissingProtectedFiles` asserting one MEDIUM Finding `fleet.safe-outputs.missing-protected-files`.
- [X] T044 [P] [US2] Add `TestStructuralScanner_EngineEnvNonAllowlist` asserting one HIGH Finding `fleet.engine.env.non-allowlist` for the engine == claude branch (engine resolved from the fixture's frontmatter `engine: claude`, not from a parameter).
- [X] T045 [P] [US2] Add `TestStructuralScanner_MissingEngine_INFO` asserting one INFO Finding `fleet.engine.env.non-allowlist` per FR-018 — engine missing or unknown is detected from the workflow's frontmatter (no `engine:` key, or value not in {claude, codex, copilot, gemini, opencode, crush}).
- [X] T046 [P] [US2] Add `TestStructuralScanner_RepoMemoryMain` asserting one HIGH Finding `fleet.repo-memory.main-branch`.
- [X] T047 [P] [US2] Add `TestStructuralScanner_MCPNonStandardHost` asserting one HIGH Finding `fleet.mcp.non-standard-server`.
- [X] T048 [P] [US2] Add `TestStructuralScanner_MalformedFrontmatter_INFO` asserting one INFO Finding `fleet.frontmatter.parse-error`, AND that all OTHER scanners still run on that file (gitleaks scans the raw bytes regardless).
- [X] T049 [P] [US2] Add `TestStructuralScanner_CleanAgenticsWorkflow` asserting zero structural findings on `clean-agentics-workflow.md` (SC-002 contribution).
- [X] T050 [P] [US2] Add `TestADR26919AllowlistMatchesFixture` reading `testdata/security/adr-26919-allowlist.json` and asserting it byte-equals the in-code map from `adr26919.go` after marshal-roundtrip. Drift in either direction fails the test.

### Implementation for User Story 2

- [X] T051 [US2] Create `internal/fleet/security/adr26919.go`: package-doc cites the upstream commit SHA captured in T040 + path `pkg/constants/engine_constants.go` (NOT the ADR file — the ADR doesn't transcribe the table). Define unexported `adr26919Allowlist map[string]map[string]bool` keyed by the six engine IDs (`claude`, `codex`, `copilot`, `gemini`, `opencode`, `crush`). Populate from the JSON fixture (transcribed manually for v1; future automation deferred). `//nolint:gochecknoglobals // immutable allowlist table` per data-model.md Entity 7.
- [X] T052 [US2] Create `internal/fleet/security/structural.go`: define unexported `structuralScanner struct {}` + `newStructuralScanner() *structuralScanner` (no engine constructor parameter — engine is per-workflow per FR-018). Define `rule struct { ID string; Severity Severity; Description string; Remedy string; Eval func(fm map[string]any) []ruleHit }` and `ruleHit struct { Line int; Detail string }` per data-model.md Entity 4. The `engine.env.non-allowlist` rule's `Eval` reads `fm["engine"]` itself.
- [X] T053 [US2] In `structural.go`, implement the `Scan` method that: (a) walks `<cloneDir>/.github/workflows/*.md`, (b) calls existing `fleet.ParseFrontmatter` from `internal/fleet/frontmatter.go` (no duplication), (c) on parse error emits one INFO Finding `fleet.frontmatter.parse-error` per data-model.md / contracts/finding.md and continues, (d) on success iterates the rule table calling each rule's `Eval(fm)`, (e) translates each `ruleHit` into a `security.Finding` per contracts/finding.md.
- [X] T054 [US2] In `structural.go`, implement rule `fleet.permissions.write-on-schedule` (HIGH): fires when frontmatter has any `write` or `admin` permission scope AND `on:` includes `schedule` or `workflow_run`. Construct Finding per contracts/finding.md. Add comment explaining WHY (write-on-schedule is the operational shape of a supply-chain compromise) per AGENTS.md.
- [X] T055 [US2] In `structural.go`, implement rule `fleet.safe-outputs.draft-false` (MEDIUM): fires when frontmatter has `safe-outputs.create-pull-request.draft: false`.
- [X] T056 [US2] In `structural.go`, implement rule `fleet.safe-outputs.missing-protected-files` (MEDIUM): fires when `safe-outputs.create-pull-request` block exists but lacks a `protected-files` key.
- [X] T057 [US2] In `structural.go`, implement rule `fleet.engine.env.non-allowlist` with both branches. The rule's `Eval` reads `fm["engine"]` (per-workflow, per FR-018) — there is no fleet-level engine. HIGH when the workflow's `engine:` is in the allowlist map AND a referenced `${{ secrets.NAME }}` is NOT in that engine's allowed set. INFO (FR-018) when the workflow's `engine:` is missing or not in the map (one INFO per workflow that has any `engine.env` entry; rule is skipped). Looks up against `adr26919Allowlist` from T051.
- [X] T058 [US2] In `structural.go`, implement rule `fleet.repo-memory.main-branch` (HIGH): fires when `repo-memory.branch-name == "main"` or `"master"`.
- [X] T059 [US2] In `structural.go`, implement rule `fleet.mcp.non-standard-server` (HIGH): fires for any MCP server entry whose host is not in `{"github.com", "githubusercontent.com", "raw.githubusercontent.com"}` (FR-019). Hard-code the allowlist as a set; comment explains npm/typosquat rationale.
- [X] T060 [US2] In `internal/fleet/security/finding.go`'s `Run`, register `newStructuralScanner()` as the SECOND entry in the scanner list (after gitleaks). Order matters per contracts/scanner-interface.md.

**Checkpoint**: User Story 2 is fully functional. The structural scanner detects all six fleet-specific anti-patterns with the documented severities. SC-001 (canonical-anti-pattern detection) is now achievable on the fixture suite.

---

## Phase 5: User Story 3 - Surface findings in the opened PR body (Priority: P3)

**Goal**: When `--apply` opens a PR, the PR body includes a `## Security Findings` section with a severity-tallied summary line and per-finding bullets (markdown list, em-dash separated). The section is positioned AFTER `## ⚠ Setup required` and BEFORE the workflow list/footer. Section is OMITTED entirely when zero findings exist (FR-005). Stable header — downstream tooling can grep for it.

**Independent Test**: Compose a `DeployResult` with synthetic `SecurityFindings` (HIGH + MEDIUM + INFO) and a non-empty `MissingSecret`; call `prBody(...)`. Confirm the returned string contains `## ⚠ Setup required` BEFORE `## Security Findings`, the summary line tallies correctly, the bullets sort by severity-desc / file-asc / line-asc. Then call with `SecurityFindings: nil` — confirm no `## Security Findings` heading anywhere.

### Tests for User Story 3 (write FIRST, ensure they FAIL before implementation) ⚠️

- [X] T061 [P] [US3] In `internal/fleet/deploy_test.go`, add `TestSecurityFindingsSection_NoFindings` asserting `securityFindingsSection(&DeployResult{SecurityFindings: nil}) == ""`.
- [X] T062 [P] [US3] Add `TestSecurityFindingsSection_SingleHigh`: one HIGH finding → exact golden render matching contracts/pr-body-section.md "Single HIGH finding (typical)" example, including the `## Security Findings\n\n` heading, the `**Summary**: 1 HIGH` line, the bullet, trailing newline.
- [X] T063 [P] [US3] Add `TestSecurityFindingsSection_MixedSeverities` for HIGH+HIGH+MEDIUM+INFO → assert summary `**Summary**: 2 HIGH, 1 MEDIUM, 1 INFO` (zero-count severities omitted), bullet order matches sort contract.
- [X] T064 [P] [US3] Add `TestSecurityFindingsSection_StableSort` passing scrambled-input findings → output order matches sort contract (severity desc → file asc → line asc) byte-identically.
- [X] T065 [P] [US3] Add `TestPRBodyAppendsSecurityFindings` constructing a `DeployResult` with both `MissingSecret` populated AND `SecurityFindings` populated → assert returned `prBody` string contains `## ⚠ Setup required` substring at an index BEFORE the `## Security Findings` substring (positional invariant per contracts/pr-body-section.md).

### Implementation for User Story 3

- [X] T066 [US3] Implement `RenderForPRBody` body in `internal/fleet/security/render.go` (T011 created the signature): summary line uses `**Summary**: ` prefix, severity tallies in HIGH→MEDIUM→LOW→INFO order omitting zeros, then a blank line, then per-finding bullets `- **<SEVERITY>** \`rule_id\` — \`file:line\` — message — remedy`. File-slot rules: `Line==0` and `File != ""` → `file` (no `:0`); `File==""` → empty between em-dashes. Em-dash is U+2014.
- [X] T067 [US3] In `internal/fleet/deploy.go`, add `securityFindingsSection(res *DeployResult) string` near `setupRequiredSection` per contracts/scanner-interface.md: returns `""` on nil/empty findings; otherwise returns `"## Security Findings\n\n" + security.RenderForPRBody(res.SecurityFindings) + "\n"`.
- [X] T068 [US3] In `internal/fleet/deploy.go`'s `prBody` (~line 814–820), add a call to `securityFindingsSection(res)` immediately AFTER the `setupRequiredSection(res)` call and BEFORE the existing footer/workflow list. Mirror the empty-string-suppresses-heading idiom (`if section := securityFindingsSection(res); section != "" { b.WriteString(section) }`).
- [X] T069 [US3] In `internal/fleet/sync.go`, add the parallel `securityFindingsSection` composer + `prBody` (or `syncPRBody`) wiring at the equivalent position.
- [X] T070 [US3] In `internal/fleet/upgrade.go`, add the parallel `securityFindingsSection` composer + `prBody` (or `upgradePRBody`) wiring at the equivalent position.

**Checkpoint**: User Story 3 is fully functional. Reviewers see findings in the PR body without needing to dig through stderr / tooling output. SC-007 (security posture visible from PR view) is achievable.

---

## Phase 6: User Story 4 - Catch compiled-workflow lint issues with graceful degradation (Priority: P4)

**Goal**: After `gh aw add` produces `.lock.yml` files, shell out to `actionlint` once per file. Errors → HIGH findings, warnings → MEDIUM. If the binary is missing from PATH (`exec.LookPath("actionlint")` errs), emit exactly one INFO finding `actionlint:not-installed` and skip — run continues without failure (FR-007, SC-005).

**Independent Test**: Two-part. (a) With `actionlint` installed, run `deploy` (dry-run) against a fixture profile pulling `compiled-with-actionlint-error.lock.yml` — assert one HIGH finding emitted with the lock-file path / line / rule. (b) Strip `actionlint` from PATH (or use a fake PATH in a subtest), run again — assert exactly one INFO finding `actionlint:not-installed` and exit code zero.

### Tests for User Story 4 (write FIRST, ensure they FAIL before implementation) ⚠️

- [X] T071 [P] [US4] Create fixture `internal/fleet/security/testdata/security/compiled-with-actionlint-error.lock.yml` with a deliberately malformed/lint-failing workflow (e.g. invalid action reference syntax). Verify by hand that `actionlint compiled-with-actionlint-error.lock.yml` returns a diagnostic.
- [X] T072 [P] [US4] Create fixture `internal/fleet/security/testdata/security/compiled-clean.lock.yml` (a syntactically valid compiled workflow that actionlint accepts cleanly).
- [X] T073 [P] [US4] In `internal/fleet/security/security_test.go`, add `TestActionlintScanner_ErrorMapsToHigh` asserting one HIGH Finding with `RuleID` starting `"actionlint:"`, expected file/line. Skipped via `t.Skip` if `exec.LookPath("actionlint")` fails (matches the FR-007 graceful-degradation contract for the test itself).
- [X] T073a [P] [US4] Create fixture `internal/fleet/security/testdata/security/compiled-with-actionlint-warning.lock.yml` whose YAML produces an actionlint **warning** (not error) — e.g. shellcheck warning, or expression-syntax warning that actionlint classifies as severity warning. Verify by hand that `actionlint --format '{{json .}}' compiled-with-actionlint-warning.lock.yml` returns at least one diagnostic with `kind` mapped to severity warning.
- [X] T073b [P] [US4] Add `TestActionlintScanner_WarningMapsToMedium` asserting one MEDIUM Finding with `RuleID` starting `"actionlint:"` against `compiled-with-actionlint-warning.lock.yml`. Same `t.Skip` if `exec.LookPath("actionlint")` fails. **FR-014 acceptance test for the warning→MEDIUM branch** (T073 covers error→HIGH).
- [X] T074 [P] [US4] Add `TestActionlintScanner_CleanLockFile` asserting zero findings on `compiled-clean.lock.yml`. Same `t.Skip` if missing.
- [X] T075 [P] [US4] Add `TestActionlintScanner_MissingBinary_PATHStripped` subtest: set `t.Setenv("PATH", "")` (or a directory with no actionlint), call `Run`, assert exactly one INFO Finding `actionlint:not-installed`, message contains `"PATH"`, and the test passes regardless of host actionlint installation. This is the SC-005 acceptance test.

### Implementation for User Story 4

- [X] T076 [US4] Create `internal/fleet/security/actionlint.go`: package-doc cites research.md R2 (JSON-format expectation, exit-code → severity mapping). Define `actionlintScanner struct { binPath string }`. Implement `newActionlintScanner() *actionlintScanner` that calls `exec.LookPath("actionlint")` and stores result; constructor never errors.
- [X] T077 [US4] In `actionlint.go`, implement `(*actionlintScanner).Scan`: if `binPath == ""`, return one INFO Finding per the contracts/finding.md "missing binary" shape and exit. Otherwise walk `<cloneDir>/.github/workflows/*.lock.yml`, shell out per-file with `exec.CommandContext(ctx, binPath, "--format", "{{json .}}", lockPath)`, capture stdout, parse JSON array of diagnostics, map each to a Finding per contracts/finding.md (kind=error/exit-1 → HIGH, others → MEDIUM). Honor `ctx` cancellation.
- [X] T078 [US4] In `internal/fleet/security/finding.go`'s `Run`, register `newActionlintScanner()` as the THIRD entry in the scanner list (after gitleaks and structural). Order matches contracts/scanner-interface.md.

**Checkpoint**: User Story 4 is fully functional. The optional compiled-YAML scanner integrates without making `actionlint` a hard fleet dependency. All four user stories now ship; the v1 scanner layer is feature-complete.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Integration test that exercises all scanners end-to-end, the SC-003 benchmark, the SC-006 reproducibility test, the operator-facing skill update, and the local-gate / real-world dry-run validation.

- [X] T079 [P] Create `internal/fleet/security/security_integration_test.go`: `TestRunEndToEnd` invokes `Run(ctx, "testdata/security")` against the fixtures dir and asserts the union of findings matches a golden expected slice (rule IDs + severities + file paths + line numbers). Asserts `len(findings) == expectedCount` AND per-severity counts. **Also asserts that no finding has `Severity == SeverityLow`** — FR-015 invariant ("v1 MUST NOT emit LOW"); a future rule mistakenly typed `SeverityLow` would slip past CI without this guard. This is the SC-001 coverage assertion.
- [X] T080 [P] In `security_integration_test.go`, add `TestRunReproducibility`: run `Run` twice against the fixtures dir and assert the two `[]Finding` slices are byte-identical after `json.Marshal`. SC-006 acceptance.
- [X] T081 [P] In `security_integration_test.go`, add `TestStderrMatchesPRBody` (the SC-004 acceptance test): invoke `Run` against the fixtures dir, then call `RenderForStderr(findings)` AND `RenderForPRBody(findings)`. Parse each surface's per-finding lines/bullets to extract `(severity, rule_id, file, line)` tuples. Assert the two extracted multisets are equal. SC-004 reads "Findings emitted on stderr correspond exactly to findings rendered in the opened PR body when --apply is used (same set, same rule IDs, same file:line references)" — this test enforces that invariant.
- [X] T081a [P] In `security_integration_test.go`, add `TestStderrMatchesEnvelopeWarnings` (separate from SC-004): project all findings via `ToDiagnostic`, render via `RenderForStderr`, assert the rule-IDs and severities visible in the stderr render are the same set as the rule-IDs visible in the `Diagnostic.Fields["rule_id"]` collection. Defends the cmd-layer wiring contract (T018–T021) — stderr and JSON envelope must agree.
- [X] T082 [P] Create `internal/fleet/security/security_bench_test.go` with `BenchmarkRun10Workflows`: replicate one fixture into a temp dir 10 times, time `Run` end-to-end, assert wall-clock < 2s. Record the measured number in the PR description for SC-003. (Constitution Principle IV — benchmark, not assertion-only.)
- [X] T083 [P] Update `skills/fleet-deploy/SKILL.md` with a new paragraph (per quickstart Step 10): "The deploy pipeline now scans fetched workflow markdown for embedded credentials, fleet-structural anti-patterns, and (when actionlint is installed) compiled-YAML lint issues. Findings appear on stderr during dry-run and apply, and in a `## Security Findings` section in the opened PR body. Findings are advisory — they do not block the deploy. Review the section before merging the PR."
- [X] T084 Run `make ci` (fmt-check + vet + lint + test) — must pass per AGENTS.md "Local gate." Fix any lint findings (do NOT re-add the `revive`/`staticcheck` suppressions to `.golangci.yml`; narrow them to specific paths instead if needed).
- [X] T085 Real-world dry-run validation per quickstart Step 9: run `go run . deploy <some-test-repo>` against a scratch repo containing one workflow with an `AKIAIOSFODNN7EXAMPLE` literal. Confirm: (a) stderr emits the expected zerolog-warn line, (b) dry-run summary still prints, (c) `--output json` includes the finding in `warnings[]`, (d) **exit code is zero** (FR-006 acceptance: findings never block; verify via `echo $?` after the dry-run AND after a synthetic `--apply`-blocked rerun with the same fixture), (e) **SC-007 manual UX check**: open the resulting PR (or, if not running with `--apply`, render `prBody` from a synthetic `DeployResult`) and visually confirm every finding visible on stderr also appears in the PR body with rule-ID, file:line, and remedy text — sufficient context to act without leaving the PR view. Document the run in the PR description, including the exit-code observation and a screenshot or quoted excerpt of the PR body's `## Security Findings` section.

**Checkpoint**: Feature is complete, locally validated, and ready to follow the three-turn `--apply` pattern (dry-run → user approval → apply) for the actual ship PR.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1: Setup** — No dependencies; can start immediately.
- **Phase 2: Foundational** — Depends on Phase 1 (needs the dependency added and the package directory exists). BLOCKS all user stories.
- **Phase 3: US1 (P1)** — Depends on Foundational. No dependencies on US2/US3/US4.
- **Phase 4: US2 (P2)** — Depends on Foundational. Independent of US1/US3/US4 (different scanner adapter; only shared edit is the scanner-list registration in `Run`, which is a one-line addition per story).
- **Phase 5: US3 (P3)** — Depends on Foundational. Can produce a meaningful PR-body section even with zero scanners registered (test paths use synthetic findings); BUT for the manual quickstart validation it benefits from US1 or US2 producing real findings, so completing US1 first is the natural order.
- **Phase 6: US4 (P4)** — Depends on Foundational. Independent of US1/US2/US3.
- **Phase 7: Polish** — Depends on all desired user stories being complete (T079 integration test asserts findings from all three scanners; if any of US1/US2/US4 is deferred, narrow T079's expected slice accordingly).

### User Story Dependencies

- **US1 → independent**. Plug-in single scanner (gitleaks).
- **US2 → independent**. Plug-in single scanner (structural).
- **US3 → independent**. Pure render path; testable with synthetic findings.
- **US4 → independent**. Plug-in single scanner (actionlint).
- All four stories share the foundational `Run` scanner-list registration site in `internal/fleet/security/finding.go`. Three tasks (T031, T060, T078) edit the same file at the same code region — sequential within each phase but no merge churn because each adds exactly one line in a fixed position.

### Within Each User Story

- Tests written FIRST, must FAIL before implementation (per AGENTS.md TDD posture and quickstart Step 4).
- Within each story: fixtures + tests in parallel ([P]); implementation tasks generally sequential within the same file.
- US2 has the largest implementation cohort: T052 (skeleton) must precede T053–T060; T051 (allowlist map) must precede T057.

### Parallel Opportunities

- **Phase 1**: T004 [P] is alone; T001+T002+T003 are sequential setup.
- **Phase 2**: T012 + T013 + T014 [P] (different files), T020 + T021 [P] (different cmd files), T022 + T023 [P] (different test files), T010 [P] vs T006 (different files).
- **Phase 3 (US1)**: All five test/fixture tasks T024–T028 are [P] (different files); implementation T029→T030→T031 sequential (same/related files).
- **Phase 4 (US2)**: All fixture+test tasks T032–T050 are [P] (different files). Implementation T051+T052 then T053→T054→T055→T056→T057→T058→T059 (most modify `structural.go`, sequential), then T060.
- **Phase 5 (US3)**: All five test tasks T061–T065 are [P] (same test file but different test funcs — Go allows this; treat as [P] only if running tests, not editing). Implementation T066 then T067 then T068+T069+T070 (different files, [P]).
- **Phase 6 (US4)**: T071–T075 [P]; T076→T077→T078 sequential.
- **Phase 7**: T079–T083 [P] (all independent); T084+T085 sequential at end.

---

## Parallel Example: User Story 2 (highest [P] count)

```bash
# Launch all US2 fixture creation in parallel (eight files):
Task: "Create fixture workflow-with-write-on-schedule.md"
Task: "Create fixture workflow-with-draft-false.md"
Task: "Create fixture workflow-with-missing-protected-files.md"
Task: "Create fixture workflow-with-engine-env-non-allowlist.md"
Task: "Create fixture workflow-with-missing-engine.md"
Task: "Create fixture workflow-with-repo-memory-main.md"
Task: "Create fixture workflow-with-mcp-npm-host.md"
Task: "Create fixture workflow-with-malformed-frontmatter.md"
Task: "Create fixture adr-26919-allowlist.json"

# Launch all US2 test additions in parallel (one test per rule):
Task: "Add TestStructuralScanner_WriteOnSchedule"
Task: "Add TestStructuralScanner_DraftFalse"
Task: "Add TestStructuralScanner_MissingProtectedFiles"
Task: "Add TestStructuralScanner_EngineEnvNonAllowlist"
Task: "Add TestStructuralScanner_MissingEngine_INFO"
Task: "Add TestStructuralScanner_RepoMemoryMain"
Task: "Add TestStructuralScanner_MCPNonStandardHost"
Task: "Add TestStructuralScanner_MalformedFrontmatter_INFO"
Task: "Add TestStructuralScanner_CleanAgenticsWorkflow"
Task: "Add TestADR26919AllowlistMatchesFixture"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 (Setup) — gitleaks dep added, package skeleton.
2. Complete Phase 2 (Foundational) — types, Run scaffold, cmd-layer surfaces wired. `make ci` green; nothing visible to users yet.
3. Complete Phase 3 (US1) — gitleaks scanner registered. End-to-end: a fake AWS key in a fixture surfaces on stderr and in JSON envelope.
4. **STOP and VALIDATE**: run quickstart Step 9 manually. Confirm SC-001 + SC-002 (gitleaks-only subset) + SC-006 (reproducibility) — all achievable with US1 alone.
5. Ship MVP if the broader fleet team needs the credential-detection capability immediately. The other three stories can land in subsequent PRs if scoping requires; the architecture is plug-in by design.

### Incremental Delivery

1. Setup + Foundational → MVP foundation ready (no observable behavior).
2. Add US1 → secrets detected → ship MVP.
3. Add US2 → structural anti-patterns detected → ship.
4. Add US3 → PR-body integration → ship (now reviewers see findings without leaving PR).
5. Add US4 → actionlint integration → ship (compiled-YAML coverage).
6. Phase 7 polish lands once all desired stories are in.

### Parallel Team Strategy

If staffing permits multiple developers post-Foundational:

- Developer A: US1 (gitleaks adapter)
- Developer B: US2 (structural rules + ADR-26919 port — largest single phase)
- Developer C: US3 (PR-body composer + render polish)
- Developer D: US4 (actionlint adapter)

Each developer's branch only edits one new file (their scanner) plus a one-line addition to `Run`'s scanner-list. Merge conflicts are minimal and resolvable by sequencing the scanner-list edits.

---

## Notes

- `[P]` tasks edit different files with no dependency on incomplete tasks.
- `[Story]` label is REQUIRED for tasks in user-story phases (Phases 3–6); ABSENT for Setup, Foundational, Polish.
- Per AGENTS.md: every exported identifier MUST have a godoc comment ending in a period; reuse `fleet.ParseFrontmatter` rather than re-implementing frontmatter parsing.
- Per AGENTS.md: do NOT bypass gpg signing in any wired-in command; do NOT add `git add`/`git commit`/`git push` from the Bash tool — only the existing `internal/fleet/` exec.Command paths.
- Per AGENTS.md: `make ci` is the ship gate; `go build`/`go vet` alone is insufficient.
- Per AGENTS.md: do NOT hand-edit `CHANGELOG.md` — release-please regenerates it. The implementation commit subject IS the changelog entry; use `feat(security): ...` per the issue title.
- FR-008a is a security invariant — failing the redaction tests is a regression to surface, not a stylistic flake to retry.
- The scanner-list registration order in `Run` is fixed (gitleaks → structural → actionlint) per contracts/scanner-interface.md — preserve this order even when implementing stories out of priority order.
