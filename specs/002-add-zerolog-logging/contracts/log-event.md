# Contract: Log Event Shape

**Feature**: 002-add-zerolog-logging
**Spec FRs**: FR-005, FR-006, FR-007, FR-009, FR-010, FR-011, FR-016, FR-017

This contract pins the shape of log events emitted to stderr. It is the machine-observable interface exposed to operators parsing stderr with `jq`, grep, or CI log collectors.

---

## Required top-level fields (every event)

| Field | Type | Notes |
|---|---|---|
| `level` | string | One of `trace`, `debug`, `info`, `warn`, `error`. Lowercase. |
| `time` | string | RFC3339 format, e.g. `2026-04-20T14:30:22-07:00`. Set via `zerolog.TimeFieldFormat = time.RFC3339` at configuration time. |
| `message` | string | Human-readable short description. MAY be duplicated with `error` content on error events per FR-017. |

---

## Optional fields (closed allowlist — FR-016)

Only these field names are permitted in a log event. Any other field name is a regression.

| Field | Type | When present |
|---|---|---|
| `error` | string | Error events (`level=error` or `level=warn` carrying an error cause). Contains `err.Error()` output. Populated by zerolog's `.Err(err)` chain step. |
| `repo` | string | Events about a specific target repository. Format: `owner/name`. |
| `workflow` | string | Events about a specific workflow within a repo. Format: workflow file base name without `.md` (e.g., `daily-news`). |
| `clone_dir` | string | Events about a scratch clone. Format: absolute path, typically `/tmp/gh-aw-fleet-<random>`. |
| `tool` | string | Subprocess summary events. Values: `git`, `gh`, `gh aw`. |
| `subcommand` | string | Subprocess summary events. Short label for the invoked subcommand, e.g., `push`, `pr create`, `aw upgrade`, `api`. |
| `exit_code` | integer | Subprocess summary events. Exit status of the invoked process. `0` for success, positive for failure. |
| `duration` | integer | Subprocess summary events. Milliseconds, emitted as a JSON number via zerolog's default `.Dur(...)` field writer. Operators filter numerically (e.g., `jq 'select(.duration > 5000)'`). |
| `hint` | string | Diagnostic hint events. The full hint text from `internal/fleet.CollectHints`. |

### Accepted additions (ratified 2026-04-20 during T003)

| Field | Type | When present |
|---|---|---|
| `secret` | string | Missing-Actions-secret warning in `cmd/deploy.go`. Value is the secret *name* (e.g., `DEPLOY_TOKEN`), never the secret value. |
| `drift` | array of strings | Drift warning in `cmd/sync.go`. Values are workflow file base names. |

Both values are non-sensitive (public config names / file names). The allowlist in FR-016 has been extended accordingly; the Log event entity in the spec and in data-model.md reflects the final set.

---

## Forbidden fields

The following field shapes MUST NEVER appear in any log event:

- Raw subprocess argv (any full or partial copy of `cmd.Args`).
- Raw subprocess environment (any full or partial copy of `cmd.Env`).
- URLs that carry credentials: `x-access-token:...@github.com/...`, pre-signed URLs with tokens in query strings, Bearer tokens, PATs (`ghp_`, `github_pat_`), etc.
- The contents of files captured via `io.MultiWriter(os.Stderr, &buf)` — those are already shown live to the operator and are not re-logged as structured content.

---

## Error event convention (FR-017)

When an event represents an underlying Go error, the event MUST satisfy BOTH:

1. A top-level `error` field containing the error's `.Error()` string.
2. The same error text appended to the `message` string.

**Canonical call-site pattern**:
```go
log.Error().Err(err).Msgf("deploy failed: %s", err)
```

This produces (in JSON mode):
```json
{"level":"error","time":"...","error":"gpg failed to sign","message":"deploy failed: gpg failed to sign"}
```

**Why both**: JSON consumers filter on `.error` (`jq 'select(.error)'`). Console consumers read `.message` alone and need self-contained human-readable text. The two representations are derived from the same error value at a single call site, so they cannot drift.

**Anti-pattern to avoid**: `log.Error().Err(err).Msg("deploy failed")` — this leaves the error text absent from `message`. Operators reading the console output see `deploy failed` with no error detail on the same line. Reject in code review.

---

## JSON mode example events

### Warning (missing secret)
```json
{"level":"warn","time":"2026-04-20T14:30:22-07:00","repo":"acme/api","secret":"DEPLOY_TOKEN","message":"Actions secret not set on repo"}
```

### Warning (drift)
```json
{"level":"warn","time":"2026-04-20T14:30:22-07:00","repo":"acme/api","drift":["legacy-workflow","one-off-experiment"],"message":"Drift detected: workflows on disk not declared in fleet.json"}
```

### Diagnostic hint
```json
{"level":"warn","time":"2026-04-20T14:30:22-07:00","repo":"acme/api","hint":"Workflow uses `mount-as-clis`, an unreleased gh-aw feature. ...","message":"diagnostic hint"}
```

### Subprocess summary (debug)
```json
{"level":"debug","time":"2026-04-20T14:30:22-07:00","tool":"gh","subcommand":"aw upgrade","exit_code":0,"duration":2345,"repo":"acme/api","clone_dir":"/tmp/gh-aw-fleet-xyz","message":"subprocess exited"}
```

### Error (fatal exit)
```json
{"level":"error","time":"2026-04-20T14:30:22-07:00","error":"fleet.json: repo not found: acme/unknown","message":"fatal: fleet.json: repo not found: acme/unknown"}
```

---

## Console mode (informational)

Console mode is intended for human reading and is not a machine contract. A typical line layout:
```text
2026-04-20T14:30:22-07:00 WRN Actions secret not set on repo repo=acme/api secret=DEPLOY_TOKEN
```

zerolog's `ConsoleWriter` is deterministic given the same event but the exact spacing is an implementation detail. Tests that assert on console output should check for the presence of expected fields rather than exact formatting.

---

## jq filtering idioms operators can rely on

| Intent | Filter |
|---|---|
| All events about one repo | `jq 'select(.repo=="acme/api")'` |
| All errors | `jq 'select(.error)'` |
| All diagnostic hints | `jq 'select(.hint) \| .hint'` |
| Slow subprocesses | `jq 'select(.duration) \| select(.duration > 5000)'` |
| Drifted repos | `jq 'select(.drift) \| {repo, drift}'` |

These idioms are part of the contract — the field names and semantics MUST remain stable across future minor versions so operators' scripts don't break.
