---
description: "Task list for Dependabot Config Conflict Scanner (Advisory)"
---

# Tasks: Dependabot Config Conflict Scanner (Advisory)

**Input**: Design documents from `/specs/013-dependabot-conflict-scanner/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/scanner-contract.md, quickstart.md

**Tests**: INCLUDED. The spec's acceptance criteria explicitly require fixtures
(correct-ignore-present, github-actions-block-without-ignore, gomod-only, absent,
malformed) and `make ci` passing; the `internal/fleet/security` package is already
fixture-tested (see `renovate_test.go`). Test tasks are therefore first-class here.

**Organization**: Tasks grouped by user story (US1 warn-on-unprotected → US2
quiet-and-safe). The two stories are logically independent and independently testable,
but both edit the same two files (`dependabot.go`, `dependabot_test.go`), so they are
implemented **sequentially**; the parallelizable work is fixtures and the
constants/diag wiring. **This is the sibling of 012 (Renovate); the key divergence is
one conflict rule (not two), a YAML parse path (`yaml.v3`, not `hujson`), and a remedy
that must carry the name-only caveat (FR-004).**

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1 / US2 (user-story phases only)
- All paths are repository-relative.

## Path Conventions

Single Go module. Feature lives in `internal/fleet/security/` with the additive
diag-code pair in `internal/fleet/fleetdiag/` mirrored by `internal/fleet/diagnostics.go`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Anchor the new scanner file so the package keeps compiling as foundation lands.

- [X] T001 Create scanner skeleton `internal/fleet/security/dependabot.go`: `// Package`-consistent file with the unexported `dependabotScanner` struct, `newDependabotScanner()` constructor, and a stub `Scan(_ context.Context, cloneDir string) []Finding` returning `nil` (mirrors the unexported `renovateScanner` shape in `renovate.go`; `ctx` is unused, so name it `_` like `renovate.go`/`gitleaks.go` to avoid an unused-param lint nit). Must `go build ./...` clean.

**Checkpoint**: Package compiles with an inert scanner present.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared probe/parse/registration/diag machinery both user stories need.

**⚠️ CRITICAL**: No user-story detection work can begin until this phase is complete.

- [X] T002 [P] Add Dependabot rule-ID constants to `internal/fleet/security/constants.go`: `ruleIDDependabotGhAwActionsNotIgnored = "fleet.dependabot.gh-aw-actions-not-ignored"`, `ruleIDDependabotParseError = "fleet.dependabot.parse-error"`, and `rulePrefixDependabot = "fleet.dependabot."` (add to the existing rule-ID and prefix const blocks, keeping alphabetical grouping).
- [X] T003 [P] Add the diag code `DiagSecurityDependabot = "security_dependabot"` (with a godoc sentence) to `internal/fleet/fleetdiag/diag.go`, and mirror it as `DiagSecurityDependabot = fleetdiag.DiagSecurityDependabot` in the alias block of `internal/fleet/diagnostics.go`. (One family code; per-rule granularity rides on `Fields["rule_id"]` per existing convention.)
- [X] T004 Implement `probeDependabotConfig(cloneDir string) (rel string, full string, found bool)` in `internal/fleet/security/dependabot.go`: probe order `.github/dependabot.yml` → `.github/dependabot.yaml`, first match wins, returning the clone-relative slash-form path for `Finding.File` (research.md Decision 1). (depends on T001)
- [X] T005 Implement the parse layer in `internal/fleet/security/dependabot.go`: minimal `dependabotConfig` / `dependabotUpdate` / `dependabotIgnore` structs (fields per data-model.md), reading bytes → `yaml.Unmarshal` (do NOT enable `KnownFields` — unknown keys must be ignored). Return `(cfg, parseErr)`; a YAML *syntax* error becomes `parseErr` (research.md Decision 2). (depends on T001)
- [X] T006 Wire the scanner into the registry in `internal/fleet/security/finding.go`: add `newDependabotScanner()` to `defaultScanners()` (after `newRenovateScanner()`), and add a `case strings.HasPrefix(ruleID, rulePrefixDependabot): return fleetdiag.DiagSecurityDependabot` arm to `diagCodeForRuleID()`. While editing that function, fix its now-stale godoc ("maps a Finding's RuleID to one of the **nine** new diag codes" — already inaccurate after Renovate, and this scanner adds another); make it count-agnostic. (depends on T002, T003)
- [X] T007 [P] Create the baseline fixture `internal/fleet/security/testdata/security/dependabot/correct/.github/dependabot.yml`: a `github-actions` ecosystem entry whose `ignore` list covers the gh-aw family (the three canonical `dependency-name` entries) — the shared "no findings" baseline.

