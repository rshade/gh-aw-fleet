---
description: "Task list for feature implementation: structured logging via zerolog"
---

# Tasks: Structured Logging for Errors, Warnings, and Diagnostics

**Input**: Design documents from `/specs/002-add-zerolog-logging/`
**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/cli-flags.md](contracts/cli-flags.md), [contracts/log-event.md](contracts/log-event.md), [quickstart.md](quickstart.md)

**Tests**: Tests ARE included in this plan because the spec's Testing Strategy section explicitly requires unit tests for `internal/log.Configure`, flag-registration assertions, and an integration-style test capturing a structured warning via stderr.

**Organization**: Tasks are grouped by user story (US1, US2, US3) so each story can be implemented and validated independently, matching the spec's three-slice priority breakdown.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Every description includes the exact file path(s) touched

## Path Conventions

Single-project Go CLI. Source under `cmd/` and `internal/`. Tests colocated as `*_test.go` next to production files. Module path `github.com/rshade/gh-aw-fleet`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Dependency addition and environmental verification.

- [X] T001 Add `github.com/rs/zerolog` dependency in `go.mod` via `go get github.com/rs/zerolog@latest && go mod tidy`; commit `go.sum` changes.
- [X] T002 Verify zero new transitive dependencies: capture `go mod graph | sort` before and after T001; diff MUST show exactly one added edge (`github.com/rshade/gh-aw-fleet github.com/rs/zerolog@<tag>`). If more lines are added, back out and re-evaluate per research.md R1.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Lock the pending spec decision from Phase 1 planning (the `secret` / `drift` field allowlist) so US2 can migrate warnings without ambiguity.

**⚠️ CRITICAL**: No user-story work may begin until this phase is complete.

- [X] T003 Amend `specs/002-add-zerolog-logging/spec.md` FR-016 allowlist to include `secret` (string — missing-secret name, e.g., `DEPLOY_TOKEN`) and `drift` (array of strings — drifted workflow base names). Also extend the "Log event" entity in the Key Entities section to list the two fields. Add a one-line note in `specs/002-add-zerolog-logging/data-model.md` marking the "Proposed additions" table as **accepted**. Rationale for acceptance (record in commit message): both values are non-sensitive (public config name / public file names) and keeping them as structured fields preserves jq-filterability of the warning events.

**Checkpoint**: Spec and data-model agree on the final field allowlist. US2 warning migrations are unblocked.

---

## Phase 3: User Story 1 - Operator configures log verbosity and format from the CLI (Priority: P1) 🎯 MVP

**Goal**: Establish the logger package, the two persistent CLI flags, and the root-command wiring. After this phase, `gh-aw-fleet list --log-level=debug --log-format=json 2> /tmp/log.json` produces parseable JSON on stderr and byte-identical stdout; `--log-level=invalid` exits non-zero with a plain-text error.

**Independent Test**: Run quickstart.md steps 2, 3, 4, 5, 6. Stdout byte-identical (step 2). Stderr JSON parses (step 5). Invalid flag rejected with exit 1 and plain-text error (step 6). Debug level exposes additional stderr content (step 4).

### Tests for User Story 1 ⚠️

> Write these tests FIRST, ensure they FAIL before implementation.

