# Quickstart: Dependabot Config Conflict Scanner

How to build, test, and exercise the scanner during development. Assumes the standard
local toolchain (Go 1.26.4, `make ci` gate per AGENTS.md).

## Build & gate

```bash
go build ./...                          # compile
go vet ./...                            # vet
go test ./internal/fleet/security/...   # scanner unit tests (fast)
make ci                                 # full gate: fmt-check vet lint test  (REQUIRED before "done")
```

> Per the project rule, run the local lint via `~/go/bin/golangci-lint run ./...`
> if `make lint` resolves the wrong binary; it can exceed 5 minutes.

## Where the code lives

| File | Role |
|------|------|
| `internal/fleet/security/dependabot.go` | the scanner (probe, parse, detect, emit) |
| `internal/fleet/security/dependabot_test.go` | table-driven tests over the fixtures |
| `internal/fleet/security/constants.go` | rule IDs + `rulePrefixDependabot` |
| `internal/fleet/security/finding.go` | `defaultScanners()` registration + `diagCodeForRuleID()` mapping |
| `internal/fleet/fleetdiag/diag.go` | `DiagSecurityDependabot` code |
| `internal/fleet/diagnostics.go` | alias mirror of the new code |
| `internal/fleet/security/testdata/security/dependabot/<case>/.github/dependabot.yml` | per-case fixtures (one subdir per case) |

## Exercise the unit tests against fixtures

The scanner is pure and offline — point it at a fixture directory (the clone-dir root)
and assert the findings. Mirror the existing `renovate_test.go` table-driven style:

```go
got := newDependabotScanner().Scan(context.Background(), "testdata/security/dependabot/<case-dir>")
```

> The probe is for the fixed path `.github/dependabot.yml` (then `.yaml`), so each
> fixture case is a subdirectory whose config lives at
> `<case>/.github/dependabot.yml`. The test passes `<case>` as the clone-dir; the
> scanner probes `<case>/.github/dependabot.yml`. This exercises the probe order and
> "first match wins" cleanly.

Cases to cover (one per row of the C2 contract table):

| Fixture | Expectation |
|---------|-------------|
| `correct/.github/dependabot.yml` | 0 findings (ignore covers gh-aw family) |
| `missing-ignore/.github/dependabot.yml` | 1 × `LOW` gh-aw-actions-not-ignored |
| `gomod-only/.github/dependabot.yml` | 0 findings (no github-actions entry) |
| `wildcard-ignore/.github/dependabot.yml` | 0 findings (`github/gh-aw-actions*` wildcard) |
| `pr-limit-zero/.github/dependabot.yml` | 0 findings (`open-pull-requests-limit: 0`) |
| `multiple-unprotected/.github/dependabot.yml` | 2 × `LOW` (one per github-actions entry) |
| `malformed/.github/dependabot.yml` | 1 × `INFO` parse-error |
| *(no config)* | 0 findings |

## Exercise end-to-end (dry-run, no mutation)

Because the scanner auto-registers, a normal dry-run surfaces its findings with no
flag. Against a scratch clone whose `.github/dependabot.yml` has a deficient
`github-actions` entry:

```bash
# stderr shows: [LOW] fleet.dependabot.gh-aw-actions-not-ignored  .github/dependabot.yml  ...
go run . deploy <owner/repo>            # dry-run (no --apply); advisory only, never blocks

# JSON surface:
go run . deploy <owner/repo> --output json | jq '.warnings[] | select(.code=="security_dependabot")'
```

`deploy`, `sync`, and `upgrade` all run `security.Run` and surface findings
identically. **Never run `--apply` to test this** — the scanner is fully exercised by
dry-runs and unit tests (per AGENTS.md testing guidance).

## Live validation reference

The motivating real instance is `rshade/finfocus#1246` — a Dependabot `github_actions`
PR bumping `github/gh-aw-actions` 0.77.4 → 0.78.3 across nine-plus `*.lock.yml` files.
A dry-run of `deploy`/`sync`/`upgrade` against that repo's clone (whose
`.github/dependabot.yml` has a `github-actions` entry without the gh-aw ignore) should
surface exactly one `LOW` finding.

## Parallel parity note (fleet's own repo)

The fleet's own `.github/dependabot.yml` is **gomod-only** (no `github-actions` entry),
so this scanner produces **zero** findings on it — it is not at risk today. Adding a
`github-actions` entry later would require the gh-aw ignore for parity, but that is
**not** part of this scanner's acceptance.

## Done criteria

- All C2 contract rows covered by tests; `make ci` green.
- No new `go.mod` direct dependency (yaml.v3 reused); no `cmd.SchemaVersion` /
  `fleet.SchemaVersion` bump.
- A dry-run against a deficient config shows the `LOW` finding on stderr, in JSON
  `warnings[]`, and in the PR `## Security Findings` section — with the name-only
  caveat in the remedy — and the deploy still reports success (advisory, non-blocking).
