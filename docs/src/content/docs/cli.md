---
title: CLI reference
description: Every gh-aw-fleet command, flag, and exit code.
---

Complete reference for the `gh-aw-fleet` command line. For task-oriented recipes
see the How-to guides; for the config-file schema see
[Configuration](/gh-aw-fleet/configuration/).

## Global flags

These persistent flags are accepted by every command:

| Flag | Default | Description |
| --- | --- | --- |
| `--dir <path>` | `.` | Directory containing `fleet.json` / `fleet.local.json`. |
| `--log-level <level>` | `info` | Log verbosity: `trace`, `debug`, `info`, `warn`, `error`. `debug` logs every subprocess call, timing, and exit code. |
| `--log-format <format>` | `console` | Log format: `console` (human, on stderr) or `json` (structured). |
| `-o`, `--output <format>` | `text` | Output format: `text` or `json`. Not supported on `add` or `template fetch`. |

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success. For `status` and `overview`, every in-scope repo is aligned. |
| `1` | A runtime error; **or** `status`/`overview` detected a drifted or errored repo; **or** a `--strict` HIGH-finding block; **or** a declined interactive findings confirmation. |

## Commands

### `list`

```bash
gh-aw-fleet list
```

Lists tracked repos and their resolved workflow sets, including parallel `TIERS`
and `COST_CENTER` columns. Reads `fleet.json` overlaid with `fleet.local.json`
and prints the loaded mode on stderr. Takes no arguments. Supports `-o json`.

### `add <owner/repo>`

```bash
gh-aw-fleet add <owner/repo> --profile <name> [--apply]
```

Registers one repository in `fleet.local.json`. Requires exactly one
`owner/repo`. Dry-run by default. Does **not** support `-o json`.

| Flag | Description |
| --- | --- |
| `--profile <name>` | **Required.** Profile name(s) to assign; repeatable or comma-separated. |
| `--engine <name>` | Engine override (e.g. `claude`, `copilot`), validated against known engine secrets. |
| `--exclude <name>` | Workflow name to exclude from the selected profiles (repeatable). |
| `--extra-workflow <spec>` | Extra workflow outside profiles (repeatable). Accepts `name`, `owner/repo/name@ref`, or `owner/repo/.github/workflows/name.md@ref`. |
| `--apply` | Actually write `fleet.local.json` (default is dry-run). |
| `--yes` | Confirm `--apply` without the interactive prompt (required in a non-TTY). |

### `deploy <repo>`

```bash
gh-aw-fleet deploy <repo> [--apply]
```

Installs the declared workflow set into a repo via `gh aw add`, then commits,
pushes, and opens a PR. Requires exactly one repo. Dry-run by default. Supports
`-o json`.

| Flag | Description |
| --- | --- |
| `--apply` | Actually commit, push, and open a PR (default is dry-run). |
| `--force` | Overwrite existing workflow files (passes `--force` to `gh aw add`). |
| `--branch <name>` | Branch name for the deploy PR (default `fleet/deploy-<timestamp>`). |
| `--pr-title <text>` | PR title (default auto-generated). |
| `--work-dir <path>` | Deploy into an existing clone; skips clone and auto-cleanup and resumes at the commit/push gate. |
| `--strict` | Fail when HIGH Layer 1 security findings are present. Does not change `gh aw compile --strict`. |
| `--yes` | Skip the interactive security-findings confirmation prompt (findings still print). |

### `sync <repo>`

```bash
gh-aw-fleet sync <repo> [--apply]
```

Reconciles a repo to its declared profile: adds missing workflows and flags
drift. Requires exactly one repo. Dry-run by default. Supports `-o json`.

| Flag | Description |
| --- | --- |
| `--apply` | Add missing workflows and optionally prune drift (default is dry-run). |
| `--prune` | Delete drift workflow files (requires `--apply`). |
| `--force` | Overwrite existing workflow files (passes `--force` to `gh aw add`). |
| `--work-dir <path>` | Sync in an existing clone; skips clone and auto-cleanup. |
| `--strict` | Fail when HIGH Layer 1 security findings are present. |
| `--yes` | Skip the interactive security-findings confirmation prompt. |

### `upgrade [repo | --all]`

```bash
gh-aw-fleet upgrade <repo> [--apply]
gh-aw-fleet upgrade --all [--apply]
```

Bumps profile pins and runs `gh aw upgrade` / `gh aw update`. Target one repo or
pass `--all`. Dry-run by default. Supports `-o json`; `--all` emits NDJSON (one
record per repo).

| Flag | Description |
| --- | --- |
| `--apply` | Actually commit, push, and open a PR (default is dry-run). |
| `--all` | Upgrade every repo in the fleet. |
| `--audit` | Only run `gh aw upgrade --audit`; skip the upgrade. |
| `--major` | Allow major-version bumps for tag pins (passes `--major` to `gh aw update`). |
| `--force` | Update even when no changes are detected (passes `--force` to `gh aw update`). |
| `--work-dir <path>` | Upgrade in an existing clone; skips clone and auto-cleanup. |
| `--strict` | Fail when HIGH Layer 1 security findings are present. |
| `--yes` | Skip the interactive security-findings confirmation prompt. |

### `consumption [repo...]`

```bash
gh-aw-fleet consumption [repo...] [--by <axis>] [window]
```

Read-only fleet-wide AI-credit (AIC) rollup from `gh aw logs`. Pass zero or more
repos to scope it. Supports `-o json`.

| Flag | Default | Description |
| --- | --- | --- |
| `--by <axis>` | `repo` | Group-by axis: `repo`, `profile`, `cost-center`, or `workflow`. |
| `--source <src>` | `logs` | Data source: `logs` (needs no deployed report) or `artifacts` (legacy). |
| `--latest` | on | Most-recent valid report per repo. |
| `--trailing <Nd>` | — | All reports in the trailing N-day window (e.g. `7d`). |
| `--since <YYYY-MM-DD>` | — | All reports on or after the date. |
| `--budget <AIC>` | — | Flag rows whose AIC strictly exceeds this ceiling (adds an `OVER` column). |

`--latest`, `--trailing`, and `--since` are mutually exclusive.

### `overview [repo...]`

```bash
gh-aw-fleet overview [repo...] [window]
```

Read-only dashboard joining drift, run health, no-op rate, and AIC/cost. Pass
zero or more repos to scope it. Exits `1` on any drifted or errored repo; run
failures stay advisory. Supports `-o json`.

| Flag | Default | Description |
| --- | --- | --- |
| `--trailing <Nd>` | `7d` | All runs in the trailing N-day window. |
| `--since <YYYY-MM-DD>` | — | All runs on or after the date. |
| `--latest` | — | Most-recent run per workflow (the `NOOP` column is best-effort in this mode). |

`--latest`, `--trailing`, and `--since` are mutually exclusive.

### `status [repo]`

```bash
gh-aw-fleet status [repo]
```

Diffs desired (`fleet.json`) versus actual workflow refs — read-only, no clones.
Accepts at most one repo; omit it to check the whole fleet. Exits `1` if any
queried repo has drifted. Supports `-o json`.

### `template fetch`

```bash
gh-aw-fleet template fetch
```

Refreshes `templates.json` from `github/gh-aw` and `githubnext/agentics`. Takes
no arguments. Does **not** support `-o json`.
