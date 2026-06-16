# Quickstart: Renovate Config Conflict Scanner

How to build, test, and exercise the scanner during development. Assumes the
standard local toolchain (Go 1.26.4, `make ci` gate per AGENTS.md).

## Build & gate

```bash
go build ./...                       # compile
go vet ./...                         # vet
go test ./internal/fleet/security/...   # scanner unit tests (fast)
make ci                              # full gate: fmt-check vet lint test  (REQUIRED before "done")
```

> Per the project rule, run the local lint via `~/go/bin/golangci-lint run ./...`
> if `make lint` resolves the wrong binary; it can exceed 5 minutes.

## Where the code lives

| File | Role |
|------|------|
| `internal/fleet/security/renovate.go` | the scanner (probe, parse, detect, emit) |
| `internal/fleet/security/renovate_test.go` | table-driven tests over the fixtures |
| `internal/fleet/security/constants.go` | rule IDs + `rulePrefixRenovate` |
| `internal/fleet/security/finding.go` | `defaultScanners()` registration + `diagCodeForRuleID()` mapping |
| `internal/fleet/fleetdiag/diag.go` | `DiagSecurityRenovate` code |
| `internal/fleet/diagnostics.go` | alias mirror of the new code |
| `internal/fleet/security/testdata/security/renovate/<case>/renovate.json[5]` | per-case fixtures (one subdir per case) |

## Exercise the unit tests against fixtures

The scanner is pure and offline — point it at a fixture directory and assert the
findings. Mirror the existing `security_test.go` table-driven style:

```go
got := newRenovateScanner().Scan(context.Background(), "testdata/security/renovate/<case-dir>")
```

Cases to cover (one per row of the C2 contract table):

| Fixture | Expectation |
|---------|-------------|
| `correct/renovate.json` | 0 findings |
| `missing-gh-aw-actions/renovate.json` | 1 × `LOW` gh-aw-actions |
| `missing-lockfile/renovate.json` | 1 × `LOW` lockfile |
| `missing-both/renovate.json` | 2 × `LOW` |
| `disabled/renovate.json` | 0 findings (root `enabled:false` disables Renovate repo-wide) |
| `equivalent-forms/renovate.json` | 0 findings (alternate syntax / `ignoreDeps` / `ignorePaths`) |
| `comments/renovate.json5` | 0 findings (hujson parses JWCC) |
| `malformed/renovate.json` | 1 × `INFO` parse-error |
| *(no config)* | 0 findings |

> Note: the existing scanners point at the flat `testdata/security/` dir and select
> files by suffix. Because this scanner probes for fixed filenames (`renovate.json`,
> etc.), each test case needs its own subdirectory containing exactly one config
> file so the probe order and "first match wins" behavior are exercised cleanly. The
> implemented layout uses one subdirectory per case under
> `testdata/security/renovate/<case>/` (see the table above).

## Exercise end-to-end (dry-run, no mutation)

Because the scanner auto-registers, a normal dry-run surfaces its findings with no
flag. Against a scratch clone that has a deficient `renovate.json`:

```bash
# stderr shows: [LOW] fleet.renovate.gh-aw-actions-not-disabled  renovate.json  ...
go run . deploy <owner/repo>            # dry-run (no --apply); advisory only, never blocks

# JSON surface:
go run . deploy <owner/repo> --output json | jq '.warnings[] | select(.code=="security_renovate")'
```

`deploy`, `sync`, and `upgrade` all run `security.Run` and surface findings
identically. **Never run `--apply` to test this** — the scanner is fully exercised
by dry-runs and unit tests (per AGENTS.md testing guidance).

## Optional parallel self-fix

The fleet's own `renovate.json` also lacks both rules. Adding them is a worthwhile
parallel cleanup (the scanner would flag this repo too), but it is **not** part of
the scanner's acceptance — keep it a separate, clearly-scoped change.

## Done criteria

- All C2 contract rows covered by tests; `make ci` green.
- No new `go.mod` direct dependency; no `cmd.SchemaVersion` / `fleet.SchemaVersion`
  bump.
- A dry-run against a deficient config shows the `LOW` findings on stderr, in JSON
  `warnings[]`, and in the PR `## Security Findings` section — and the deploy still
  reports success (advisory, non-blocking).
