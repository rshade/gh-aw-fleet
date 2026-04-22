# Phase 1 Data Model: Structured Logging

**Feature**: 002-add-zerolog-logging
**Date**: 2026-04-20

This feature has no persistent state (no database, no config file, no cache). The "data model" here describes the runtime entities that shape log events and the closed allowlist they obey. All entities live in memory for the lifetime of one CLI invocation.

---

## Entity 1: Logger Configuration

**Purpose**: Captures the single decision made in `PersistentPreRunE` about how log events should be serialized and filtered for the lifetime of this process.

| Field | Type | Source | Notes |
|---|---|---|---|
| `level` | enum: `trace` \| `debug` \| `info` \| `warn` \| `error` | `--log-level` flag (default `info`) | FR-001 |
| `format` | enum: `console` \| `json` | `--log-format` flag (default `console`) | FR-002 |

**Lifecycle**: Immutable after `internal/log.Configure(level, format)` returns. No runtime mutation from any subcommand.

**Validation**:
- Invalid `level`: reject via `zerolog.ParseLevel` error wrapped in `fmt.Errorf("invalid --log-level %q: %w", ...)`; cobra's default error path writes the message to stderr plain-text and exits non-zero (FR-004, Q4 clarification).
- Invalid `format`: reject via explicit check (`console`/`json` only); same error path.

**Out of scope**: Env-var binding, config-file loading, per-subcommand override. These are explicitly deferred in spec Assumptions.

---

## Entity 2: Log Event

**Purpose**: A single structured record emitted to stderr. Serialized as a line-delimited JSON object (format `json`) or a compact human-readable line (format `console`).

### Allowlisted fields (FR-016)

Every field that appears in a log event MUST come from this closed set. Adding a field requires an explicit edit to this list AND a reviewer check against the no-secrets principle.

| Field | Type | When present | Source |
|---|---|---|---|
| `level` | string | always | zerolog (auto) |
| `time` | RFC3339 string | always | zerolog (auto, via `zerolog.TimeFieldFormat = time.RFC3339`) |
| `message` | string | always | call-site `.Msg(...)` or `.Msgf(...)` |
| `error` | string | when the event represents an error | call-site `.Err(err)` — zerolog auto-serializes as top-level `error` field (FR-017) |
| `repo` | string | warning and error events about a specific repository | call-site `.Str("repo", r)` — e.g., `owner/name` |
| `workflow` | string | events about a specific workflow within a repo | call-site `.Str("workflow", w)` |
| `clone_dir` | string | events about a scratch clone at `/tmp/gh-aw-fleet-*` | call-site `.Str("clone_dir", d)` |
| `tool` | string | subprocess summary events | call-site `.Str("tool", "git"\|"gh"\|"gh aw")` |
| `subcommand` | string | subprocess summary events | call-site `.Str("subcommand", "push"\|"pr create"\|...)` |
| `exit_code` | int | subprocess summary events | call-site `.Int("exit_code", cmd.ProcessState.ExitCode())` |
| `duration` | integer (milliseconds) | subprocess summary events | call-site `.Dur("duration", time.Since(start))` — zerolog's default `.Dur()` writer renders as an integer number of milliseconds, not a Go-duration string. Operators filter with `jq '.duration > 5000'`. |
| `hint` | string | diagnostic hint events | call-site `.Str("hint", hintText)` — full hint text in its own field so `jq '.hint'` works |

### Explicitly forbidden fields

- Raw `cmd.Args` slice, `cmd.Env` slice, or any `[]string` capture of a subprocess invocation.
- URL-bearing strings (remote URLs, `gh` tokens, pre-signed URLs).
- Full error dumps that embed credentials (`err.Error()` strings should never carry tokens — any credential-bearing error from `gh` must be redacted at the call site before reaching the logger).

### Serialization shape

**JSON mode** (single line):
```json
{"level":"warn","time":"2026-04-20T14:30:22-07:00","repo":"acme/api","message":"secret missing: DEPLOY_TOKEN"}
```

**Console mode** (multi-column compact):
```text
2026-04-20T14:30:22-07:00 WRN secret missing: DEPLOY_TOKEN repo=acme/api
```

