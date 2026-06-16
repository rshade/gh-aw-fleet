---
description: "Task list for Renovate Config Conflict Scanner (Advisory)"
---

# Tasks: Renovate Config Conflict Scanner (Advisory)

**Input**: Design documents from `/specs/012-renovate-conflict-scanner/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/scanner-contract.md, quickstart.md

**Tests**: INCLUDED. The spec's acceptance criteria explicitly require fixtures
(present-correct, present-missing-each-rule, absent, malformed) and `make ci`
passing; the `internal/fleet/security` package is already fixture-tested. Test tasks
are therefore first-class here, written before the implementation they cover.

**Organization**: Tasks grouped by user story (US1 Rule A → US2 Rule B → US3
quiet-and-safe). Stories are logically independent and independently testable, but
US1–US3 all edit the same two files (`renovate.go`, `renovate_test.go`), so they are
implemented **sequentially**; the parallelizable work is fixtures and the
constants/diag wiring.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1 / US2 / US3 (user-story phases only)
- All paths are repository-relative.

## Path Conventions

Single Go module. Feature lives in `internal/fleet/security/` with the additive
diag-code pair in `internal/fleet/fleetdiag/` mirrored by `internal/fleet/diagnostics.go`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Anchor the new scanner file so the package keeps compiling as foundation lands.

- [X] T001 Create scanner skeleton `internal/fleet/security/renovate.go`: `// Package`-consistent file with the unexported `renovateScanner` struct, `newRenovateScanner()` constructor, and a stub `Scan(ctx context.Context, cloneDir string) []Finding` returning `nil` (mirrors the unexported `structuralScanner` shape in `structural.go`). Must `go build ./...` clean.

**Checkpoint**: Package compiles with an inert scanner present.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared probe/parse/registration/diag machinery every user story needs.

**⚠️ CRITICAL**: No user-story detection work can begin until this phase is complete.

- [X] T002 [P] Add Renovate rule-ID constants to `internal/fleet/security/constants.go`: `ruleIDRenovateGhAwActionsNotDisabled = "fleet.renovate.gh-aw-actions-not-disabled"`, `ruleIDRenovateLockfileNotDisabled = "fleet.renovate.lockfile-not-disabled"`, `ruleIDRenovateParseError = "fleet.renovate.parse-error"`, and `rulePrefixRenovate = "fleet.renovate."` (add to the existing rule-ID and prefix const blocks, keeping alphabetical grouping).
- [X] T003 [P] Add the diag code `DiagSecurityRenovate = "security_renovate"` (with a godoc sentence) to `internal/fleet/fleetdiag/diag.go`, and mirror it as `DiagSecurityRenovate = fleetdiag.DiagSecurityRenovate` in the alias block of `internal/fleet/diagnostics.go`. (One family code; per-rule granularity rides on `Fields["rule_id"]` per existing convention.)
- [X] T004 Implement `probeRenovateConfig(cloneDir string) (rel string, full string, found bool)` in `internal/fleet/security/renovate.go`: probe order `renovate.json` → `renovate.json5` → `.renovaterc` → `.renovaterc.json` → `.renovaterc.json5` → `.github/renovate.json` → `.github/renovate.json5`, first match wins, returning the clone-relative slash-form path for `Finding.File`. (depends on T001)
- [X] T005 Implement the parse layer in `internal/fleet/security/renovate.go`: minimal `renovateConfig` + `packageRule` structs (fields per data-model.md), reading bytes → `hujson.Standardize` → `json.Unmarshal`; tolerate scalar-or-array matcher values (so a single-string matcher is not a parse error). Return `(cfg, parseErr)`. (depends on T001)
- [X] T006 Wire the scanner into the registry in `internal/fleet/security/finding.go`: add `newRenovateScanner()` to `defaultScanners()`, and add a `case strings.HasPrefix(ruleID, rulePrefixRenovate): return fleetdiag.DiagSecurityRenovate` arm to `diagCodeForRuleID()`. (depends on T002, T003)
- [X] T007 [P] Create the baseline fixture `internal/fleet/security/testdata/security/renovate/correct/renovate.json` containing BOTH canonical rules (Rule A + Rule B blocks from research.md Decision 6) — the shared "no findings" baseline for all stories.

**Checkpoint**: Scanner is registered (emits nothing yet), parse/probe ready, diag code routable. User-story detection can begin.

---

## Phase 3: User Story 1 - Warn on missing gh-aw-actions disable rule (Priority: P1) 🎯 MVP

**Goal**: When a Renovate config is present but does not disable `gh-aw-actions`
package updates, emit one `LOW` finding (`fleet.renovate.gh-aw-actions-not-disabled`)
quoting the exact Rule A remediation block.

**Independent Test**: Run the scanner against `missing-gh-aw-actions/` → exactly one
`LOW` Rule A finding with the canonical block as `Remedy`; against `correct/` → no
Rule A finding.

