---
description: "Task list for feature implementation: CLI JSON output mode"
---

# Tasks: CLI JSON Output Mode

**Input**: Design documents from `/specs/003-cli-output-json/`
**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/cli-flags.md](contracts/cli-flags.md), [contracts/envelope.md](contracts/envelope.md), [contracts/ndjson.md](contracts/ndjson.md), [quickstart.md](quickstart.md)

**Tests**: Tests ARE included in this plan because spec.md's Testing Strategy section explicitly calls for: (a) envelope shape / schema_version / empty-array pinning tests in `cmd/output_test.go`; (b) flag-validator unit tests; (c) ListResult snapshot tests; (d) manual `jq` validation via quickstart.md. TDD ordering is used — each user story phase writes the failing test first, then the implementation makes it pass.

**Organization**: Tasks are grouped by user story (US1 = list P1 MVP, US2 = deploy P2, US3 = sync + upgrade P3) so each story can be implemented and validated independently, matching the spec's three-slice priority breakdown.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Every description includes the exact file path(s) touched

## Path Conventions

Single-project Go CLI. Source under `cmd/` and `internal/fleet/`. Tests colocated as `*_test.go` next to production files. Module path `github.com/rshade/gh-aw-fleet`. No new dependencies — stdlib `encoding/json` + `reflect` only (per research.md R2).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Baseline capture for the invariants this feature must preserve: zero new deps, byte-identical text-mode output.

- [X] T001 Capture baseline `go mod graph | sort > /tmp/gomod-003-before.txt` from repo root. No dependency changes are expected in this feature (FR-018, SC-008). T030 will diff against this file post-implementation; the diff MUST be empty.
- [X] T002 Capture text-mode baseline outputs for later byte-identity verification (FR-014, SC-003). Run `go run . list > /tmp/list-text-before.txt 2>/dev/null`; run `go run . deploy rshade/gh-aw-fleet > /tmp/deploy-text-before.txt 2>/dev/null`; run `go run . sync rshade/gh-aw-fleet > /tmp/sync-text-before.txt 2>/dev/null`; run `go run . upgrade rshade/gh-aw-fleet > /tmp/upgrade-text-before.txt 2>/dev/null`. T028 will re-run without `--output` (default `text`) and diff; any diff fails SC-003.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Lock the shared types (`Envelope`, `Diagnostic`, `ListResult` base), helpers (`writeEnvelope`, `outputMode`, `initSlices`, `validateOutputMode`), and the `-o`/`--output` flag that every user story depends on.

**⚠️ CRITICAL**: No user-story work may begin until this phase is complete.