---

## Entity 3: Subprocess Summary (specialization of Log Event)

**Purpose**: Emitted at `debug` level (FR-011, Q2 clarification) after each external tool invocation returns, to give operators a post-hoc timing and exit-code record without duplicating the live-teed stdout/stderr.

**Required fields**: `level=debug`, `time`, `message="subprocess exited"`, `tool`, `subcommand`, `exit_code`, `duration`.
**Optional fields**: `repo`, `workflow`, `clone_dir` (attached by the caller via `extraFields` when the subprocess is associated with a specific repo operation).

**Example** (JSON mode, `--log-level=debug`):
```json
{"level":"debug","time":"2026-04-20T14:30:22-07:00","tool":"gh","subcommand":"aw upgrade","exit_code":0,"duration":1234,"repo":"acme/api","clone_dir":"/tmp/gh-aw-fleet-xyz","message":"subprocess exited"}
```

**NOT in a subprocess summary**: the raw command line, the environment, any URL passed as an argument, or the captured stdout/stderr (which is already visible via the live tee — FR-012).

---

## Entity 4: Diagnostic Hint (specialization of Log Event)

**Purpose**: Emitted at `warn` level whenever `internal/fleet.CollectHints` returns a non-empty list. Each hint from the list becomes one log event, in order. The hint text is in a dedicated `hint` field (FR-010) so operators can do `jq 'select(.hint) | .hint'` to enumerate remediations across a multi-repo run.

**Required fields**: `level=warn`, `time`, `message="diagnostic hint"`, `hint`, `repo`.
**Typical optional fields**: `workflow`, `clone_dir`.

The existing plaintext `hint: <text>` line in tabwriter output (see `cmd/deploy.go:88`, `cmd/sync.go:141`, `cmd/upgrade.go:111`) is preserved — the structured event is emitted *alongside* it so operators who grep stdout don't lose the signal. This is a deliberate choice: removing the tabwriter line would change stdout on failure paths and violate SC-001 for those runs.

---

## Entity 5: Warning Event (specialization of Log Event)

**Purpose**: Events that previously rendered as `⚠ WARNING:` lines on stdout via the tabwriter are now `warn`-level structured log events on stderr. Two concrete call sites exist today:

| Call site | Fields |
|---|---|
| `cmd/deploy.go` "Actions secret ... is not set" (line 98 pre-feature) | `level=warn`, `repo`, `secret` (where `secret` is the missing-secret name — note: `secret` is proposed as a new allowlist entry and needs explicit addition to FR-016's list). Alternative: collapse into `message` only. |
| `cmd/sync.go` "Drift detected" (line 77 pre-feature) | `level=warn`, `repo`, `drift` (list of drifted workflow file names) — `drift` is likewise a proposed allowlist addition. |

**Decision (accepted 2026-04-20 during T003)**: The allowlist is extended to include `secret` and `drift`. `secret` is the *name* of a missing Actions secret (public-ish config), not its value; `drift` is a list of workflow file paths relative to `.github/workflows/`. Neither is sensitive under the security model. Spec FR-016 and the Log event entity have been updated accordingly. Proposed additions in `contracts/log-event.md` and entity 5 here are **accepted**.

---

## Field source-of-truth summary

| Field | Allowlisted? | Source at runtime |
|---|---|---|
| `level`, `time` | yes | zerolog global |
| `message` | yes | call-site `Msg`/`Msgf` |
| `error` | yes | `Err(err)` |
| `repo` | yes | caller (ResolvedRepo / ResolvedWorkflow) |
| `workflow` | yes | caller (ResolvedWorkflow.Name) |
| `clone_dir` | yes | caller (`/tmp/gh-aw-fleet-*` path) |
| `tool`, `subcommand` | yes | caller (string literal at exec site) |
| `exit_code`, `duration` | yes | execlog helper |
| `hint` | yes | `internal/fleet.CollectHints` output |
| `secret`, `drift` | **pending allowlist review** | caller (deploy/sync warning call sites) |

No other fields are permitted. Any future field addition follows the same explicit-allowlist procedure.
