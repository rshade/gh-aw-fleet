# Phase 0 Research: CLI JSON Output Mode

**Feature**: 003-cli-output-json
**Date**: 2026-04-21

This document resolves the technical unknowns identified in `plan.md`'s Technical Context. Each section follows the Decision / Rationale / Alternatives pattern.

---

## R1: `json.RawMessage` nesting for `UpgradeResult.AuditJSON`

**Question**: When `UpgradeResult.AuditJSON` (of type `json.RawMessage`) is marshaled as part of the envelope, does it nest as a native JSON object, or does the encoder stringify/escape it? What happens on invalid JSON bytes or a nil slice?

**Decision**: Keep `json.RawMessage` as-is and rely on its documented marshal behavior: a non-nil, valid-JSON slice embeds verbatim into the output at the field position (native nested object — no escape-encoding). A nil slice marshals as JSON `null`. A non-nil slice with invalid JSON bytes causes `json.Marshal` on the parent struct to return an error, which the envelope writer surfaces as a fatal.

**Rationale**:
- From `encoding/json` source (`encode.go`): `encodeByteSlice` is short-circuited for `RawMessage` — the implementation type-asserts and writes the bytes directly via `Write(...)` without wrapping, base64-encoding, or escaping.
- `json.Marshal(struct{ X json.RawMessage }{X: []byte(`{"k":1}`)})` yields `{"X":{"k":1}}` — a native nested object, verifiable in any Go playground.
- `AuditJSON` is populated from `gh aw audit --json` output (see `internal/fleet/upgrade.go:145 res.AuditJSON = out`). `gh aw audit` produces valid JSON by contract. If it ever regresses and emits non-JSON bytes, the marshal error will fail the envelope write cleanly (non-zero exit, stderr error) rather than producing a broken envelope on stdout.
- No defensive validation (`json.Valid(bytes)` at capture time) is added. It would double-scan every upgrade's audit payload without materially improving the failure mode — a malformed `gh aw audit` is an upstream bug that deserves to surface, not get hidden.

**Alternatives considered**:
- **Validate at capture time with `json.Valid`**: Rejected. Adds a full JSON-scan per upgrade for no meaningful benefit; silently converting malformed bytes to a fallback value (`null`, empty object) would hide an upstream bug; surfacing the error at marshal time is cleaner.
- **Change type to `any` / `map[string]any`**: Rejected. Pulls the whole audit payload through reflection-based decode+encode, losing the zero-copy property of `RawMessage` and introducing unnecessary allocations. Also re-encodes keys in Go-map order (non-deterministic) unless sorted, which changes byte-for-byte output day to day.
- **Store as `string`**: Rejected. Would force consumers to `fromjson()` the value instead of descending with `jq '.result.audit_json.foo'` — directly violates spec FR-016.

---

## R2: Empty-slice-vs-nil-slice JSON marshaling

**Question**: How do we guarantee that empty slice fields on `DeployResult`, `SyncResult`, `UpgradeResult`, and `ListResult` marshal as `[]` (not `null`) without plumbing `omitempty` into every struct (which would *hide* empty fields entirely, the opposite of what we want)?

**Decision**: Initialize slice fields to non-nil empty slices (`[]T{}`) before envelope marshal. Implement as a small helper `initSlices(result any)` in `cmd/output.go` that uses reflection to walk the result struct and replace nil slices with empty-slice equivalents of the same element type. Call this helper from `writeEnvelope` just before `json.Encoder.Encode`. Leave business-layer code (`internal/fleet/*.go`) untouched — the JSON-shape concern stays in the JSON path.

**Rationale**:
- `encoding/json` marshals a nil slice as `null` and a non-nil empty slice as `[]`. This is documented in the package docs and is stable across Go versions.
- The alternative of setting the initial value at struct construction time (e.g., `&DeployResult{Added: []WorkflowOutcome{}}`) requires touching every construction site and creates a permanent maintenance burden — any new code path that builds a `DeployResult` risks missing the init.
- Centralizing the init in the envelope writer means the JSON contract is enforced at exactly one point. Business logic continues to use idiomatic Go (`var added []WorkflowOutcome` + `append`), which creates nil slices by default.
- Reflection cost is negligible — `json.Marshal` already uses reflection; walking the struct once more adds ~microseconds per envelope.

**Alternatives considered**:
- **`omitempty` on every slice field**: Rejected. Would omit empty-slice fields entirely from the JSON output, forcing consumers to write `result.added // []` everywhere. Breaks the SC-004 / FR-009 promise that "every slice is always present as an array".
- **Initialize at construction time**: Rejected (maintenance burden; see above).
- **Custom `MarshalJSON` on each result struct**: Rejected. Multiplies boilerplate across four struct types; each custom marshaler has to re-emit every field, so adding a new field to `DeployResult` would require an edit in two places.
- **Third-party library (go-cmp, sjson, etc.) to rewrite the JSON output**: Rejected — adds a dependency, violates Principle I.

