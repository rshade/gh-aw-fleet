# Phase 0 Research: Strict Security Gate

The feature spec has no unresolved clarification markers. This research records the
implementation decisions that bind the strict gate to the current scanner,
command, JSON, and work-dir behavior.

## Decision 1 - `--strict` remains the flag name

**Decision**: Add `--strict` to `deploy`, `sync`, and `upgrade`. The flag is
invocation-only, defaults false, and is not persisted in fleet config.

**Rationale**: The issue explicitly asks for `--strict`, and the spec resolves the
name collision with compile-strict through documentation rather than renaming. The
existing compile-strict policy is about `gh aw compile --strict`; this feature is
about the Layer 1 security gate. Help text must name that distinction.

**Alternatives considered**:
- `--security-strict` or `--fail-on-high`. Rejected because it diverges from the
  issue and spec language.
- Config-level strict policy. Rejected by FR-012; team-wide defaults remain
  advisory.

## Decision 2 - Gate after scanner output, before mutation

**Decision**: Evaluate strict mode immediately after `security.Run` populates the
command result and before any compile, manifest, branch, commit, push, or PR path.

**Rationale**: The command layer already renders `SecurityFindings` on stderr and in
JSON envelopes after the fleet operation returns. Returning `res, StrictSecurityError`
lets text and JSON modes render the same findings they render today, then return a
non-zero error. Placing the gate in `internal/fleet` keeps mutation prevention close
to the mutation gates and avoids duplicating policy in `cmd`.

**Command placement**:
- `Deploy`: after `res.SecurityFindings = security.Run(ctx, res.CloneDir)`, before
  the dry-run return and before `runCompileStrictIfNeeded`.
- `Upgrade`: after `res.SecurityFindings = security.Run(ctx, res.CloneDir)`, before
  no-change/dry-run/apply branches.
- `Sync`: for paths that delegate to `Deploy`, pass strict through so Deploy blocks
  before its commit/PR path. For direct sync paths (clean, drift-only, prune-only),
  run/evaluate the scanner before any prune commit.

**Alternatives considered**:
- Gate in `cmd` after printing. Rejected because apply-mode mutation may already
  have happened by then.
- Gate inside `internal/fleet/security.Run`. Rejected because scanner output must
  stay advisory and reusable; strict is command policy, not detection.

## Decision 3 - Blocking predicate

**Decision**: A blocking finding is:

```text
finding.Severity == security.SeverityHigh
AND NOT strings.HasPrefix(finding.RuleID, "promptinj:")
```

The gate keys on severity tier, not count or rule family. The `promptinj:` prefix is
the Layer 3 carve-out.

**Rationale**: Current scanner findings do not carry an explicit layer enum. The
spec defines `promptinj:` as the future Layer 3 identifier, so a prefix carve-out is
the minimum stable contract. Everything else at HIGH severity remains Layer 1 for
gate purposes.

**Alternatives considered**:
- Add a layer field to `security.Finding`. Rejected because it would expand the JSON
  finding shape and introduce more churn than the spec requires.
- Block on diagnostic code families. Rejected because the spec says severity tier,
  not rule count or family, and because future Layer 1 HIGH rules should block
  automatically.

## Decision 4 - Breadcrumb file and clone preservation

**Decision**: On strict abort, write `findings.json` at the work-dir clone root. The
file is a JSON array of every finding from that run, not just blockers. Preserve the
clone even for dry-run temp clones.

**Rationale**: FR-007 and SC-004 require a post-mortem artifact. Writing all findings
keeps context for lower-severity related issues and matches the existing
`security_findings` result surface. Dry-run currently removes throwaway clones; strict
abort is a failure breadcrumb exception and must cancel cleanup.

**File contract**:
- Path: `<clone>/findings.json`.
- Format: pretty or compact JSON array of `security.Finding` values.
- Permissions: normal user-readable project artifact permissions are acceptable.
- Write failure: return an error that still makes clear strict mode found blockers;
  tests should cover the happy path and one write-failure path.

