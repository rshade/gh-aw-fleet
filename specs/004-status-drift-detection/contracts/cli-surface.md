# Contract: `status` CLI Surface

**Feature**: `004-status-drift-detection` | **Date**: 2026-04-28

This document is the wire contract for `gh-aw-fleet status` from the operator's perspective. It defines the command synopsis, arguments, flags, output channels, and exit codes. Anything not specified here is undefined behavior; any change to a specified element is a breaking change.

---

## Synopsis

```text
gh-aw-fleet status [owner/repo]
```

## Description

Diff desired (`fleet.json` + `fleet.local.json`) versus actual (per-repo workflow set + pinned refs) WITHOUT cloning the target repos. Pure read-only observability. Exits non-zero if any queried repo is drifted, has unpinned workflows, or returned an error during fetch.

---

## Positional Arguments

| Position | Name | Required | Description |
|---|---|---|---|
| 1 | `owner/repo` | No | When supplied, query only this repo. The repo MUST be declared in the loaded fleet config (FR-008). When omitted, query every repo in the loaded fleet config (FR-007). |

## Flags

Status defines **no flags of its own**. It honors the following flags inherited from the root command:

| Flag | Type | Source | Behavior in `status` |
|---|---|---|---|
| `-o`, `--output` | string (`text` \| `json`) | root persistent | `text` (default): tabwriter output on stdout. `json`: single envelope on stdout. |
| `--log-level` | string | root persistent | Standard zerolog levels (`debug`, `info`, `warn`, `error`). Affects stderr only. |
| `--log-format` | string | root persistent | Standard zerolog formats (`console`, `json`). Affects stderr only. |

**No status-specific flags ship in this release.** The follow-up issue **#61** tracks demand for a future opt-in `--ref <branch>` flag.

---

## Output Channels

### stdout (text mode — default)

Per-repo tabwriter table. Recommended columns:

```text
REPO                           STATE     MISSING  EXTRA  DRIFTED  UNPINNED
rshade/gh-aw-fleet             aligned   0        0      0        0
rshade/cronai                  drifted   1        0      2        0
rshade/private-thing           errored   -        -      -        -
```

Followed (when any repo is `drifted`) by a per-workflow detail block scoped to drifted/missing/extra/unpinned items, formatted to match the existing `sync` and `deploy --dry-run` style. Errored repos get a single error line beneath the table.

The exact column widths and formatting are left to the implementation; what matters is consistency with the project's existing tabwriter outputs. The state column uses the literal strings `aligned`, `drifted`, `errored` (matching the JSON values).

### stdout (JSON mode — `-o json`)

A single `Envelope` per the `cmd.Envelope` shape (`cmd/output.go:29`), filled with `Command: "status"`, `Apply: false`, and `Result: *StatusResult`. See `contracts/json-envelope.md` for the full shape. The envelope is emitted as compact single-line JSON terminated by a newline, parseable by `jq -e`.

### stderr

- Structured zerolog events at the active log level (warnings about empty fleet, redirected repos, unrecognized labels in fleet.json, etc.).
- Subprocess summaries for `gh api` calls at `debug` level.
- The breadcrumb line `(loaded fleet.json + fleet.local.json)` (or single-source variant) follows the existing `LoadConfig` convention.

In **JSON mode**, every warning and hint that appears in the envelope's `warnings[]` / `hints[]` MUST also be emitted on stderr via zerolog (FR-011/FR-012 from spec 003 — applies transitively). Operators silencing stderr (`2>/dev/null`) get pure JSON on stdout; humans tailing the run get live diagnostics.

---

## Exit Codes

Per FR-011 and the spec's user stories:

| Exit | Meaning |
|---|---|
| 0 | Every queried repo's `drift_state` is `aligned`. No errors, no drift. |
| 1 | At least one queried repo is `drifted` OR `errored`. Includes any of: missing/extra/drifted workflows present, unpinned workflows present, repo inaccessible (404/403), rate limit hit. |
| 2 | (Reserved by Cobra for argument/flag validation errors. Status itself does not produce exit 2; it surfaces from Cobra when, e.g., `-o yaml` is supplied.) |

There is **no separate exit code** for "errored repos but no drifted repos." Operators who want that gating must parse the envelope's `warnings[]` / `hints[]` (or `result.repos[].drift_state == "errored"`) in JSON mode.

---

## Argument validation

| Input | Behavior |
|---|---|
| Zero positional args | Fleet-wide path. |
| One positional arg matching a `repos[].repo` entry in cfg | Single-repo path; query only that repo. |
| One positional arg NOT in cfg | Exit non-zero with error: `repo "owner/name" is not declared in fleet config`. **No GitHub API calls are issued** (FR-008). In JSON mode, emit a `result: null` envelope with the error in `hints[]` via `preResultFailureEnvelope`. |
| Two or more positional args | Exit non-zero with Cobra usage error. |
| `-o yaml` (or any value other than `text` / `json`) | Caught by `validateOutputMode` in root's `PersistentPreRunE`; exits non-zero before status's `RunE` runs. |

---

## Required setup

Status requires:

- A loadable fleet config (`fleet.json`, `fleet.local.json`, or both). If neither exists, `LoadConfig` fails and status surfaces the error via `preResultFailureEnvelope` in JSON mode or a plain stderr error in text mode.
- An authenticated `gh` CLI session (`gh auth status`). Inherited from the operator's environment; not validated by status itself — the first failing `gh api` call surfaces auth issues via `CollectHints`.
- Network access to `api.github.com`.

Status does **NOT** require:

- `gh aw` to be installed. (Status does not invoke it. If a future code path does, a hint will surface via the existing diagnostics layer.)
- A clone, working directory, or scratch directory.
- Any environment variables beyond what `gh` itself reads.

---

## Backward compatibility

- The existing stub at `cmd/stubs.go` `newStatusCmd()` returns the literal error `status: not yet implemented`. After this feature lands, the same invocation produces real output (or, in failure cases, structured errors). **No CLI surface promise is broken** — the stub never claimed any contract.
- The stub explicitly rejects `-o json` via `rejectJSONMode`. After this feature lands, status accepts and honors `-o json`. Any operator who built a workaround for the rejection (e.g., `gh-aw-fleet status 2>&1 | head -1`) gets a different error path on upgrade — but the rejection's existence was a placeholder, not a contract. No deprecation cycle is needed.

---

## Examples

```bash
# Fleet-wide quick check
gh-aw-fleet status

# CI gate before deploy
gh-aw-fleet status && gh-aw-fleet deploy --apply rshade/gh-aw-fleet

# Single-repo drill-down
gh-aw-fleet status rshade/gh-aw-fleet

# JSON for dashboards
gh-aw-fleet status -o json | jq '.result.repos | map(select(.drift_state == "drifted")) | length'

# Debug logging
gh-aw-fleet status --log-level debug --log-format console
```