- [X] T004 [P] [US1] Write `internal/log/log_test.go` covering: (a) all five valid levels (`trace`, `debug`, `info`, `warn`, `error`) parse successfully via `Configure(level, "console")`; (b) both formats (`console`, `json`) parse successfully via `Configure("info", format)`; (c) invalid level (`"shouting"`) returns non-nil error whose message contains the string `--log-level` and the offending value; (d) invalid format (`"yaml"`) returns non-nil error whose message contains `--log-format` and the offending value; (e) using `os.Pipe()` swap on `os.Stderr` per research.md R5, a `log.Info().Msg("t")` call after `Configure("info", "json")` produces a line that parses as JSON and contains `"level":"info"` and `"message":"t"`; (f) **FR-015 silencing** — after `Configure("error", "console")`, a subsequent `log.Warn().Msg("should be dropped")` produces zero bytes on the captured pipe; (g) **FR-017 dual-write** — after `Configure("info", "json")`, a `log.Error().Err(errors.New("boom")).Msgf("deploy failed: %s", errors.New("boom"))` call produces exactly one JSON line whose `.error` field equals `"boom"` AND whose `.message` field contains the substring `"boom"`. Tests MUST be serial (no `t.Parallel()`) because the global logger is process-wide.
- [X] T005 [P] [US1] Write `cmd/root_logging_test.go` (new file — `cmd/` has no existing tests today) covering: (a) `NewRootCmd()` registers `log-level` and `log-format` as persistent flags with defaults `info` and `console`; (b) constructing the root command with args `["list", "--log-level=shouting"]` and calling `.Execute()` returns a non-nil error and the error message contains `invalid --log-level` — assert the `list` subcommand's `RunE` did NOT run (use a stub subcommand or an observable side effect). Tests MUST be serial.

### Implementation for User Story 1

- [X] T006 [P] [US1] Create `internal/log/log.go` with `package log`, exposing `Configure(level, format string) error`. Implementation follows research.md R2 verbatim: parse level via `zerolog.ParseLevel`; wrap error as `fmt.Errorf("invalid --log-level %q: %w", level, err)`; switch on format to select `os.Stderr` (json) or `zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}` (console); invalid format returns `fmt.Errorf("invalid --log-format %q: must be %q or %q", format, "console", "json")`; set `zerolog.TimeFieldFormat = time.RFC3339`, `zerolog.SetGlobalLevel(lvl)`, and `zlog.Logger = zerolog.New(w).With().Timestamp().Logger()`. Add a package doc comment describing the "one-call-per-process, global-logger-replacement" contract and the FR-017 canonical pattern (`log.Error().Err(err).Msgf("…: %s", err)`). No exports beyond `Configure`.
- [X] T007 [US1] Update `cmd/root.go`: on the root `*cobra.Command`, add two persistent string flags via `root.PersistentFlags().String("log-level", "info", "Log verbosity: trace|debug|info|warn|error")` and `root.PersistentFlags().String("log-format", "console", "Log format: console|json")`. Add `root.PersistentPreRunE` that reads both flag values via `cmd.Flags().GetString(...)` and calls `log.Configure(level, format)` (import as `logpkg "github.com/rshade/gh-aw-fleet/internal/log"`), returning its error directly so cobra's default error writer prints it as plain text (satisfies FR-004 and Q4 clarification). `SilenceUsage: true` remains set; keep the existing `--dir` flag and subcommand registration untouched.
- [X] T008 [US1] Update `main.go`: replace the current two-liner error path (`fmt.Fprintln(os.Stderr, err); os.Exit(1)`) with `log.Error().Err(err).Msgf("fatal: %s", err); os.Exit(1)` (import `zlog "github.com/rs/zerolog/log"`). Rationale: produces both a top-level `error` field and the error text appended to `message` per FR-017. Keep the exit code behavior (non-zero on any error returned from `cmd.Execute()`) identical.

**Checkpoint**: Run T004 and T005 tests — all green. Run quickstart.md steps 2–6 manually — all pass. MVP complete: operators can tune verbosity, pipe JSON, and a warning emitted from any future caller will route through the configured logger.

---

## Phase 4: User Story 2 - Existing user-facing warnings carry structured context for filtering (Priority: P2)

**Goal**: The two existing `⚠ WARNING:` output sites (missing Actions secret in deploy, drift detection in sync) emit as `warn`-level structured events on stderr with `repo` and the appropriate detail field (`secret` / `drift`). Operators filtering `deploy --log-format=json 2>&1 | jq 'select(.repo=="acme/api")'` see the warning cleanly.

