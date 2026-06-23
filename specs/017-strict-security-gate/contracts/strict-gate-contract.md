# Contract: Strict Security Gate

This CLI has no network API. The strict gate contract is the user-visible CLI flag
behavior plus the internal policy helper that prevents mutation when strict mode
finds blocking Layer 1 findings.

## C1 - CLI flags

The following commands accept a new boolean flag:

| Command | Flag | Default | Meaning |
|---------|------|---------|---------|
| `deploy <repo>` | `--strict` | false | Fail if HIGH Layer 1 security findings are present. |
| `sync <repo>` | `--strict` | false | Fail if HIGH Layer 1 security findings are present. |
| `upgrade [repo|--all]` | `--strict` | false | Fail if HIGH Layer 1 security findings are present. |

Help text must disambiguate this from compile-strict. Suggested wording:

```text
Fail when HIGH Layer 1 security findings are present (does not change gh aw compile --strict)
```

No read-only commands (`list`, `status`, `consumption`) gain the flag.

## C2 - Internal option propagation

`cmd` must pass the flag into `internal/fleet`:

```go
type SecurityOpts struct {
    Strict bool // block on HIGH non-promptinj security findings
}

type DeployOpts struct {
    ...
    Security SecurityOpts
}

type SyncOpts struct {
    ...
    Security SecurityOpts
}

type UpgradeOpts struct {
    ...
    Security SecurityOpts
}
```

`Sync` must pass `opts.Security` into any delegated `Deploy` call.

## C3 - Blocking predicate

The gate blocks only when all conditions are true:

```text
SecurityOpts.Strict == true
finding.Severity == security.SeverityHigh
!strings.HasPrefix(finding.RuleID, "promptinj:")
```

Non-blocking cases:

| Finding set | Strict result |
|-------------|---------------|
| Empty | proceed |
| INFO/LOW/MEDIUM only | proceed |
| HIGH `promptinj:*` only | proceed |
| HIGH non-`promptinj:*`, strict false | proceed advisory-only |
| HIGH non-`promptinj:*`, strict true | abort |

The gate must not mutate the finding slice, sort order, severities, messages, or
remedies.

## C4 - Abort placement by command

### `deploy`

Order:

```text
prepare clone
ensure gh aw init
gh aw add / compile-added workaround
preflight checks
security.Run
strict gate
dry-run return OR apply compile/manifest/commit/push/PR
```

Strict abort occurs before `runCompileStrictIfNeeded`, manifest write, branch/stage,
commit, push, and PR creation.

### `sync`

Order:

```text
prepare clone
ensure gh aw init
compute drift/missing
if missing workflows:
  delegate to Deploy with SecurityOpts
else:
  security.Run directly
  strict gate
  return dry-run OR prune/commit/push
```

Strict abort occurs before any prune commit/push. If delegated Deploy blocks, Sync
propagates that strict error and preserves the same clone.

### `upgrade`

Order:

```text
prepare clone
optional audit path (strict no-op)
ensure gh aw init
gh aw upgrade + update
check conflicts
get changed files
security.Run
strict gate
no-change/dry-run return OR compile/manifest/commit/push/PR
```

Strict abort occurs before compile-strict, manifest write, branch/stage, commit,
push, and PR creation.

## C5 - Error contract

When the gate blocks, return a typed error whose message contains:

- the phrase `strict security gate`;
- the blocking HIGH finding count;
- the repo slug;
- the unblock path: fix findings or re-run without `--strict`;
- the `findings.json` path when written.

Example:

```text
strict security gate blocked 2 HIGH Layer 1 findings for acme/widgets; fix the findings or re-run without --strict to proceed advisory-only (findings saved to /tmp/gh-aw-fleet-acme-widgets-123/findings.json)
```

The command must return non-zero after rendering existing output surfaces.

## C6 - Findings rendering contract

Strict mode must not suppress existing output. For a blocked run:

- stderr contains one warning per finding via `emitSecurityFindingWarnings`;
- JSON mode includes finding diagnostics in `warnings[]`;
- text mode prints the normal command summary available in the result;
- apply-mode PR body is not created because no PR is created.

No `cmd.SchemaVersion` bump. No `fleet.SchemaVersion` bump.

## C7 - Breadcrumb contract

On strict abort:

```text
<clone>/findings.json
```

must exist and contain a JSON array of every finding from the run. The array uses the
existing `security.Finding` JSON field names:

```json
[
  {
    "rule_id": "fleet.permissions.write-on-schedule",
    "severity": 3,
    "file": ".github/workflows/daily.md",
    "line": 5,
    "message": "Workflow has write permissions and a schedule trigger",
    "remedy": "Restrict permissions to read-only or remove the schedule trigger."
  }
]
```

The clone must be preserved after strict abort, including dry-run temp clones that
would otherwise be removed.

## C8 - `upgrade --all` behavior

Non-strict behavior remains unchanged.

Strict behavior:

- Text mode stops at the first repo whose gate blocks.
- JSON mode emits complete per-repo envelopes through the blocked repo, then stops.
- The blocked repo's envelope contains the normal result and warning diagnostics.
- The command returns the strict error as the process error.

This is a strict-only override of the non-strict NDJSON continue-after-error loop.

## C9 - Non-contract / out of scope

- No per-finding ignore file.
- No config-level strict policy.
- No MEDIUM/LOW promotion.
- No prompt-injection classifier.
- No scanner rule content changes.
- No compile-strict behavior changes.
