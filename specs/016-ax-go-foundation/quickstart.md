# Quickstart: Verifying Phase 1 (ax-go adoption) Locally

Every check below is runnable from the repo root and maps to a Success Criterion
in [spec.md](./spec.md). Run order roughly follows the implementation order.

## 0. Prerequisites

- Go 1.26.4 on PATH (`go version`) — the gate already requires it; this phase
  raises the `go.mod` directive to match (FR-004).
- `golangci-lint` v2.12.2 (per AGENTS.md / your standing lint rule).

## 1. Dependency adopted, isolated, and tidy (SC-003, SC-003a)

```bash
# exactly one new direct require, pinned to v0.2.0; hujson still direct
grep -nE 'rshade/ax-go|tailscale/hujson' go.mod
go mod tidy && git diff --exit-code go.mod go.sum   # no further diff

# IMPORT ISOLATION: the build must NOT reach OTel/gRPC/protobuf
go list -deps ./... | grep -E 'go.opentelemetry.io|google.golang.org/grpc|google.golang.org/protobuf' \
  && echo 'FAIL: heavy stack leaked into the build' || echo 'OK: no OTel/gRPC/protobuf in build'

# and they must not be in go.mod at all
grep -E 'opentelemetry|google.golang.org/(grpc|protobuf)' go.mod \
  && echo 'FAIL: heavy modules in go.mod' || echo 'OK: go.mod clean of heavy stack'
```

## 2. Config-IO parity — existing tests pass unchanged (SC-002, SC-007)

```bash
# the load/save suite is the regression guard; assertions are NOT edited
go test ./internal/fleet/ -run \
  'TestLoadConfig|TestLoadTemplates|TestProbeConfigPath|TestSaveTemplates|TestBillingMetadata|TestAdd_Apply' -v
```

Expected: all green, including `TestSaveTemplates_PreservesEvaluationsComments`
(comments preserved through `config.Patch`) and `TestLoadConfig_BothExtensionsError`
(the `"ambiguous"` probe error, unchanged).

## 3. `__schema` discoverability works and is additive (SC-004)

```bash
# valid JSON, lists all eight subcommands + the tool version
go run . __schema | python3 -m json.tool >/dev/null && echo 'OK: valid JSON'
go run . __schema | python3 -c '
import json,sys
s=json.load(sys.stdin)
subs={c["use"].split()[0] for c in s["command"]["commands"]}
need={"list","status","add","template","deploy","sync","upgrade","consumption"}
assert need<=subs, f"missing: {need-subs}"
assert s["tool"]=="gh-aw-fleet" and s["version"], "tool/version unset"
print("OK: all 8 subcommands +", s["version"])
'

# MCP adapter form
go run . __schema --as mcp | python3 -c 'import json,sys; print("OK: tools=",len(json.load(sys.stdin)["tools"]))'

# hidden from human help, but invokable
go run . --help | grep -q '__schema' && echo 'NOTE: __schema visible in help' || echo 'OK: __schema hidden from --help'

# unknown format → validation error, non-zero exit (the one contained ax error)
go run . __schema --as bogus; echo "exit=$?"   # expect non-zero
```

## 4. No observable change to existing commands (SC-004, SC-006)

```bash
# read-only commands produce output identical to main
go run . list      # compare against `git stash` / a main checkout if in doubt
go run . status

# wire/format versions are frozen
grep -n 'SchemaVersion = ' cmd/output.go internal/fleet/schema.go   # values unchanged (cmd=1, fleet=1)
```

## 5. Constitution amended (SC-005)

```bash
grep -nE 'ax-go|1\.2\.0' .specify/memory/constitution.md   # ax-go listed; version footer v1.2.0
```

Expect: `github.com/rshade/ax-go` on the Approved-direct-dependencies list with the
three-alternatives rationale + import-isolation note; footer reads
`**Version**: 1.2.0`; a Sync Impact Report entry for the amendment.

## 6. Full gate (SC-001)

```bash
make ci    # fmt-check, vet, lint, test — must be green, no new lint suppressions
```

## Done when

All of §1–§6 pass and `make ci` is green. That satisfies SC-001..SC-007 and the
FR set; the feature is ready for `/speckit-tasks`.