**Independent Test**: Add a unit-ish test that constructs a minimal deploy result with `MissingSecret` set, calls the warning-printing function with `--log-format=json` pre-configured, captures stderr, and asserts a JSON event with `level=warn`, `repo=<value>`, `secret=<value>`. Analogous test for sync drift. Per quickstart.md step 7 style.

### Tests for User Story 2 ⚠️

- [X] T009 [P] [US2] Add a test to `cmd/root_logging_test.go` (extends T005's file): call the deploy-warning print path with a constructed result carrying `MissingSecret="DEPLOY_TOKEN"` and `Repo="acme/api"`, capture stderr with an `os.Pipe` swap under `Configure("info", "json")`, and assert the emitted line parses as JSON with `level=="warn"`, `repo=="acme/api"`, `secret=="DEPLOY_TOKEN"`. Serial execution (global logger).
- [X] T010 [P] [US2] Add a parallel test in `cmd/root_logging_test.go` for the sync-drift path: construct a result with `Drift=["legacy-workflow", "experiment"]` and `Repo="acme/api"`, assert the emitted JSON event has `level=="warn"`, `repo=="acme/api"`, and `drift` is a JSON array containing both values. Serial execution.

### Implementation for User Story 2

- [X] T011 [P] [US2] Edit `cmd/deploy.go`: locate the block at roughly line 97–103 that writes `⚠ WARNING: Actions secret %q is not set on %s` and the follow-up `gh secret set` hint into the tabwriter `w`. Replace the whole block with `log.Warn().Str("repo", res.Repo).Str("secret", res.MissingSecret).Msgf("Actions secret %q is not set on %s; workflows will fail until added (gh secret set %s --repo %s)", res.MissingSecret, res.Repo, res.MissingSecret, res.Repo)`. If `res.SecretKeyURL != ""`, append via a chained `.Str("secret_key_url", res.SecretKeyURL)` — **wait**: `secret_key_url` is NOT in the FR-016 allowlist; instead, inline it into the `Msgf` only (console readable) without structured capture, since URLs are forbidden as structured fields per FR-016. Remove the now-dead tabwriter lines entirely. Stdout on warning-free runs is unchanged; stdout on warning-triggering runs loses the 3–4 line block (intentional per spec).
- [X] T012 [P] [US2] Edit `cmd/sync.go`: locate the block at roughly line 76–80 that writes `⚠️  WARNING: Drift detected.` into the tabwriter `w`. Replace with `log.Warn().Str("repo", res.Repo).Strs("drift", driftNames(res.Drift)).Msg("Drift detected: workflows on disk not declared in fleet.json")`, where `driftNames(res.Drift)` returns the base names of drifted workflow files. If the follow-up `--prune --apply` guidance line depends on `!prune`, keep that as a tabwriter line on stdout — it's operator guidance, not a warning. Consolidate so the warning is one structured event and any operator guidance remains human-readable on stdout.
- [X] T013 [US2] Grep the codebase for any remaining `⚠` or `WARNING:` lines in `cmd/*.go` that go through `fmt.Fprintln`/`fmt.Fprintf`. Confirm T011 and T012 covered every site; if a new site is discovered, migrate it following the same pattern. Record the grep command and output in the PR description as evidence for SC-007 ("no warning remains as unstructured `fmt.Fprintln`").

**Checkpoint**: Run T009, T010, plus US1 tests — all green. Manually trigger a deploy dry-run against a test repo with a missing secret under `--log-format=json 2>/tmp/w.json 1>/tmp/s.txt`, confirm `/tmp/w.json` contains a parseable warn event, confirm `/tmp/s.txt` no longer contains the `⚠ WARNING:` block.

---

## Phase 5: User Story 3 - Subprocess outcomes and diagnostic hints are captured as queryable events (Priority: P3)

**Goal**: Every `exec.CommandContext` invocation in `internal/fleet/*.go` emits a `debug`-level summary event after completion with `tool`, `subcommand`, `exit_code`, `duration` fields (plus caller-supplied `repo`/`workflow`/`clone_dir` where available). Diagnostic hints from `CollectHints` are emitted as `warn`-level structured events alongside the existing plaintext `hint:` tabwriter lines.

**Independent Test**: Run quickstart.md step 4 against a command that invokes at least one subprocess (e.g., `template fetch`), confirm at least one `DBG` / `"level":"debug"` line with the expected shape appears on stderr. For hints: trigger a known-hint pattern (e.g., `gh aw upgrade` against a repo that hits `Unknown property: mount-as-clis`) under `--log-format=json`, confirm a `warn` event with the `hint` field appears on stderr AND the plaintext `hint: ...` line still appears on stdout.

### Tests for User Story 3 ⚠️

- [X] T014 [P] [US3] Add `internal/fleet/execlog_test.go`: test that `runLogged` with a succeeding command (`exec.Command("true")`) emits one debug event with `exit_code=0` and a positive `duration`; test that `runLogged` with a failing command (`exec.Command("false")`) emits one debug event with `exit_code=1` and returns the error. Use `os.Pipe` swap on `os.Stderr` to capture output. Tests MUST be serial.
- [X] T015 [P] [US3] Add a test in `cmd/root_logging_test.go` exercising the hint-event path: construct a synthetic `errs` list containing `"Unknown property: mount-as-clis"`, call the hint-logging helper (or call `fleet.CollectHints` and iterate), under `--log-format=json` stderr, assert exactly one `warn`-level JSON event is emitted with a `hint` field whose value starts with `Workflow uses \`mount-as-clis\``. Serial execution.

### Implementation for User Story 3

- [X] T016 [US3] Create `internal/fleet/execlog.go` with the `runLogged(cmd *exec.Cmd, toolLabel, subcommandLabel string, extraFields map[string]string) error` helper per research.md R4. Implementation: `start := time.Now(); err := cmd.Run(); log.Debug().Str("tool", toolLabel).Str("subcommand", subcommandLabel).Int("exit_code", cmd.ProcessState.ExitCode()).Dur("duration", time.Since(start))…extraFields loop…Msg("subprocess exited"); return err`. Guard against a nil `cmd.ProcessState` (happens if `exec.Cmd.Start()` itself fails before the process runs) — in that case log `exit_code=-1` and skip duration. Add a package comment noting the FR-016 no-argv guarantee and why `toolLabel`/`subcommandLabel` are caller-supplied literals. The helper preserves the `io.MultiWriter(os.Stderr, &buf)` pattern (FR-012) — it does not touch `cmd.Stdout` / `cmd.Stderr`, so callers retain full control of the live tee.
- [X] T017 [P] [US3] Edit `internal/fleet/deploy.go`: replace each `exec.CommandContext(...).Run()` or equivalent direct run of cmd built locally (sites at approximately lines 185, 198, 216, 223, 235, 247, 286, 310) with `runLogged(cmd, "<tool>", "<subcommand>", fields)`. Examples: the clone site becomes `runLogged(cmd, "gh", "repo clone", map[string]string{"repo": repo, "clone_dir": dir})`; the `gh pr create` site becomes `runLogged(cmd, "gh", "pr create", map[string]string{"repo": repo})`. The `gh api` / `git diff --cached` sites that call `.Output()` or `.Run() != nil` (existence checks) are intentionally skipped — they're read-only probes and not interesting for summary events. Preserve existing error-handling behavior exactly (same error wrapping, same return paths). The `exec.CommandContext(ctx, "gh", "api", path).Run() == nil` truthiness check at line 286 stays as-is — it's a tight bool check, not a logged invocation.
- [X] T018 [P] [US3] Edit `internal/fleet/upgrade.go`: replace the `cmd.Run()` calls at approximately lines 158 (`gh aw upgrade`), 175 (`gh aw update`), 180 (`git status --porcelain` inside `checkConflicts`), 225 (second `git status --porcelain`). For the two `runUpgrade` / `runUpdate` functions that tee output via `io.MultiWriter`, `runLogged` is invoked instead of `cmd.Run()` — the MultiWriter assignment on `cmd.Stdout`/`cmd.Stderr` remains untouched. The `gh aw upgrade --audit --json` site at line 135 uses `cmd.Output()` — skip (read-only JSON probe, not interesting for summary events).
- [X] T019 [P] [US3] Edit `internal/fleet/fetch.go`: the two sites at lines 183 (`gh api path`) and 195 (`gh api -H ... path`) use `.Output()`. For consistency with T017/T018's rule of wrapping `.Run()` sites but not `.Output()` probes, **skip** fetch.go wrapping. Document this decision in the package comment of `execlog.go`: read-only probes via `.Output()` are not wrapped because they are not interesting for timing/exit summaries, they already return their data via the return value, and logging them would triple the noise at `--log-level=debug` without corresponding diagnostic value. (This converts T019 into a no-op verification task: confirm no `.Run()` calls exist in `fetch.go`. If any are added later, they should be wrapped.)
- [X] T020 [US3] Emit structured diagnostic hint events in three call sites: `cmd/deploy.go` around line 87–89 (existing `for _, h := range fleet.CollectHints(errs...) { fmt.Fprintf(w, "  hint: %s\n", h) }` loop), `cmd/sync.go` around line 140–142, and `cmd/upgrade.go` around line 110–112. For each hint in the loop body, in addition to the existing `fmt.Fprintf(w, "  hint: %s\n", h)` line (which stays — keeps stdout byte-identical on any run that does NOT hit warning paths, and preserves the operator-readable signal on stdout), add `log.Warn().Str("repo", <res.Repo or similar>).Str("hint", h).Msg("diagnostic hint")`. The plaintext line is operator-readable on stdout; the structured event is grep/jq-queryable on stderr. Both emit; they do not duplicate in the sense of FR-009 because this is *hint* surfacing, not *warning* re-routing — hints were already on stdout and stay there.

**Checkpoint**: Run T014, T015, plus all prior tests — all green. Run quickstart.md step 4 — at least one debug-level subprocess summary appears. Run a `upgrade` dry-run that hits a hint pattern — confirm both the tabwriter `hint:` line on stdout AND the structured `warn` event on stderr with `hint` field.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, skill updates, and the final CI gate. Each item affects the whole feature rather than a single story.

- [X] T021 [P] Update `CHANGELOG.md` "Unreleased" section with two entries: "Added: `--log-level` and `--log-format` persistent flags on the root command (values `trace|debug|info|warn|error` and `console|json`; defaults `info` and `console`); structured logging for errors, warnings, diagnostic hints, and subprocess summaries on stderr." and "Changed: `⚠ WARNING:` lines for missing Actions secrets (deploy) and workflow drift (sync) moved from stdout (tabwriter) to stderr (structured `warn`-level log events). Scripts that grepped stdout for `⚠ WARNING:` should switch to stderr. The `hint:` plaintext lines on stdout are unchanged and additionally emitted as structured `warn` events on stderr." Keep the existing format of CHANGELOG.md (detect format via a quick head/grep).
- [X] T022 [P] Update `skills/fleet-deploy/SKILL.md`: add a short paragraph under the troubleshooting section noting that `--log-level=debug --log-format=json 2>/tmp/log.json` gives operators per-subprocess timing and a greppable record of warnings when a deploy fails across many repos. Do NOT restructure the skill's three-turn pattern — it's unchanged.
- [X] T023 [P] Verify `CLAUDE.md` "Active Technologies" auto-update (already performed by `update-agent-context.sh` during `/speckit.plan`) reads cleanly; tidy duplicate `Go 1.25.8` entries if the script added one. Add a single line under "Architecture big-picture" about the new diagnostic layer: "Structured logging: `internal/log.Configure(level, format)` wires a zerolog global logger in root's `PersistentPreRunE`; warnings/errors/subprocess summaries emit on stderr, tabwriter status stays on stdout."
- [X] T024 Run `make ci` (fmt-check + vet + lint + test) and confirm all gates pass. This is the SC-005 acceptance bar per the project's CLAUDE.md rule. If `lint` flags new zerolog-related idioms, fix before proceeding. Do NOT skip lint even if it exceeds 5 minutes.
- [X] T025 Run the full quickstart.md validation (steps 1–9) manually. Record evidence: the diff output for step 2 showing `stdout IDENTICAL`; a sample JSON line from step 5 that parses with `jq -e '.level'`; the `diff /tmp/deps_before.txt /tmp/deps_after.txt` output from step 9 showing exactly one added zerolog edge. Attach the evidence in the PR description.
- [X] T026 Mark the requirements checklist `specs/002-add-zerolog-logging/checklists/requirements.md` complete for this implementation (add a note at the bottom with the PR URL once opened).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 Setup (T001–T002)**: No prior dependencies; blocks everything.
- **Phase 2 Foundational (T003)**: Depends on Setup; blocks US2 specifically (US2's warning migrations need the allowlist extension locked).
- **Phase 3 US1 (T004–T008)**: Depends on Setup; does NOT depend on Phase 2 (US1 establishes the logger — no warning migration yet). Blocks US2 and US3 because their log calls require the logger to exist.
- **Phase 4 US2 (T009–T013)**: Depends on US1 and Phase 2. Independent of US3.
- **Phase 5 US3 (T014–T020)**: Depends on US1. Independent of US2.
- **Phase 6 Polish (T021–T026)**: Depends on US1 + US2 + US3 being complete.

### User Story Dependencies

- **US1 (P1 — MVP)**: Requires Setup only. Blocks US2 and US3 as a runtime dependency (logger must be configured before `log.Warn` / `log.Debug` calls emit anything useful).
- **US2 (P2)**: Requires US1 + T003 (allowlist). Independent of US3 — warnings and subprocess summaries touch disjoint files (`cmd/deploy.go`/`cmd/sync.go` for US2 vs `internal/fleet/*.go` for US3).
- **US3 (P3)**: Requires US1. Independent of US2.

### Within Each User Story

- Tests (T004, T005, T009, T010, T014, T015) MUST be written and FAIL before the corresponding implementation task.
- In US1: tests (T004 parallel to T005) → implementation (T006 before T007; T008 independent after T006).
- In US2: tests (T009 parallel to T010) → implementation (T011 parallel to T012) → coverage audit (T013).
- In US3: tests (T014 parallel to T015) → execlog helper (T016) → site migrations (T017/T018/T019 parallel) → hint surfacing (T020).

### Parallel Opportunities

- **Within Phase 3 (US1)**:
  - T004 and T005 in parallel (different test files).
  - T006 standalone (blocks T007, T008).
  - T007 and T008 in parallel after T006 (different files: `cmd/root.go` vs `main.go`).
- **Within Phase 4 (US2)**:
  - T009 and T010 in parallel (both extend `cmd/root_logging_test.go` — they touch the same file; exception: one developer adds both, or mark them sequential if strict same-file rule applies). Prefer **sequential** to avoid merge-noise; remove [P] if executing strictly by the rule.
  - T011 and T012 in parallel (`cmd/deploy.go` vs `cmd/sync.go`).
- **Within Phase 5 (US3)**:
  - T014 and T015 in parallel (different files: `internal/fleet/execlog_test.go` vs `cmd/root_logging_test.go`).
  - T017, T018, T019 in parallel (three different files).
- **After US1 completes**: US2 and US3 in parallel by different developers.
- **Polish**: T021, T022, T023 in parallel (CHANGELOG.md / skills / CLAUDE.md).

---

## Parallel Example: User Story 1 post-test phase

```bash
# After tests are written (T004, T005) and confirmed failing, launch:
# T006 blocks T007 and T008, so run T006 alone first.
Task: "Create internal/log/log.go with Configure per research.md R2"

# Once T006 is done, these two run in parallel:
Task: "Wire --log-level / --log-format + PersistentPreRunE in cmd/root.go"
Task: "Route main.go error exit through log.Error().Err(err).Msgf(...)"
```

---

## Parallel Example: User Story 3 site migrations

```bash
# After T016 (execlog.go) is done, these three run in parallel:
Task: "Migrate exec sites in internal/fleet/deploy.go to runLogged"
Task: "Migrate exec sites in internal/fleet/upgrade.go to runLogged"
Task: "Verify no .Run() calls exist in internal/fleet/fetch.go (no-op if confirmed)"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 (Setup: T001–T002).
2. Complete Phase 2 (Foundational: T003).
3. Complete Phase 3 (US1: T004–T008).
4. **STOP and VALIDATE**: Run quickstart.md steps 1–6. The tool now has the flag surface, the logger package, and routes its own fatal errors through the logger. Operators can `--log-format=json` any subcommand and get at least a valid JSON stderr stream even with no warnings yet.
5. This is a shippable slice — the flag contract is in place, no behavior regressions, new structure ready to host US2 and US3.

### Incremental Delivery

1. Setup + Foundational + US1 → **MVP shippable** (flag plumbing done; no warning regressions; structure ready).
2. Add US2 → operators now get structured `warn` events with `repo` field for the two headline warning sites → **ship**.
3. Add US3 → operators get per-subprocess `debug` summaries and structured `hint` events for diagnostic patterns → **ship**.
4. Polish → CHANGELOG, docs, skills, CI gate → **final release**.

### Parallel Team Strategy

With two developers after US1 is complete:

- **Developer A**: Phase 4 (US2 — T009–T013). Disjoint files (`cmd/deploy.go`, `cmd/sync.go`, extends `cmd/root_logging_test.go`).
- **Developer B**: Phase 5 (US3 — T014–T020). Disjoint files (`internal/fleet/execlog*.go`, edits to `internal/fleet/deploy.go` / `upgrade.go` / `fetch.go`).
- Coordination point: both extend `cmd/root_logging_test.go` (T009, T010, T015). Agree on a simple rule: Developer A owns the warning-path tests; Developer B adds a hint-path test. No function-level conflicts expected.

---

## Task Format Self-Check

Every task above conforms to `- [ ] [TaskID] [P?] [Story?] Description with file path(s)`:

- Setup/Foundational/Polish tasks have NO `[Story]` label.
- US1/US2/US3 tasks each carry `[US1]` / `[US2]` / `[US3]`.
- `[P]` is applied only to tasks that touch different files and have no incomplete dependencies.
- Every task description names at least one file path (or command being run, for T001/T002/T024/T025).

---

## Notes

- `[P]` tasks = different files, no dependencies.
- Tests (T004, T005, T009, T010, T014, T015) are serial-only: they mutate the global zerolog logger. Do NOT mark them `t.Parallel()` and do NOT run them with `go test -parallel`.
- Each story is independently completable: US1 delivers the flag surface + logger package + main.go routing; US2 delivers structured warnings; US3 delivers subprocess summaries + hint structuring.
- Commit after each task or logical group. Commit messages follow Conventional Commits with scope `ci(workflows)` per the project's constitution (see `CLAUDE.md`).
- Stop at any checkpoint to validate independently before proceeding.
- Avoid: same-file parallel work, cross-story dependencies that undermine independent testability, and any tabwriter output changes outside the two warning sites in T011/T012.