**Alternatives considered**:
- Write only blocking findings. Rejected because the spec asks for all findings.
- Put the file under `.github/aw/`. Rejected because the spec fixes clone-root
  `findings.json`, and root is easier for operators to inspect.

## Decision 5 - Error and output behavior

**Decision**: Return a typed strict error carrying the blocking count, breadcrumb
path, and blocking findings. Its `Error()` message must state the count and the
unblock path: fix the findings or re-run without `--strict` to proceed
advisory-only.

**Rationale**: A typed error gives tests and JSON/text emitters a stable way to
recognize the strict gate without string matching. The message satisfies FR-008 and
is usable as the standard failure hint in current envelope logic.

**Output behavior**:
- Text mode prints the normal command result and emits existing warning lines, then
  returns the strict error.
- JSON mode emits the normal single-repo envelope (or NDJSON line) with the existing
  result shape and `warnings[]`, then returns the strict error.
- No `cmd.SchemaVersion` bump; warnings and error/hint entries are additive.
- No `fleet.SchemaVersion` bump; config shape is unchanged.

**Alternatives considered**:
- Add a new `strict_blocked` field to result JSON. Rejected for this slice; the
  existing non-zero error plus warnings already expresses failure. A future AX error
  envelope phase can standardize richer error metadata.

## Decision 6 - `sync` path handling

**Decision**: `sync --strict` uses two gate paths:

1. If sync delegates to `Deploy` for missing workflows, pass strict through to
   `Deploy` so generated workflow content is scanned and blocked before deploy
   commits/pushes/opens a PR.
2. If sync does not delegate to `Deploy` (clean, drift-only, or prune-only), scan the
   current clone directly and evaluate before returning or before any prune commit.

**Rationale**: Sync has more than one mutation path. Delegating missing-workflow
cases to Deploy avoids duplicate scans and catches findings in newly added workflow
content. Direct sync paths still need strict behavior because pre-existing workflows
can contain HIGH findings and prune-only apply can otherwise commit before the
current end-of-function scan.

**Alternatives considered**:
- Gate only on `SyncResult.SecurityFindings` after `applyDeployOrPrune`. Rejected
  because prune-only apply would commit before the gate.

## Decision 7 - `upgrade --all --strict` fail-fast, including JSON mode

**Decision**: Strict mode is a policy gate that stops `upgrade --all` at the first
blocked repo. In text mode this matches existing `UpgradeAll` fail-fast behavior. In
JSON mode, emit complete envelopes through the blocked repo, then stop; non-strict
`upgrade --all -o json` continues to emit one line per repo per the existing NDJSON
contract.

**Rationale**: FR-010 requires fail-fast when strict blocks a repo. Existing JSON
mode normally continues after errors to satisfy the 003 NDJSON contract, but `--strict`
is a new opt-in policy mode with no existing callers. Emitting the blocked repo's
envelope before stopping preserves machine-readable context while honoring the gate.

**Alternatives considered**:
- Continue all repos in JSON mode. Rejected because it violates the strict fail-fast
  requirement.
- Stop before emitting the blocked repo's line. Rejected because it would hide the
  findings from JSON consumers.

## Decision 8 - `upgrade --audit --strict`

**Decision**: Accept the flag combination but leave audit behavior unchanged. The
audit path does not materialize or scan workflow content today, so strict has no
findings to evaluate there.

**Rationale**: The spec examples and requirements target normal `upgrade` dry-runs
and applies, not `--audit`. Rejecting `--audit --strict` would add validation churn
without improving the Layer 1 gate.

## Decision 9 - Documentation and skills

**Decision**: Update human-facing docs and relevant operator skills in the
implementation phase:

- `README.md`: mention strict in reconcile/usage guidance.
- `docs/src/content/docs/reconcile.md`: add a strict gate section and disambiguate
  compile-strict.
- `skills/fleet-deploy/SKILL.md`: mention strict dry-run/apply behavior and
  breadcrumb inspection.
- `skills/fleet-upgrade-review/SKILL.md`: mention `upgrade --strict` and
  fail-fast behavior.

**Rationale**: The constitution requires user-facing documentation to move with
visible flag changes, and AGENTS.md says skills must be updated when command flags
or operator flows change.
