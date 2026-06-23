# Quickstart: Strict Security Gate

How to validate the strict gate during implementation. Assumes the standard local
toolchain from `AGENTS.md`: Go 1.26.4 and the full `make ci` gate.

## Build & gate

```bash
go build ./...
go vet ./...
go test ./internal/fleet/security/... ./internal/fleet/... ./cmd/...
make ci
```

`make ci` is required before the implementation is considered done.

## Unit validation matrix

Cover the pure gate helper first:

| Case | Findings | Strict | Expected |
|------|----------|--------|----------|
| clean | `[]` | true | proceed |
| lower severity only | LOW/MEDIUM/INFO | true | proceed |
| one HIGH Layer 1 | HIGH `gitleaks:*` | true | abort, count 1 |
| multiple HIGH Layer 1 | two HIGH non-`promptinj:` | true | abort, count 2 |
| HIGH prompt injection only | HIGH `promptinj:*` | true | proceed |
| mixed prompt and Layer 1 | HIGH `promptinj:*` plus HIGH `fleet.*` | true | abort, count 1 |
| strict disabled | HIGH non-`promptinj:` | false | proceed advisory-only |

Also cover:

- `findings.json` contains every finding, not just blockers.
- Strict abort preserves a dry-run temp clone instead of deleting it.
- Strict error text includes count, repo, unblock path, and breadcrumb path.
- Breadcrumb write failure is reported without losing the blocking count.

## Command validation

### `deploy --strict`

Expected behavior with a HIGH Layer 1 finding:

```bash
go run . deploy <owner/repo> --strict
```

- exits non-zero;
- prints/records the finding on stderr;
- writes `<clone>/findings.json`;
- preserves the clone;
- does not say "Re-run with --apply..." as a successful dry-run completion if the
  command returns a strict error.

Apply-path test must use seams or an explicitly approved live run:

```bash
go run . deploy <owner/repo> --strict --apply
```

Expected:

- exits non-zero before commit/push/PR;
- creates zero PRs;
- still renders findings before the error.

### `sync --strict`

Validate three paths:

```bash
go run . sync <owner/repo> --strict
go run . sync <owner/repo> --strict --apply
go run . sync <owner/repo> --strict --prune --apply
```

Expected:

- missing-workflow paths delegate to Deploy and block before deploy PR creation;
- clean/drift-only dry-run paths still scan and block on existing HIGH findings;
- prune-only apply blocks before commit/push when current workflows have HIGH
  findings.

### `upgrade --strict`

Validate normal dry-run:

```bash
go run . upgrade <owner/repo> --strict
```

Expected:

- exits non-zero when upgraded clone content has a blocking HIGH finding;
- writes `findings.json`;
- preserves the clone;
- does not change compile-strict behavior.

Validate fleet-wide fail-fast:

```bash
go run . upgrade --all --strict
go run . upgrade --all --strict --output json
```

Expected:

- text mode stops at the first blocked repo;
- JSON mode emits complete envelopes through the blocked repo and then stops;
- non-strict `upgrade --all --output json` remains continue-all.

### `upgrade --audit --strict`

```bash
go run . upgrade <owner/repo> --audit --strict
```

Expected:

- audit behavior is unchanged;
- strict has no scanner findings to evaluate on the audit path.

## JSON surface checks

For a blocked run:

```bash
go run . deploy <owner/repo> --strict --output json
```

Expected:

- process exits non-zero;
- JSON envelope is still written;
- `warnings[]` contains the security finding diagnostics;
- `result.security_findings` is present when the scanner ran;
- schema version remains unchanged.

Example checks:

```bash
go run . deploy <owner/repo> --strict --output json \
  | jq '.warnings[] | select(.fields.severity == "HIGH")'
```

## Documentation checks

Implementation tasks must update:

- `README.md`;
- `docs/src/content/docs/reconcile.md`;
- `skills/fleet-deploy/SKILL.md`;
- `skills/fleet-upgrade-review/SKILL.md`.

The docs must distinguish:

- security strict gate: `gh-aw-fleet deploy|sync|upgrade --strict`;
- compile strict: the existing `gh aw compile --strict` policy surfaced as
  `compile-strict:` in command output.

## Done criteria

- Blocking predicate, prompt-injection carve-out, and breadcrumb behavior covered by
  unit tests.
- `deploy`, `sync`, `upgrade`, and `upgrade --all` strict paths covered.
- User-facing docs and relevant skills updated.
- `make ci` passes locally.