**Checkpoint**: Scanner is registered (emits nothing yet), parse/probe ready, diag code routable. User-story detection can begin.

---

## Phase 3: User Story 1 - Warn when Dependabot could bump the compiler-coupled action (Priority: P1) 🎯 MVP

**Goal**: When a Dependabot config has a `github-actions` ecosystem entry that does
not ignore the gh-aw action family (and is not otherwise disabled), emit one `LOW`
finding (`fleet.dependabot.gh-aw-actions-not-ignored`) **per unprotected entry**,
quoting the canonical `ignore:` block AND the name-only caveat (FR-004). Detection is
intent-based: an entry is protected if any `ignore[].dependency-name` contains
substring `gh-aw`, OR `open-pull-requests-limit` is `0` (research.md Decision 4).

**Independent Test**: Run the scanner against `missing-ignore/` → exactly one `LOW`
finding with the canonical ignore block + caveat as `Remedy`; against
`multiple-unprotected/` → two `LOW` findings (one per `github-actions` entry, labeled
by `directory`); against `correct/` → no finding.

### Tests for User Story 1

> Write first; expect failure against the inert/foundational scanner.

- [X] T008 [US1] Add US1 test cases to `internal/fleet/security/dependabot_test.go`: `missing-ignore/` → exactly 1 finding, `RuleID == fleet.dependabot.gh-aw-actions-not-ignored`, `Severity == SeverityLow`, `File == ".github/dependabot.yml"`, and `Remedy` contains both the canonical `ignore:` block (the three `dependency-name` entries) and the name-only caveat substring (FR-004); `partial-unrelated-ignore/` → exactly 1 finding (same RuleID/Severity — proves the substring gate still fires when an `ignore` block is present but covers only an unrelated dep; spec edge case "Partial / unrelated ignore present"); `multiple-unprotected/` → exactly 2 findings (per-entry, distinct `Message` naming each `directory`); `correct/` → 0 findings. (authored before the T011 implementation and expected to fail until it lands; needs fixtures T009, T010)

### Implementation for User Story 1

