# Quickstart: Verifying the `pkg/fleet` Contract Export

How to confirm each success criterion locally once the change is implemented. All
commands run from the repo root. No `--apply`, no network mutation — this slice is
pure code relocation.

## 1. Build & full gate (SC-003, SC-004)

```bash
go build ./...        # compiles, including the new pkg/fleet package
go vet ./...
make ci               # fmt-check + vet + lint + full test suite (the CI gate)
```

`make ci` must be green with **no new lint suppressions** (SC-003) and **no
modified behavior-test expectations** — only references to relocated symbols
should change in existing tests (SC-004).

> Per project convention (operator preference): if `make lint` misbehaves, run the
> linter directly with `~/go/bin/golangci-lint run ./...`. Lint can exceed 5
> minutes — use an extended timeout, do not skip it.

## 2. External consumer can import the contract (SC-001, C-1/C-2/C-3)

The black-box test in `pkg/fleet/config_test.go` (`package fleet_test`) is the
in-module proof — it can only touch exported identifiers, exactly like an outside
module:

```bash
go test ./pkg/fleet/ -run 'TestExternalConsumer|Example' -v
```

Expect: declares `fleet.Config`, `fleet.RepoSpec`, `fleet.Profile`,
`fleet.SourcePin`, `fleet.ProfileWorkflow`, `fleet.ExtraWorkflow`,
`fleet.Defaults`; asserts `fleet.SchemaVersion == 1`; asserts
`EffectiveEngine` returns the per-repo override then the default.

Optional belt-and-suspenders (a literal *separate module* importing the engine) is
not required by this slice — the external test package gives the same compile-time
guarantee within CI.

## 3. Wire bytes stay byte-identical (SC-002, C-4/C-5/C-6)

```bash
go test ./pkg/fleet/ -run TestGoldenRoundTrip -v
```

The test: read `fleet.example.json` → `json.Unmarshal` into `pkg/fleet.Config` →
`json.MarshalIndent(cfg, "", "  ")` + trailing newline → assert **byte-equal** to
`pkg/fleet/testdata/config.canonical.json`.

To regenerate the golden after an *intentional* example-data change (rare — not in
this slice):

```bash
# conceptual — implement as a small -update flag or one-off:
#   marshal(unmarshal(fleet.example.json)) > pkg/fleet/testdata/config.canonical.json
```

It also asserts `LoadedFrom` never appears in the output (C-5) and that the
`omitzero`/`omitempty`/no-omit cases each behave as documented (C-6).

> Reminder: the golden is **not** `fleet.example.json` verbatim — the example is
> hand-aligned and carries `omitempty` empty arrays that drop on re-marshal
> (research.md Decision 4).

## 4. No on-disk or behavior change (SC-004, SC-006, FR-011/FR-012)

```bash
# Before and after the change, on main vs the branch:
go run . list 2>&1 | tee /tmp/list.after.txt
# diff against a capture from main — output + exit code must match.

git diff main -- go.mod          # expect: empty (no new require) — SC-006
grep -rn "SchemaVersion = " internal/fleet cmd   # values unchanged (still 1)
```

`fleet.json` / `fleet.local.json` on disk are never rewritten by this change.

## 5. Godoc completeness (SC-005, C-7)

```bash
make lint     # revive + staticcheck flag any exported symbol missing a godoc comment
go doc ./pkg/fleet            # eyeball: every type/const/method documented
go doc ./pkg/fleet Config
```

## Acceptance checklist (maps to spec Success Criteria)

- [ ] SC-001 — external `package fleet_test` references all 7 types + `SchemaVersion`, compiles.
- [ ] SC-002 — golden round-trip byte-identical to `config.canonical.json`.
- [ ] SC-003 — `make ci` green, no new lint suppressions.
- [ ] SC-004 — `go run . list` (and suite) output/exit unchanged vs `main`; only relocated-symbol references edited.
- [ ] SC-005 — 100% godoc on new exported identifiers (clean revive/staticcheck).
- [ ] SC-006 — no `SchemaVersion` value change; `go.mod` gains no `require`.