- [X] T003 [P] TDD — write failing tests in new `internal/fleet/diagnostics_test.go` covering: (a) `Diagnostic` struct marshals with snake_case JSON keys `code`, `message`, `fields`, and `fields` is omitted when nil/empty (per data-model.md Entity 2); (b) every entry in the package-level `hints` slice has a non-empty `Code` field that is snake_case (asserts pattern `^[a-z][a-z0-9_]*$`) — forces T005 to backfill codes; (c) `CollectHintDiagnostics("Unknown property: foo")` returns a single `Diagnostic` with `Code == DiagUnknownProperty`, `Message` equal to the hint text, and `Fields["hint"]` equal to the same text; (d) `CollectHintDiagnostics("")` returns an empty slice (non-nil — per FR-006 JSON arrays never null semantics for hint-collector consumers). Tests MUST be in package `fleet` (internal). They MUST fail initially because `Diagnostic` and `CollectHintDiagnostics` do not yet exist.
- [X] T004 [P] TDD — write failing tests in new `cmd/output_test.go` covering: (a) `TestEnvelope_TopLevelKeysPinned` — construct an envelope with a `ListResult{LoadedFrom: "", Repos: []ListRow{}}` result, empty warnings/hints, marshal via `writeEnvelopeTo(&buf, ...)`, assert the output byte-equals exactly `{"schema_version":1,"command":"list","repo":"","apply":false,"result":{"loaded_from":"","repos":[]},"warnings":[],"hints":[]}\n`; (b) `TestEnvelope_SchemaVersionIs1` — assert the top-level `schema_version` is the integer `1`; (c) `TestInitSlices_NilToEmpty` — construct a `DeployResult{}` with all-nil slices, call `initSlices(&r)`, assert `r.Added`, `r.Skipped`, `r.Failed` are non-nil empty slices; (d) `TestOutputFlag_AcceptsText`, `TestOutputFlag_AcceptsJSON`, `TestOutputFlag_DefaultText`, `TestOutputFlag_RejectsYAML`, `TestOutputFlag_RejectsUpperCase`, `TestOutputFlag_RejectsEmpty` — build a root command with a stub subcommand, execute with the corresponding `-o` value, assert the validator error path per `contracts/cli-flags.md`; (e) `TestEnvelope_ResultNullOnPreFailure` — pass `nil` as result to `writeEnvelopeTo`, assert the output contains `"result":null` and `schema_version/command/repo/apply/warnings/hints` are still present. All tests MUST fail initially.
- [X] T005 Extend `internal/fleet/diagnostics.go`: (a) add `Diagnostic` struct — `type Diagnostic struct { Code string \`json:"code"\`; Message string \`json:"message"\`; Fields map[string]any \`json:"fields,omitempty"\` }`; (b) add package-level constants — `const ( DiagMissingSecret = "missing_secret"; DiagDriftDetected = "drift_detected"; DiagHint = "hint"; DiagUnknownProperty = "unknown_property"; DiagHTTP404 = "http_404"; DiagGPGFailure = "gpg_failure" )`; (c) add `Code string` field to the existing `Hint` struct; (d) backfill each entry in the `hints` slice with its code — `Unknown property: mount-as-clis` → `DiagUnknownProperty`, `Unknown property:` → `DiagUnknownProperty`, `HTTP 404` → `DiagHTTP404`, `gpg failed to sign` → `DiagGPGFailure`; (e) add `func CollectHintDiagnostics(texts ...string) []Diagnostic` that mirrors `CollectHints`'s scan-and-dedupe loop but emits `Diagnostic{Code: h.Code, Message: h.Message, Fields: map[string]any{"hint": h.Message}}` per matched hint; return `[]Diagnostic{}` (non-nil empty) when no matches, for consumer iteration ergonomics. Add package comment explaining the two-function invariant: text-mode keeps using `CollectHints`, JSON-mode uses `CollectHintDiagnostics`; both must stay in sync by construction (same `hints` slice). Tests from T003 MUST pass.
- [X] T006 Add JSON tags to `WorkflowOutcome` in `internal/fleet/deploy.go`: `Name` → `json:"name"`, `Spec` → `json:"spec"`, `Reason` → `json:"reason"`, `Error` → `json:"error"`. This is foundational (not US2-scoped) because `WorkflowOutcome` is nested inside `DeployResult.Added/Skipped/Failed` AND inside `SyncResult.Deploy.Added/...` AND inside `SyncResult.DeployPreflight.Added/...`, so it must be tagged before US3 can build its envelope. No struct-shape change; only tag additions.
- [X] T007 Create `cmd/output.go` (package `cmd`). Contents: (a) imports — `encoding/json`, `fmt`, `io`, `os`, `reflect`, `github.com/spf13/cobra`, `github.com/rshade/gh-aw-fleet/internal/fleet`; (b) `type Envelope struct` with exactly the fields in `data-model.md` Entity 1, all with json tags; (c) `const SchemaVersion = 1` (package-level); (d) `func writeEnvelope(cmd *cobra.Command, commandName, repo string, apply bool, result any, warnings, hints []fleet.Diagnostic) error` that dispatches to `writeEnvelopeTo(cmd.OutOrStdout(), ...)`; (e) `func writeEnvelopeTo(w io.Writer, commandName, repo string, apply bool, result any, warnings, hints []fleet.Diagnostic) error` which calls `initSlices(result)` (only if non-nil), normalizes `warnings`/`hints` to non-nil empty slices if nil, builds an `Envelope{SchemaVersion, Command, Repo, Apply, Result, Warnings, Hints}`, and emits via `json.NewEncoder(w).Encode(env)` — `Encoder.Encode` handles newline termination per research.md R4; (f) `func outputMode(cmd *cobra.Command) string` — reads `--output`, returns `"text"` on empty (defensive; PersistentPreRunE has validated); (g) `func validateOutputMode(mode string) error` — closed set `{"text","json"}`, returns `fmt.Errorf("unsupported output mode %q: expected one of: text, json", mode)` on mismatch; (h) `func rejectJSONMode(cmd *cobra.Command, subcommand string) error` — returns non-nil error iff `outputMode(cmd) == "json"`, message per `contracts/cli-flags.md` FR-013 section; (i) `func initSlices(v any)` — reflection walker per research.md R2, handles pointer-to-struct, struct, `reflect.Slice`, nested slice-of-struct; skips pointer fields whose `Kind() == reflect.Ptr` but recurses into them when non-nil and pointing to a struct. Add comments explaining WHY `initSlices` exists (FR-009 `[]`-not-`null` contract) and WHY the mutex note for future parallelization (`contracts/ndjson.md`). Tests (a) and (c) from T004 MUST pass after this task; tests (d) and (e) MUST fail until T008 lands; test (b) passes trivially.
- [X] T008 Update `cmd/root.go`: (a) add persistent flag registration — `root.PersistentFlags().StringP("output", "o", "text", "Output format: text|json")`; (b) extend the existing `PersistentPreRunE` (landed in #34 for log-flag validation) to additionally read the `--output` value and call `validateOutputMode(mode)`, returning its error directly so cobra's default error writer prints plain text to stderr and exits non-zero (FR-002 per `contracts/cli-flags.md`). Order: log-flag validation first (matches #34 pattern), then `--output` validation. Keep `SilenceUsage: true`. All T004 tests MUST pass after this task.

**Checkpoint**: `Envelope`, `Diagnostic`, `writeEnvelope`, `outputMode`, `initSlices`, `validateOutputMode`, `rejectJSONMode`, the `-o`/`--output` flag with validator, and `WorkflowOutcome` JSON tags are all in place. All foundational tests green (T003, T004). User story work can begin.

---

## Phase 3: User Story 1 - Structured `list` output for agentic consumption (Priority: P1) 🎯 MVP

**Goal**: `go run . list -o json` emits a single JSON envelope on stdout with `schema_version: 1`, `command: "list"`, `result.loaded_from`, and `result.repos[]` populated. Text mode byte-identical. The `(loaded fleet.local.json)` breadcrumb stays on stderr.

**Independent Test**: Run quickstart.md steps 1 (list -o json) and 8 (text mode byte-identity). Also run `go run . list -o json 2>/dev/null | jq -e .schema_version` — exit 0 with stdout `1`. Run `go run . list -o json 2>/dev/null | jq -e '.result.repos[0].repo'` — exit 0 with first repo name.

### Tests for User Story 1 ⚠️

> Write these tests FIRST, ensure they FAIL before implementation.

- [X] T009 [P] [US1] Write failing test `TestBuildListResult_Shape` in new `internal/fleet/list_result_test.go`: load a fixture `Config` (inline literal, not from disk) with two repos — one minimal, one with `ExtraWorkflows` and `ExcludeFromProfiles`. Call `BuildListResult(cfg)`. Assert: `result.LoadedFrom == cfg.LoadedFrom`; `result.Repos` is sorted alphabetically by `Repo`; every `ListRow.Profiles/Workflows/Excluded/Extra` is non-nil (empty slice, not nil) per FR-009. Also assert `ListRow.Engine == cfg.EffectiveEngine(repo)` — empty string when no engine (NOT the text-mode `"-"` placeholder per data-model.md ListRow note).
- [X] T010 [P] [US1] Write failing tests in `cmd/output_test.go`: (a) `TestListEnvelope_EmptyFleet` — build a `Config` with `Repos: map[string]RepoSpec{}`, call `fleet.BuildListResult`, pass through `writeEnvelopeTo`, assert stdout JSON has `result.repos == []` (non-null) and `result.loaded_from == ""`; (b) `TestListEnvelope_Populated` — use `testdata/list-envelope.golden.json` (create this fixture) and compare byte-for-byte via a canned config; (c) `TestListCmd_JSONMode` — use cobra `ExecuteC` in-process with a canned config via a test-only `--dir` override, capture stdout, assert it parses as `cmd.Envelope{Command: "list"}`.

### Implementation for User Story 1

- [X] T011 [US1] Create `internal/fleet/list_result.go` containing: (a) `type ListResult struct { LoadedFrom string \`json:"loaded_from"\`; Repos []ListRow \`json:"repos"\` }`; (b) `type ListRow struct { Repo string \`json:"repo"\`; Profiles []string \`json:"profiles"\`; Engine string \`json:"engine"\`; Workflows []string \`json:"workflows"\`; Excluded []string \`json:"excluded"\`; Extra []string \`json:"extra"\` }`; (c) `func BuildListResult(cfg *Config) (*ListResult, error)` — walks `cfg.Repos` keys, sorts via `sort.Strings`, for each repo calls `cfg.ResolveRepoWorkflows(repo)` (propagates errors), extracts names via a helper `workflowNames([]ResolvedWorkflow) []string`, builds `ListRow` with `Engine: cfg.EffectiveEngine(repo)` (NOT run through `orDash`), initializes `Profiles/Workflows/Excluded/Extra` as non-nil empty slices when absent. Returns `&ListResult{LoadedFrom: cfg.LoadedFrom, Repos: rows}`. Tests T009 and T010(a)/(b) MUST pass.
- [X] T012 [US1] Update `cmd/list.go`: replace the current `RunE` to branch on `outputMode(cmd)`. Text path — keep existing tabwriter code unchanged (the breadcrumb `fmt.Fprintf(cmd.OutOrStderr(), "  (loaded %s)\n", cfg.LoadedFrom)` stays). JSON path — call `res, err := fleet.BuildListResult(cfg)`; on error, call `writeEnvelope(cmd, "list", "", false, nil, nil, []fleet.Diagnostic{{Code: fleet.DiagHint, Message: err.Error(), Fields: map[string]any{"hint": err.Error()}}})` and return the error (FR-020 + FR-021 — non-zero exit preserved); on success, call `return writeEnvelope(cmd, "list", "", false, res, nil, nil)`. Keep the breadcrumb route to stderr in BOTH modes — text mode consumers expect it, JSON mode consumers already redirect stderr. Test T010(c) MUST pass.
- [X] T013 [US1] Reject `-o json` on subcommands that do not support it: edit `cmd/template.go` (template fetch RunE — first check), `cmd/add.go` (add RunE — first check), `cmd/stubs.go` (status stub RunE — first check). In each, add at the very top of RunE: `if err := rejectJSONMode(cmd, "<subcommand name>"); err != nil { return err }`. Subcommand names as they appear in the error message: `"template fetch"`, `"add"`, `"status"`. Add a test `TestJSONModeRejected_TemplateFetch` in `cmd/output_test.go` that executes `template fetch -o json` and asserts the error message matches the contract in `contracts/cli-flags.md`.

**Checkpoint**: After this phase, `list -o json` works end-to-end. Quickstart steps 1 and 8 pass. The MVP is deliverable; US2 and US3 can be deferred to later sprints without breaking US1.

---

## Phase 4: User Story 2 - Structured `deploy` output with embedded warnings (Priority: P2)

**Goal**: `go run . deploy <repo> -o json` emits an envelope with `result.added/skipped/failed` arrays (never null when empty), `result.missing_secret` / `result.secret_key_url`, `result.pr_url`, and when applicable a `warnings[]` entry with `code: "missing_secret"` AND the same warning on stderr via zerolog (dual-emission per research.md R5).

**Independent Test**: Run quickstart.md step 2 (deploy -o json dry-run) and step 2's missing-secret demo. Verify `jq '.result.added | type'` returns `"array"` (not `"null"`) on a deploy with no adds. Verify `jq '.warnings[0].code'` returns `"missing_secret"` on a deploy targeting a repo without the engine secret.

### Tests for User Story 2 ⚠️

- [X] T014 [P] [US2] Write failing tests in `cmd/output_test.go`: (a) `TestDeployEnvelope_EmptyArrays` — construct a `fleet.DeployResult{Repo: "x/y"}` (all slices nil), pass through `writeEnvelopeTo` as the "deploy" command, parse the stdout JSON, assert `.result.added`, `.result.skipped`, `.result.failed` are all empty arrays (JSON type `array`, length 0); (b) `TestDeployEnvelope_MissingSecretWarning` — construct a `DeployResult{Repo: "x/y", MissingSecret: "ANTHROPIC_API_KEY", SecretKeyURL: "https://example.com/key"}`, build a warnings slice containing a single `fleet.Diagnostic{Code: DiagMissingSecret, Message: "Actions secret ANTHROPIC_API_KEY is missing...", Fields: map[string]any{"secret": "ANTHROPIC_API_KEY", "url": "https://example.com/key"}}`, emit envelope, assert `.warnings[0].code == "missing_secret"`, `.warnings[0].fields.secret == "ANTHROPIC_API_KEY"`; (c) `TestDeployEnvelope_ApplyFlag` — assert `envelope.apply == true` when `apply=true` is passed, `false` otherwise.

### Implementation for User Story 2

- [X] T015 [US2] Add JSON tags to `DeployResult` in `internal/fleet/deploy.go` per data-model.md Entity 4: `Repo` → `json:"repo"`, `CloneDir` → `json:"clone_dir"`, `Added` → `json:"added"`, `Skipped` → `json:"skipped"`, `Failed` → `json:"failed"`, `InitWasRun` → `json:"init_was_run"`, `BranchPushed` → `json:"branch_pushed"`, `PRURL` → `json:"pr_url"`, `MissingSecret` → `json:"missing_secret"`, `SecretKeyURL` → `json:"secret_key_url"`. Do NOT use `omitempty` on any slice — FR-009 requires `[]`-not-omitted even when empty. Do NOT change struct field shape. Verify with `go build ./...` + `go vet ./...` after the change.
- [X] T016 [US2] Update `cmd/deploy.go` to branch on `outputMode(cmd)`:

  (a) **Text-mode path (default)**: keep `printDeploy(cmd, res, flagApply); return deployErr` exactly as today — no change.

  (b) **JSON-mode path**: do NOT call `printDeploy` (suppresses stdout tabwriter per FR-003). Instead:

  1. Build `var warnings []fleet.Diagnostic`. If `res != nil && res.MissingSecret != ""`, call `emitDeployWarnings(res)` (existing zerolog stderr emission — unchanged, satisfies FR-011 stderr half) AND append `fleet.Diagnostic{Code: fleet.DiagMissingSecret, Message: <reconstruct the same text `emitDeployWarnings` builds at cmd/deploy.go:113–118>, Fields: map[string]any{"secret": res.MissingSecret, "url": res.SecretKeyURL}}` to `warnings` (envelope half of dual-emission). Consider factoring the message-construction out of `emitDeployWarnings` into a helper `buildMissingSecretMessage(res) string` so both paths share the exact text.
  2. Build `var hints []fleet.Diagnostic`. If `res != nil && len(res.Failed) > 0`, construct `errs := make([]string, 0, len(res.Failed))` and populate from each `WorkflowOutcome.Error` field (matches cmd/deploy.go:84–88 existing text-mode pattern — this is the concrete hint source for deploy), then call `rawHints := fleet.CollectHints(errs...)` + `emitHints(res.Repo, rawHints)` (existing stderr emission — unchanged), then assign `hints = fleet.CollectHintDiagnostics(errs...)` (envelope half). Do NOT print `hint: ...` lines to stdout — those are text-mode-only tabwriter content.
  3. On success (`deployErr == nil`): return `writeEnvelope(cmd, "deploy", repo, flagApply, res, warnings, hints)`.
  4. On failure with a result (`res != nil && deployErr != nil`): `writeEnvelope(cmd, "deploy", repo, flagApply, res, warnings, hints)` — `result` is non-null because `Deploy` returned partial state; then `return deployErr` for the non-zero exit.
  5. On pre-result failure (`res == nil && deployErr != nil`): `writeEnvelope(cmd, "deploy", repo, flagApply, nil, nil, []fleet.Diagnostic{{Code: fleet.DiagHint, Message: deployErr.Error(), Fields: map[string]any{"hint": deployErr.Error()}}})` (FR-020); then `return deployErr`.
  6. Exit-code parity (FR-021): every return path returns the unwrapped `deployErr`, which is the same value the text-mode path returns.

  Tests T014 MUST pass after this task.

**Checkpoint**: After this phase, `deploy -o json` emits structured output including warnings. Quickstart step 2 passes. US3 (sync + upgrade) can proceed.

---

## Phase 5: User Story 3 - Structured `sync` and `upgrade` output (Priority: P3)

**Goal**: `sync -o json` and `upgrade -o json` emit envelopes with full result structs; `upgrade --all -o json` emits NDJSON (one envelope per line per repo, streaming per-repo); `audit_json` nests as a native JSON object (not escape-encoded).

**Independent Test**: Run quickstart.md steps 3, 4, 5 (sync, upgrade single, upgrade --all NDJSON). Verify `jq '.result.drift | type'` returns `"array"` on a sync with no drift. Verify `jq '.result.audit_json | type'` returns `"object"` on a successful upgrade (not `"string"`). Verify `upgrade --all -o json 2>/dev/null | wc -l` equals the repo count.

### Tests for User Story 3 ⚠️

- [X] T017 [P] [US3] Write failing tests in `cmd/output_test.go` for sync: (a) `TestSyncEnvelope_EmptyArrays` — `SyncResult{Repo: "x/y"}` zero-value, assert `.result.missing`, `.result.drift`, `.result.expected`, `.result.pruned` all serialize as `[]`; (b) `TestSyncEnvelope_NilDeployFields` — `SyncResult{Deploy: nil, DeployPreflight: nil}`, assert `.result.deploy == null` and `.result.deploy_preflight == null` (pointer-nil semantics per data-model.md Entity 5); (c) `TestSyncEnvelope_NestedDeployPreflight` — `SyncResult{DeployPreflight: &DeployResult{Repo: "x/y", Added: []WorkflowOutcome{{Name: "foo"}}}}`, assert `.result.deploy_preflight.added[0].name == "foo"` — confirms nested struct serializes through; (d) `TestSyncEnvelope_DriftDiagnostic` — build a warnings slice with `Diagnostic{Code: DiagDriftDetected, Fields: map[string]any{"drift": []string{"orphan"}}}`, assert the envelope `.warnings[0].fields.drift` is the JSON array `["orphan"]` (not `"[orphan]"` string).
- [X] T018 [P] [US3] Write failing tests in `cmd/output_test.go` for upgrade: (a) `TestUpgradeEnvelope_EmptyArrays` — `UpgradeResult{Repo: "x/y"}` zero-value, assert `.result.changed_files` and `.result.conflicts` are `[]`; (b) `TestEnvelope_AuditJSONNests` — construct `UpgradeResult{AuditJSON: json.RawMessage(\`{"version":"1","findings":[]}\`)}`, marshal via `writeEnvelopeTo`, parse the output, assert `result.audit_json` is a JSON object (use `json.RawMessage` destination and `json.Unmarshal` re-parse into `map[string]any`), NOT a string — per `contracts/envelope.md` and research.md R1; (c) `TestUpgradeEnvelope_AuditJSONNil` — when `AuditJSON` is nil, `.result.audit_json == null` (stdlib default for nil RawMessage).
- [X] T019 [P] [US3] Write failing tests for NDJSON mode in `cmd/output_test.go` (per `contracts/ndjson.md` test matrix): (a) `TestUpgradeAll_NDJSONLineCount` — stub `fleet.Upgrade` (or factor the per-repo emission into an extractable helper that takes pre-built results) to yield 3 synthetic `UpgradeResult` values; capture stdout, assert exactly 3 lines, each terminated by `\n`, no trailing blank line; (b) `TestUpgradeAll_PerLineSelfContained` — parse each line independently with `json.Unmarshal` into `cmd.Envelope`; assert each has distinct `repo` and `command == "upgrade"`; (c) `TestUpgradeAll_ErrorRepoIncluded` — one of the three synthetic results is `nil` + an error; assert that repo's line has `result: null` and a populated `hints[]`, while the other two lines are unaffected; (d) `TestUpgradeAll_EmptyFleet` — zero repos, assert stdout is exactly empty (0 bytes, 0 lines), exit code 0.

### Implementation for User Story 3

- [X] T020 [US3] Add JSON tags to `SyncResult` in `internal/fleet/sync.go` per data-model.md Entity 5: `Repo` → `json:"repo"`, `CloneDir` → `json:"clone_dir"`, `Missing` → `json:"missing"`, `Drift` → `json:"drift"`, `Expected` → `json:"expected"`, `Deploy` → `json:"deploy"` (pointer — stays nullable via stdlib default), `Pruned` → `json:"pruned"`, `DeployPreflight` → `json:"deploy_preflight"`. No `omitempty` on slice fields. No struct-shape change. Tests T017(a)-(c) for serialization MUST pass.
- [X] T021 [US3] Add JSON tags to `UpgradeResult` in `internal/fleet/upgrade.go` per data-model.md Entity 6: `Repo` → `json:"repo"`, `CloneDir` → `json:"clone_dir"`, `UpgradeOK` → `json:"upgrade_ok"`, `UpdateOK` → `json:"update_ok"`, `ChangedFiles` → `json:"changed_files"`, `Conflicts` → `json:"conflicts"`, `NoChanges` → `json:"no_changes"`, `BranchPushed` → `json:"branch_pushed"`, `PRURL` → `json:"pr_url"`, `AuditJSON` → `json:"audit_json"` (keep the `json.RawMessage` type — research.md R1 confirms native nested-object semantics), `OutputLog` → `json:"output_log"`. Tests T018 MUST pass.
- [X] T022 [US3] Update `cmd/sync.go` to branch on `outputMode(cmd)`:

  (a) **Text-mode path (default)**: keep `printSync(cmd, res, flagApply, flagPrune); return syncErr` exactly as today — no change.

  (b) **JSON-mode path**: do NOT call `printSync`. Instead:

  1. Build `var warnings []fleet.Diagnostic`. If `res != nil && len(res.Drift) > 0`, call `emitSyncWarnings(res)` (existing zerolog stderr emission — unchanged) AND append `fleet.Diagnostic{Code: fleet.DiagDriftDetected, Message: "Drift detected: workflows on disk not declared in fleet.json" (match cmd/sync.go:95 verbatim), Fields: map[string]any{"drift": res.Drift}}`.
  2. Build `var hints []fleet.Diagnostic` from `res.DeployPreflight.Failed` error strings — this is the ONLY sync-side hint source in current text-mode (see `printSyncPreflight` at cmd/sync.go:140–155). If `res != nil && res.DeployPreflight != nil && len(res.DeployPreflight.Failed) > 0`, construct `errs := make([]string, 0, len(res.DeployPreflight.Failed))` from each `WorkflowOutcome.Error`, then `rawHints := fleet.CollectHints(errs...)` + `emitHints(res.Repo, rawHints)` (existing stderr emission), then `hints = fleet.CollectHintDiagnostics(errs...)`. **Important — FR-014 parity**: `res.Deploy.Failed` (from actual apply) is NOT a hint source today (`printSyncDeploy` at cmd/sync.go:129–138 prints counts only, not failure details). JSON mode MUST mirror this — do NOT add `res.Deploy.Failed` as an additional hint source, even though it's technically available. Widening the hint surface is a separate feature that would affect text mode too.
  3. On success (`syncErr == nil`): return `writeEnvelope(cmd, "sync", repo, flagApply, res, warnings, hints)`.
  4. On failure with a result (`res != nil && syncErr != nil`): `writeEnvelope(cmd, "sync", repo, flagApply, res, warnings, hints)`; then `return syncErr`.
  5. On pre-result failure (`res == nil && syncErr != nil`): `writeEnvelope(cmd, "sync", repo, flagApply, nil, nil, []fleet.Diagnostic{{Code: fleet.DiagHint, Message: syncErr.Error(), Fields: map[string]any{"hint": syncErr.Error()}}})` (FR-020); then `return syncErr`.
  6. Exit-code parity (FR-021): every return path returns the unwrapped `syncErr`, matching text mode.

  Tests T017(d) + quickstart §3 MUST pass.
- [X] T023 [US3] Update `cmd/upgrade.go`: branch on `outputMode(cmd)`. (a) Single-repo path (positional arg form): after `res, err := fleet.Upgrade(...)`, emit `writeEnvelope(cmd, "upgrade", repo, opts.Apply, res, warnings, fleet.CollectHintDiagnostics(res.OutputLog))` on success. On error (pre-result failure), emit `writeEnvelope(..., nil, nil, []fleet.Diagnostic{{Code: DiagHint, Message: err.Error(), Fields: ...}})` and return the error. (b) `--all` path: keep the existing per-repo loop; inside the loop, after each `fleet.Upgrade` returns, call `writeEnvelope(...)` once per repo — this emits the NDJSON line (one envelope + one `\n` from `json.Encoder.Encode`). Do NOT accumulate results into a slice and batch-encode — streaming is explicit per `contracts/ndjson.md`. Error in the middle of the loop: emit the envelope for that repo with `result: null` + diagnostics, then CONTINUE to the next repo (do NOT short-circuit — preserves existing text-mode behavior per FR-014). Capture the worst exit code from all per-repo outcomes (same as text mode today). Tests T018 + T019 MUST pass. Quickstart steps 4 and 5 MUST pass.

**Checkpoint**: All four commands support `-o json`. All three user stories are independently testable. Quickstart steps 1–8 all pass.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Documentation updates, regression tests, and the final local gate that certifies the feature as done.

- [X] T024 [P] Add `TestEnvelope_NoNullSlices_AllResultTypes` to `cmd/output_test.go`: a table-driven test iterating over zero-value instances of `fleet.ListResult`, `fleet.DeployResult`, `fleet.SyncResult`, `fleet.UpgradeResult` — for each, pass through `writeEnvelopeTo`, parse the output, and use reflection to walk the result struct's fields: for every `reflect.Slice` field (recursively), assert the JSON value is an array (not null). Catches future regressions where a new slice field is added without `initSlices` coverage.
- [X] T025 [P] Add `TestEnvelope_SchemaVersionConstant` to `cmd/output_test.go`: assert `cmd.SchemaVersion == 1`. Trivial but pins the contract against accidental bumps during refactor.
- [X] T026 [P] Update `CHANGELOG.md` with a `### Added` entry under the next release heading: `- CLI --output / -o flag with values text (default) and json. JSON mode emits a versioned envelope (schema_version: 1) on stdout for list, deploy, sync, upgrade; upgrade --all emits NDJSON. See specs/003-cli-output-json/quickstart.md.` Also add a new top-level section `### JSON envelope schema` documenting that schema_version 1 is the current contract and linking to `specs/003-cli-output-json/contracts/envelope.md` for the per-field detail.
- [X] T027 [P] Update `skills/fleet-deploy/SKILL.md`, `skills/fleet-eval-templates/SKILL.md`, `skills/fleet-upgrade-review/SKILL.md`, `skills/fleet-onboard-repo/SKILL.md` with a one-line note under each skill's "Debugging" or "Extended usage" section (create the section if absent): `For machine-readable output, pass -o json on list/deploy/sync/upgrade. See specs/003-cli-output-json/quickstart.md.` Do NOT modify the three-turn pattern or any approval-gated wording.
- [X] T028 Post-implementation text-mode byte-identity verification (FR-014, SC-003). Run each command without `--output` (default text) and diff against the T002 baseline: `diff /tmp/list-text-before.txt <(go run . list 2>/dev/null)`; same for `deploy`, `sync`, `upgrade` against their respective baselines. Every diff MUST be empty. If any diff has content, inspect whether the drift is intentional (e.g., a new stderr line accidentally landing on stdout in one of the phases) and fix it in the corresponding cmd/*.go file before declaring the feature complete.
- [X] T029 Run quickstart.md end-to-end against a real fleet (`rshade/gh-aw-fleet` from `fleet.json` is sufficient for all 8 numbered sections). Each section's expected output must match. Sections of particular importance: §1 (`list -o json | jq`), §5 (NDJSON line count for `upgrade --all`), §6 (error envelope for non-existent repo), §7 (invalid flag error), §8 (text-mode byte-identity; this overlaps with T028 but runs as a user-flow check). Record any failures as bugs to fix before declaring done.
- [X] T030 Verify `go mod graph | sort > /tmp/gomod-003-after.txt; diff /tmp/gomod-003-before.txt /tmp/gomod-003-after.txt` is empty (FR-018, SC-008). Zero new dependencies. If the diff is non-empty, something imported a package it shouldn't have (only stdlib + existing modules are permitted) — back out the offending import.
- [X] T031 Run the full local CI gate: `make ci` (runs `fmt-check`, `vet`, `lint`, `test`). This MUST pass before the feature is reported done (user feedback memory `feedback_local_gate.md` — `go build` / `go vet` alone are not sufficient; CI runs stricter checks). If `make lint` takes >5 minutes, that's expected (CLAUDE.md note) — do not kill the run.

**Feature complete** when T001–T031 all checked off, `make ci` green, quickstart.md verified end-to-end, no CHANGELOG drift, no byte-identity regressions in text mode.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — T001 and T002 capture baselines before any code changes.
- **Foundational (Phase 2)**: Depends on Setup completion — BLOCKS all user stories. Must complete T003–T008 before any US1/US2/US3 work.
- **User Story 1 (Phase 3)**: Depends on Foundational. T009–T013 can then proceed.
- **User Story 2 (Phase 4)**: Depends on Foundational. Can run in parallel with US1 (different files: `cmd/deploy.go` vs `cmd/list.go` + `internal/fleet/list_result.go`) if staffed, or sequential after US1 for incremental delivery.
- **User Story 3 (Phase 5)**: Depends on Foundational. Can run in parallel with US1 and US2 (different files: `cmd/sync.go`, `cmd/upgrade.go`, `internal/fleet/sync.go`, `internal/fleet/upgrade.go`) if staffed.
- **Polish (Phase 6)**: Depends on all desired user stories being complete. T024–T031 are the close-out.

### User Story Dependencies

- **User Story 1 (P1 MVP)**: No dependencies on other stories. Ships independently.
- **User Story 2 (P2)**: No code dependencies on US1 — the two stories touch disjoint command files. Shares the foundational layer (`writeEnvelope`, `Diagnostic`, etc.).
- **User Story 3 (P3)**: No code dependencies on US1 or US2 at the command level. `WorkflowOutcome` JSON tags (T006 in Foundational) ARE a shared prerequisite because `SyncResult.Deploy` nests `DeployResult` which nests `WorkflowOutcome` — hence T006's placement in Phase 2, not US2.

### Within Each User Story

- Tests come BEFORE implementation (TDD — tests must fail, then implementation makes them pass).
- Types/structs come before consumers (e.g., T011 `ListResult` type before T012 `list.go` consumer).
- Tags on existing structs (T015, T020, T021) come before command branches (T016, T022, T023).

### Parallel Opportunities

- **Phase 2**: T003 and T004 can run in parallel (different files). T006 is a single-file edit that can run at any point in Phase 2 before T007 (no ordering dependency). T005 depends on T003's failing tests; T007 depends on T004's failing tests and T005's Diagnostic type; T008 depends on T007's `validateOutputMode`.
- **Phase 3 (US1)**: T009 and T010 can run in parallel (different test files). T011 depends on both sets of failing tests and unblocks T012. T013 is a disjoint-file task that can run any time after T007.
- **Phase 4 (US2)**: T014 is the only test task (all in `cmd/output_test.go`); T015 and T016 are sequential (tag the struct, then wire the command).
- **Phase 5 (US3)**: T017, T018, T019 can all run in parallel (distinct test cases in `cmd/output_test.go`). T020 and T021 can run in parallel (different files). T022 and T023 can run in parallel (different command files) — both depend on T020 and T021 respectively.
- **Phase 6 (Polish)**: T024, T025, T026, T027 all parallelizable (distinct files). T028 and T029 are manual verification steps — can interleave. T030 is a single command. T031 is the final gate.

---

## Parallel Example: Foundational Phase (Phase 2)

```bash
# Launch two TDD test tasks together:
Task: "Write failing tests in internal/fleet/diagnostics_test.go per T003"
Task: "Write failing tests in cmd/output_test.go per T004"

# T006 can also run in parallel (single-file edit on internal/fleet/deploy.go, no dep on T003/T004):
Task: "Add json:\"...\" tags to WorkflowOutcome in internal/fleet/deploy.go per T006"

# Then T005, T007, T008 sequentially (each depends on the previous).
```

## Parallel Example: User Story 3 Phase

```bash
# Launch all three TDD test tasks together:
Task: "Write failing sync tests in cmd/output_test.go per T017"
Task: "Write failing upgrade single-repo tests in cmd/output_test.go per T018"
Task: "Write failing upgrade --all NDJSON tests in cmd/output_test.go per T019"

# Then launch the two tag-adding tasks in parallel:
Task: "Add JSON tags to SyncResult per T020"
Task: "Add JSON tags to UpgradeResult per T021"

# Then launch the two command-branch tasks in parallel:
Task: "Update cmd/sync.go JSON branch per T022"
Task: "Update cmd/upgrade.go JSON branch per T023"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001, T002 — just baselines).
2. Complete Phase 2: Foundational (T003–T008 — shared types, helpers, flag). **CRITICAL — blocks all stories.**
3. Complete Phase 3: User Story 1 (T009–T013).
4. **STOP and VALIDATE**: quickstart §1 (`list -o json`) and §8 (text byte-identity) pass. `make ci` green.
5. **Deploy/demo**: MVP shipped. Agents can consume `gh-aw-fleet list -o json`.

### Incremental Delivery

1. Setup + Foundational (Phases 1+2) → foundation ready.
2. US1 → `list -o json` works → MVP demoable.
3. US2 → `deploy -o json` works with warnings → expanded coverage.
4. US3 → `sync -o json`, `upgrade -o json`, `upgrade --all -o json` NDJSON → full coverage.
5. Polish (Phase 6) → release-ready.

Each story increment can merge independently. Text-mode byte-identity (FR-014) means zero risk to existing text-mode consumers at any increment.

### Parallel Team Strategy

With multiple developers (after Phase 2 completes):

1. Developer A: US1 (list + list_result + subcommand rejection)
2. Developer B: US2 (DeployResult tags + deploy command)
3. Developer C: US3 (SyncResult + UpgradeResult + NDJSON — the most involved phase)

No file conflicts: the commands and types are disjoint. Shared test file `cmd/output_test.go` requires light coordination (distinct test-function names per phase; conventional Go test naming avoids conflicts).

---

## Notes

- [P] tasks = different files, no dependencies on uncommitted work.
- [Story] label maps task to specific user story for traceability.
- Each user story is independently completable and testable.
- TDD discipline: verify tests fail before implementing (`go test ./cmd/ -run TestOutputFlag` must fail before T008; then pass after).
- Commit after each task or logical group (per repo convention: Conventional Commits, scope `cli` or `output`; e.g., `feat(cli): add --output flag and JSON envelope writer`).
- Stop at any Phase 3/4/5 checkpoint to validate story independently.
- Avoid: vague tasks, cross-story file conflicts (none expected by design), skipping T028–T031 gate (non-negotiable per user memory `feedback_local_gate.md`).