**Implementation sketch** (for reference; actual code lives in `cmd/output.go`):

```go
// initSlices walks a pointer-to-struct result and sets nil slice fields to
// empty non-nil slices so json.Marshal emits them as "[]" not "null".
// Handles nested struct fields and slices-of-structs.
func initSlices(v any) {
    rv := reflect.ValueOf(v)
    if rv.Kind() == reflect.Ptr {
        rv = rv.Elem()
    }
    if rv.Kind() != reflect.Struct {
        return
    }
    for i := 0; i < rv.NumField(); i++ {
        f := rv.Field(i)
        if !f.CanSet() {
            continue
        }
        switch f.Kind() {
        case reflect.Slice:
            if f.IsNil() {
                f.Set(reflect.MakeSlice(f.Type(), 0, 0))
            } else {
                // Walk slice elements (in case of []Struct)
                for j := 0; j < f.Len(); j++ {
                    if f.Index(j).Kind() == reflect.Struct {
                        initSlices(f.Index(j).Addr().Interface())
                    }
                }
            }
        case reflect.Struct:
            initSlices(f.Addr().Interface())
        case reflect.Ptr:
            if !f.IsNil() && f.Elem().Kind() == reflect.Struct {
                initSlices(f.Interface())
            }
        }
    }
}
```

---

## R3: Cobra persistent-flag validation before subcommand RunE

**Question**: How do we reject `--output yaml` (or empty string, or `JSON`) before any subcommand RunE fires — so the user gets one clean "unsupported output mode" error and exit, not a partial subcommand invocation?