### Tests for User Story 1

> Write first; expect failure against the inert/foundational scanner.

- [X] T008 [US1] Add Rule A test cases to `internal/fleet/security/renovate_test.go`: `missing-gh-aw-actions/` → exactly 1 finding, `RuleID == fleet.renovate.gh-aw-actions-not-disabled`, `Severity == SeverityLow`, `File == "renovate.json"`, `Remedy` equals the canonical Rule A block byte-for-byte; `correct/` → 0 Rule A findings. (depends on T009)

### Implementation for User Story 1

- [X] T009 [P] [US1] Create fixture `internal/fleet/security/testdata/security/renovate/missing-gh-aw-actions/renovate.json` (Rule B present, Rule A absent).
- [X] T010 [US1] Implement Rule A intent detection + emission in `internal/fleet/security/renovate.go`: a rule is present if any `packageRules[]` entry with `enabled:false` has a package matcher (`matchPackageNames`/`matchPackagePatterns`/`matchPackagePrefixes`/`matchDepNames`/`matchDepPatterns`) value containing substring `gh-aw-actions`, OR top-level `ignoreDeps[]` contains such an entry. When absent, append a `LOW` `ruleIDRenovateGhAwActionsNotDisabled` finding with the canonical Rule A block as `Remedy`. (Repo-wide `enabled:false` is handled once by the T016 short-circuit, not here.) (depends on T004, T005)

**Checkpoint**: MVP — a deficient config surfaces the Rule A advisory end-to-end (stderr + JSON `warnings[].code == security_renovate` + PR section), deploy still succeeds.

---

## Phase 4: User Story 2 - Warn on missing lock-file exclusion rule (Priority: P2)

**Goal**: When a Renovate config is present but does not exclude the generated
`*.lock.yml` files, emit one `LOW` finding (`fleet.renovate.lockfile-not-disabled`)
quoting the exact Rule B remediation block.

**Independent Test**: Run the scanner against `missing-lockfile/` → exactly one
`LOW` Rule B finding with the canonical block as `Remedy`; against `correct/` → no
Rule B finding.

### Tests for User Story 2

- [X] T011 [US2] Add Rule B test cases to `internal/fleet/security/renovate_test.go`: `missing-lockfile/` → exactly 1 finding, `RuleID == fleet.renovate.lockfile-not-disabled`, `Severity == SeverityLow`, `Remedy` equals the canonical Rule B block byte-for-byte; `correct/` → 0 Rule B findings. (depends on T012)

### Implementation for User Story 2

- [X] T012 [P] [US2] Create fixture `internal/fleet/security/testdata/security/renovate/missing-lockfile/renovate.json` (Rule A present, Rule B absent).
- [X] T013 [US2] Implement Rule B intent detection + emission in `internal/fleet/security/renovate.go`: a rule is present if any `packageRules[]` entry with `enabled:false` has a file matcher (`matchFileNames`/`matchPaths`) value containing substring `.lock.yml`, OR top-level `ignorePaths[]` contains such an entry. When absent, append a `LOW` `ruleIDRenovateLockfileNotDisabled` finding with the canonical Rule B block as `Remedy`. (Repo-wide `enabled:false` is handled once by the T016 short-circuit, not here.) (depends on T010 — same file)

**Checkpoint**: Both conflict rules detected; a config missing both yields exactly two `LOW` findings (SC-001).

---

## Phase 5: User Story 3 - Stay silent and safe when nothing is actionable (Priority: P3)

**Goal**: No config → silence; correct/equivalent → silence; malformed → one `INFO`
note, never a panic/error/block; probe order honored.

**Independent Test**: Run against absent / `correct/` / `equivalent-forms/` /
`comments/` (JWCC) → 0 findings; `malformed/` → 1 `INFO`
`fleet.renovate.parse-error`; a clone with two config files → only the first per
probe order is inspected.

### Tests for User Story 3

- [X] T014 [US3] Add safety/edge test cases to `internal/fleet/security/renovate_test.go`: no config → 0; `correct/` → 0; `missing-both/` → 2 `LOW`; `equivalent-forms/` → 0; `comments/` (JWCC) → 0; `disabled/` (root `enabled: false`) → 0; `malformed/` → exactly 1 `INFO` `fleet.renovate.parse-error`; probe-order via `t.TempDir()` with both `renovate.json` and `.github/renovate.json` → only `renovate.json` inspected; confirm `.renovaterc` and `.github/renovate.json5` locations are honored when sole; assert the discovered config file is byte-identical before vs. after `Scan` (read-only — FR-009). (depends on T015)

### Implementation for User Story 3