- [X] T009 [P] [US1] Create two US1 fixtures: `internal/fleet/security/testdata/security/dependabot/missing-ignore/.github/dependabot.yml` — a single `github-actions` entry with **no `ignore` block at all** (mirror the live `rshade/finfocus#1246` shape); and `internal/fleet/security/testdata/security/dependabot/partial-unrelated-ignore/.github/dependabot.yml` — a single `github-actions` entry whose `ignore` lists an **unrelated** dependency (e.g. `actions/checkout`) but not the gh-aw family. Both yield exactly 1 LOW finding (the second exercises the spec's "Partial / unrelated ignore present" edge case).
- [X] T010 [P] [US1] Create fixture `internal/fleet/security/testdata/security/dependabot/multiple-unprotected/.github/dependabot.yml`: two `github-actions` entries (distinct `directory` values), both lacking a gh-aw ignore.
- [X] T011 [US1] Implement github-actions detection + emission in `internal/fleet/security/dependabot.go`: iterate `updates[]` entries with `package-ecosystem == "github-actions"`; treat an entry as **protected** if any `ignore[].dependency-name` contains substring `gh-aw` OR `open-pull-requests-limit == 0`; for each unprotected entry append a `LOW` `ruleIDDependabotGhAwActionsNotIgnored` finding whose `Message` names the entry's `directory` (or first `directories` value) and whose `Remedy` is the canonical `ignore:` block + name-only caveat (research.md Decision 7). A config with no `github-actions` entry yields nothing here. (depends on T004, T005)

**Checkpoint**: MVP — a deficient config surfaces the advisory end-to-end (stderr + JSON `warnings[].code == security_dependabot` + PR section), deploy still succeeds; multiple unprotected entries produce one finding each.

---

## Phase 4: User Story 2 - Stay silent and safe when there is nothing actionable (Priority: P2)

**Goal**: No config → silence; no `github-actions` entry (gomod-only) → silence;
already-protected (covering ignore, wildcard ignore, or `open-pull-requests-limit: 0`)
→ silence; malformed YAML → one `INFO` note, never a panic/error/block; probe order
honored; read-only.

**Independent Test**: Run against absent / `gomod-only/` / `wildcard-ignore/` /
`pr-limit-zero/` → 0 findings; `malformed/` → exactly 1 `INFO`
`fleet.dependabot.parse-error`; a clone with both `.yml` and `.yaml` → only `.yml`
inspected; the discovered config file is byte-identical before vs. after `Scan`.

### Tests for User Story 2

- [X] T012 [US2] Add safety/edge test cases to `internal/fleet/security/dependabot_test.go`: no config → 0; `gomod-only/` → 0; `wildcard-ignore/` → 0; `pr-limit-zero/` → 0; `malformed/` → exactly 1 `INFO` `fleet.dependabot.parse-error` (`File` = probed path); probe-order via `t.TempDir()` with both `.github/dependabot.yml` and `.github/dependabot.yaml` → only `.yml` inspected; assert the discovered config file is byte-identical before vs. after `Scan` (read-only — FR-009). (authored before the T014 implementation and expected to fail until it lands; needs fixtures T013)

### Implementation for User Story 2

- [X] T013 [P] [US2] Create fixtures under `internal/fleet/security/testdata/security/dependabot/`: `gomod-only/.github/dependabot.yml` (only a `gomod` ecosystem — no `github-actions` entry), `wildcard-ignore/.github/dependabot.yml` (`github-actions` entry whose `ignore` uses a `github/gh-aw-actions*` wildcard `dependency-name`), `pr-limit-zero/.github/dependabot.yml` (`github-actions` entry with `open-pull-requests-limit: 0` and no ignore), `malformed/.github/dependabot.yml` (unparseable YAML).
- [X] T014 [US2] Complete the safety paths in `internal/fleet/security/dependabot.go`: parse failure (T005's `parseErr`) → single `INFO` `ruleIDDependabotParseError` finding (message names the error, `File` = probed path); absent config returns `nil`; a config with no `github-actions` entry returns `nil`; scanner never returns an error and never panics (Scanner contract). (depends on T011 — same file)

**Checkpoint**: All C2 contract rows (scanner-contract.md) covered; scanner is quiet, safe, and non-blocking across every input.

---

## Phase 5: Polish & Cross-Cutting Concerns

- [X] T015 Run the full local gate `make ci` (fmt-check, vet, lint, test) — or `~/go/bin/golangci-lint run ./...` + `go test ./...` — and fix any findings; verify `go.mod` gained no new direct dependency (yaml.v3 reused) and neither `cmd.SchemaVersion` nor `fleet.SchemaVersion` changed (FR-014).
- [X] T016 [P] Quickstart dry-run validation (NO `--apply`): `go run . deploy <owner/repo>` against a clone with a deficient `.github/dependabot.yml` (e.g. `rshade/finfocus`); confirm the `LOW` finding appears on stderr, in `--output json` `.warnings[] | select(.code=="security_dependabot")`, and in the PR `## Security Findings` section, and that the deploy still reports success (advisory, non-blocking — FR-007). Repeat the dry-run for `sync` and `upgrade` (both call `security.Run`, so the same `security_dependabot` finding must surface) to confirm FR-013's deploy/sync/upgrade reach. **Note (this session)**: the cross-surface wiring is verified at the unit level — `TestRunRegistersAllScanners` proves the scanner is in the default `security.Run` registry (so `deploy`/`sync`/`upgrade` all include it) and `TestDependabotScanner_DiagCodeIsDependabotFamily` proves findings project to `security_dependabot`; the stderr/JSON/PR rendering is the shared pipeline already validated for the Renovate sibling. The **live** `go run . deploy <repo>` dry-run was not executed here (needs the authenticated fleet env + `fleet.local.json`); run it manually to confirm against a real deficient config.
- [X] T017 [P] Document the new scanner: add a one-line note to the security-scanner description in `AGENTS.md`/`CLAUDE.md` (the slice-006 registry now includes a Dependabot scanner; new `security_dependabot` diag code; one conflict rule, name-only protection). The `013-dependabot-conflict-scanner` "Active Technologies" entry was already added by `/speckit-plan`; this task adds the architectural prose note alongside the Renovate one.
- [X] T018 [P] OPTIONAL (separate scope, not part of acceptance): confirm the fleet's own `.github/dependabot.yml` (gomod-only) produces zero findings; no change is required unless a `github-actions` entry is later added — at which point the gh-aw `ignore` block would be needed for parity.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (T001)**: no dependencies.
- **Foundational (T002–T007)**: after Setup. T002, T003, T007 are mutually `[P]`; T004 & T005 are sequential (same file, after T001); T006 after T002+T003. **Blocks both user stories.**
- **US1 (T008–T011)** → **US2 (T012–T014)**: each after Foundational. Sequential among themselves because T011/T014 both edit `dependabot.go` and T008/T012 both edit `dependabot_test.go`.
- **Polish (T015–T018)**: after the stories you intend to ship.

### Within Each User Story

- The fixture tasks ([P]) can be done first/concurrently (separate files).
- The test task is authored before the detection task and should fail until it lands.
- Detection/emission tasks edit `dependabot.go`.

### Parallel Opportunities

- Foundational: `T002`, `T003`, `T007` together.
- US1: fixtures `T009` and `T010` together (separate files), before/independent of the impl `T011`.
- US2: fixtures in `T013` are a single multi-file task (all `[P]` against everything except its own story's test).
- Polish: `T016`, `T017`, `T018` together (and `T015` first as the gate).
- Cross-story parallelism is NOT available — US1/US2 share `dependabot.go` and `dependabot_test.go`.

---

## Parallel Example: Foundational

```bash
# After T001, launch the independent foundational tasks together:
Task: "T002 Add Dependabot rule-ID constants in internal/fleet/security/constants.go"
Task: "T003 Add DiagSecurityDependabot in fleetdiag/diag.go + mirror in diagnostics.go"
Task: "T007 Create baseline fixture testdata/security/dependabot/correct/.github/dependabot.yml"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1 Setup (T001).
2. Phase 2 Foundational (T002–T007) — CRITICAL, blocks stories.
3. Phase 3 US1 (T008–T011).
4. **STOP & VALIDATE**: deficient config → `LOW` finding across all three surfaces (with the name-only caveat in the remedy); deploy still green; multiple unprotected entries → one finding each.

### Incremental Delivery

1. Setup + Foundational → scanner live (emits nothing).
2. US1 → unprotected-entry advisory (MVP), per-entry emission.
3. US2 → quiet/safe completeness (malformed INFO, gomod-only silence, wildcard / pr-limit-zero equivalence, probe order, read-only).
4. Polish → `make ci`, dry-run validation, docs, optional parity check.

---

## Notes

- `LOW` already renders end-to-end (`render.go` reserves the bucket; the Renovate scanner activated it in 012) — no render change needed.
- No edits to `deploy.go`/`sync.go`/`upgrade.go`/`cmd/` — registration in `defaultScanners()` auto-wires all three surfaces (plan.md / contracts C4).
- **One conflict rule only** — Dependabot cannot ignore by file glob, so there is no `*.lock.yml` analog to the Renovate sibling's Rule B; the remedy must educate the operator about this (FR-004 / contract C5).
- Detection markers (research.md Decision 4): protection present if any `ignore[].dependency-name` contains substring `gh-aw`, or `open-pull-requests-limit == 0`.
- `Finding.Remedy`'s godoc reads "single-sentence operator guidance," but this scanner (like the shipped Renovate scanner) puts a multi-line `ignore:` block + caveat there — consistent with existing practice. Optionally widen that godoc to "operator guidance (may be multi-line)"; not blocking.
- `make ci` must pass before "done"; no new direct dep (yaml.v3 reused); no schema-version bump.
- Commit after each task or logical group (commits require explicit user go-ahead in this repo).