**Decision**: Validate inside `PersistentPreRunE` on the root `*cobra.Command`. The root command already has a `PersistentPreRunE` (wired in #34 for log-level/log-format validation). Extend it to additionally read `--output`, check the value against the closed set `{"text", "json"}`, and return `fmt.Errorf("unsupported output mode %q: expected one of: text, json", v)` on mismatch. Cobra's default error routing writes the message to stderr and exits non-zero without calling the subcommand's RunE.

**Rationale**:
- `PersistentPreRunE` runs after flag parsing but before `RunE`. Perfect hook for cross-subcommand validation.
- Reusing the existing `PersistentPreRunE` slot (rather than adding a separate validator per subcommand) guarantees the check fires for every subcommand including `add`, `template fetch`, etc. — even those that don't accept `-o json` (FR-013). For those commands, a second check inside their own `RunE` rejects JSON mode with a more specific error ("command `template fetch` does not support --output json").
- The pattern matches what #34 already established for `--log-level` / `--log-format`. Consistency is better than re-inventing.

**Alternatives considered**:
- **`cobra.PositionalArgs`-style validator on the flag itself**: Cobra doesn't have first-class enum-flag support. `spf13/pflag` has no `EnumVar`. Possible via a custom `pflag.Value` implementation, but that's a heavier abstraction than a three-line `PersistentPreRunE` check for two values.
- **Validate inside each subcommand's RunE**: Rejected. Four duplicated validation blocks; easy to drift; violates DRY without the compensating benefit.
- **Accept any string and treat unknown as text**: Rejected. Silent fallback is a footgun — an operator typing `-o jsno` (typo) would get text output and wonder why `jq` parse fails on the ANSI tabwriter. Explicit rejection is kinder.

---

## R4: NDJSON emission semantics for `upgrade --all`

**Question**: For `upgrade --all -o json`, how do we emit one JSON envelope per repo per line, ensuring each line is flushed as the repo completes (so downstream consumers can process streaming)? And what happens if `upgrade --all` is parallelized in a future feature — does the emission path become racy?

**Decision**: Use `json.NewEncoder(os.Stdout).Encode(envelope)` once per repo completion inside the existing per-repo loop in `cmd/upgrade.go`'s `--all` branch. `Encoder.Encode` writes the JSON form followed by a single newline and does not buffer internally — the next-level writer (`os.Stdout`) is line-buffered when attached to a terminal and unbuffered when piped. Flush is implicit. Document a **mutex requirement** in `contracts/ndjson.md` for any future parallelization: the envelope write MUST be serialized, otherwise partial lines from concurrent `Encode` calls can interleave.

**Rationale**:
- `encoding/json.Encoder.Encode` is documented to "terminate each value with a newline character" — exactly the NDJSON contract. No extra framing needed.
- For the current serial-upgrade loop, concurrency is not a concern. The mutex note is forward-defensive, not required today.
- Using `Encoder` (not `Marshal` + manual `fmt.Println`) has a subtle correctness benefit: `Encoder` writes directly to the underlying writer without an intermediate `[]byte` allocation for each envelope. Memory-friendly on 20+ repo fleets.

**Alternatives considered**:
- **Accumulate all envelopes into an aggregate `{results: [...]}` and emit once at the end**: Already rejected in Q1 clarification (user chose NDJSON). Also breaks streaming — consumer waits N×per-repo-duration before seeing any output.
- **Emit NDJSON only to a bufio-wrapped stdout with explicit flush-per-line**: Unnecessary. `os.Stdout` is already line-buffered for terminals and unbuffered for pipes; adding bufio would actually hurt (need to remember to flush at exit).
- **Use `encoding/json.Marshal(envelope)` + `fmt.Fprintln(os.Stdout, ...)`**: Functionally equivalent but adds an intermediate `[]byte` allocation per repo and a split-write risk (Marshal could succeed, Fprintln could fail partway) versus Encoder's single Write.

---

## R5: Dual-emission of warnings (envelope + stderr via zerolog)

**Question**: How do warnings land BOTH in the envelope's `warnings[]` AND on stderr via the zerolog logger, without duplicating source-of-truth or creating drift between the two emissions?

**Decision**: At each warning site (currently: `cmd/deploy.go` missing-secret, `cmd/sync.go` drift detected, `cmd/hints.go` hint emission), construct a single `Diagnostic{Code, Message, Fields}` value. Append it to a local `[]Diagnostic` slice that the command carries through to the envelope writer. At the same site, also call `zlog.Warn().Fields(d.Fields).Msg(d.Message)` (for warnings) or `zlog.Warn().Str("hint", d.Message).Msg(d.Message)` (for hints). The `Diagnostic` value is constructed once and used twice — no drift possible between stderr and envelope contents.

**Rationale**:
- Warnings today already emit to stderr via zerolog (#34 landed this). The change is purely additive: at each existing `zlog.Warn()` site, also capture the same fact into a `Diagnostic` slice. Cost: one `append` per warning.
- The `Diagnostic` struct's `Code` field satisfies spec FR-011's "stable code identifier" requirement. Codes are defined as package-level string constants in `cmd/output.go` (e.g., `DiagMissingSecret = "missing_secret"`, `DiagDriftDetected = "drift_detected"`). Adding a new warning class adds one constant.
- The alternative of extracting warnings from zerolog output (via a capture hook on the logger) was considered and rejected: it would couple the envelope contents to the zerolog field schema, making any zerolog format change a breaking change to the JSON envelope.

**Mapping table** (warning sites → Diagnostic codes):

| Site | Code | Fields |
|---|---|---|
| `cmd/deploy.go` missing secret | `missing_secret` | `{secret: <name>, url: <secret_key_url>}` |
| `cmd/sync.go` drift detected | `drift_detected` | `{repo, drift: [...]}` |
| `cmd/hints.go` each hint | `hint` | `{hint: <full-hint-text>}` — maps 1:1 to existing zerolog hint event |

The hint path uses `CollectHintDiagnostics()` (new) in `internal/fleet/diagnostics.go`: it extends the existing `Hint` struct with a `Code` field and returns a structured `[]Diagnostic` parallel to `CollectHints() []string`. Text-mode callers keep using `CollectHints`; JSON-mode callers use `CollectHintDiagnostics`. Same hints.table backs both.

**Alternatives considered**:
- **Hook into zerolog via a custom `io.Writer` that tees warnings into a slice**: Rejected. Couples JSON envelope schema to zerolog's field schema; any format change in zerolog propagates through.
- **Post-process the zerolog-produced stderr buffer after the command finishes**: Rejected. Requires capturing stderr (redirection), parsing emitted log lines back into structured form, and re-encoding — round-trip is wasteful and fragile.
- **Move the source of truth to `warnings[]` and have zerolog pull from it**: Rejected. Breaks text-mode (which has no envelope to read from). The chosen direction — local `Diagnostic` value shared at the emission site — works for both modes symmetrically.

---

## Summary

| Research | Decision | Key consequence |
|---|---|---|
| R1 (`json.RawMessage`) | Pass-through marshal; no validation at capture | `AuditJSON` nests as native object (FR-016); malformed bytes surface as marshal error |
| R2 (empty slices) | Reflection-based `initSlices` in envelope writer | Business logic untouched; JSON shape enforced at one point (FR-009) |
| R3 (flag validator) | `PersistentPreRunE` on root | One error path; all subcommands covered; matches #34 pattern |
| R4 (NDJSON) | `json.Encoder.Encode` per repo | Streaming for free; mutex note for future parallelism |
| R5 (dual-emission) | Local `Diagnostic` value, used twice at the site | No drift between envelope and stderr; adds one `append` per warning |

All NEEDS CLARIFICATION items from plan.md are resolved. Phase 1 design artifacts can proceed.