- [X] T015 [P] [US3] Create fixtures under `internal/fleet/security/testdata/security/renovate/`: `missing-both/renovate.json`, `equivalent-forms/renovate.json` (uses `ignoreDeps` + `ignorePaths` and/or alternate match keys to satisfy both rules), `comments/renovate.json5` (JWCC comments + trailing commas, both rules present), `disabled/renovate.json` (top-level `enabled: false`), `malformed/renovate.json` (unparseable).
- [X] T016 [US3] Complete the safety paths in `internal/fleet/security/renovate.go`: parse failure (T005's `parseErr`) → single `INFO` `ruleIDRenovateParseError` finding (message names the error, `File` = probed path); a top-level `enabled:false` short-circuits the whole scan to no findings (the single place repo-wide disable is handled, evaluated before the Rule A/B checks per data-model.md's flow); absent config returns `nil`; scanner never returns an error and never panics (Scanner contract). (depends on T010, T013 — same file)

**Checkpoint**: All C2 contract rows (scanner-contract.md) covered; scanner is quiet, safe, and non-blocking across every input.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T017 Run the full local gate `make ci` (fmt-check, vet, lint, test) — or `~/go/bin/golangci-lint run ./...` + `go test ./...` — and fix any findings; verify `go.mod` gained no new direct dependency and neither `cmd.SchemaVersion` nor `fleet.SchemaVersion` changed (FR-014).
- [X] T018 [P] Quickstart dry-run validation (NO `--apply`): `go run . deploy <owner/repo>` against a clone with a deficient `renovate.json`; confirm the `LOW` finding(s) appear on stderr, in `--output json` `.warnings[] | select(.code=="security_renovate")`, and in the PR `## Security Findings` section, and that the deploy still reports success (advisory, non-blocking — FR-007). Repeat the dry-run for `sync` and `upgrade` (both call `security.Run`, so the same `security_renovate` finding(s) must surface) to confirm FR-013's deploy/sync/upgrade reach.
- [X] T019 [P] Document the new scanner: add a one-line note to the security-scanner description in `AGENTS.md`/`CLAUDE.md` (the slice-006 registry now includes a Renovate scanner; new `security_renovate` diag code) and a `012-renovate-conflict-scanner` entry under "Active Technologies"/"Recent Changes".
- [X] T020 [P] OPTIONAL (separate scope, not part of acceptance): add the Rule A and Rule B blocks to the fleet's own `renovate.json` so this repo stops tripping its own scanner.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (T001)**: no dependencies.
- **Foundational (T002–T007)**: after Setup. T002, T003, T007 are mutually `[P]`; T004 & T005 are sequential (same file, after T001); T006 after T002+T003. **Blocks all user stories.**
- **US1 (T008–T010)** → **US2 (T011–T013)** → **US3 (T014–T016)**: each after Foundational. Sequential among themselves because T010/T013/T016 all edit `renovate.go` and T008/T011/T014 all edit `renovate_test.go`.
- **Polish (T017–T020)**: after the stories you intend to ship.

### Within Each User Story

- The fixture task ([P]) can be done first/concurrently (separate file).
- The test task is authored before the detection task and should fail until it lands.
- Detection/emission task edits `renovate.go`.

### Parallel Opportunities

- Foundational: `T002`, `T003`, `T007` together.
- Per story: the fixture task (`T009`, `T012`, `T015`) is `[P]` against everything except its own story's test.
- Polish: `T018`, `T019`, `T020` together (and `T017` first as the gate).
- Cross-story parallelism is NOT available — US1/US2/US3 share `renovate.go` and `renovate_test.go`.

---

## Parallel Example: Foundational

```bash
# After T001, launch the independent foundational tasks together:
Task: "T002 Add Renovate rule-ID constants in internal/fleet/security/constants.go"
Task: "T003 Add DiagSecurityRenovate in fleetdiag/diag.go + mirror in diagnostics.go"
Task: "T007 Create baseline fixture testdata/security/renovate/correct/renovate.json"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1 Setup (T001).
2. Phase 2 Foundational (T002–T007) — CRITICAL, blocks stories.
3. Phase 3 US1 (T008–T010).
4. **STOP & VALIDATE**: deficient config → Rule A `LOW` finding across all three surfaces; deploy still green.

### Incremental Delivery

1. Setup + Foundational → scanner live (emits nothing).
2. US1 → Rule A advisory (MVP).
3. US2 → Rule B advisory (both-missing now → 2 findings).
4. US3 → quiet/safe completeness (malformed INFO, equivalence, probe order).
5. Polish → `make ci`, dry-run validation, docs, optional self-fix.

---

## Notes

- `LOW` already renders end-to-end (`render.go` reserves the bucket) — no render change needed.
- No edits to `deploy.go`/`sync.go`/`upgrade.go`/`cmd/` — registration in `defaultScanners()` auto-wires all three surfaces (plan.md / contracts C4).
- Remedy text MUST be byte-for-byte the research.md Decision 6 blocks (FR-010 / contract C5).
- `make ci` must pass before "done"; no new direct dep; no schema-version bump.
- Commit after each task or logical group (commits require explicit user go-ahead in this repo).
