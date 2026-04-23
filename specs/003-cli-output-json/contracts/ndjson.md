# Contract: NDJSON Stream for `upgrade --all`

**Feature**: 003-cli-output-json
**Spec FRs**: FR-019; User Story 3 acceptance scenario 5; Clarification Q1

This contract defines the newline-delimited JSON (NDJSON) stream emitted on stdout when `upgrade --all -o json` is invoked.

---

## Shape

For a fleet of N repos, `upgrade --all -o json` emits exactly **N** complete JSON envelopes on stdout, one per line. Each line is a full, single-repo envelope matching `contracts/envelope.md` — not a fragment, not a shared header, not an aggregate.

Example for N=3:

```text
{"schema_version":1,"command":"upgrade","repo":"owner/repo-a","apply":false,"result":{...},"warnings":[],"hints":[]}
{"schema_version":1,"command":"upgrade","repo":"owner/repo-b","apply":false,"result":null,"warnings":[],"hints":[{"code":"hint","message":"..."}]}
{"schema_version":1,"command":"upgrade","repo":"owner/repo-c","apply":false,"result":{...},"warnings":[{"code":"missing_secret",...}],"hints":[]}
```

Each envelope is self-contained. A consumer can process each line independently with `jq -c` or any JSON-streaming parser.

---

## Invariants

1. **Exactly one envelope per line.** No pretty-printing. No embedded newlines inside any envelope (the stdlib `json.Encoder` guarantees this for valid JSON — no stdin-injected newline can appear inside string fields without being `\n`-escaped).
2. **Exactly one newline terminator per envelope.** `json.Encoder.Encode(v)` writes one `\n` after each value. No trailing empty line after the Nth envelope (the Nth line's terminator is the file's last byte).
3. **Per-repo flush semantics.** Each envelope MUST be flushed to stdout when the corresponding repo's `Upgrade` returns — not batched until all repos finish. Consumers can stream: a downstream `jq -c '.'` or Python `for line in sys.stdin` consumer sees each repo's result within tens of milliseconds of the repo completing.
4. **Error envelopes are included in-line.** When `upgrade` for repo X fails (pre-result failure or mid-pipeline), the envelope for repo X MUST still appear on its own line with `result: null` and diagnostics populated. Processing continues with repo X+1. The loop does NOT short-circuit on first failure (existing `upgrade --all` text-mode behavior is preserved under FR-014 byte-identity — confirm against current `cmd/upgrade.go`).
5. **No preamble, no footer.** The first byte of stdout in NDJSON mode is `{` (the opening brace of the first envelope). The last byte is `\n` (the terminator of the last envelope). No bash-style banner, no count summary, nothing else.

---

## Parallelism and the mutex requirement

The current `upgrade --all` implementation is **serial** — repos are processed one at a time. NDJSON emission is trivially safe in this mode because `json.Encoder.Encode` is the only writer to stdout inside the loop.

If `upgrade --all` is parallelized in a future feature (e.g., concurrent upgrade of independent repos), the NDJSON emission path **MUST** be serialized behind a mutex. Concurrent `json.Encoder.Encode` calls to the same `os.Stdout` would interleave bytes mid-line, producing invalid JSON.

Implementation guidance for future parallelization:

```go
var stdoutMu sync.Mutex
enc := json.NewEncoder(os.Stdout)

// Per repo (possibly from a goroutine):
envelope := buildEnvelope(...)
stdoutMu.Lock()
_ = enc.Encode(envelope)
stdoutMu.Unlock()
```

The serialization point is the `Encode` call, which must be atomic from the underlying writer's perspective. The Go scheduler does not guarantee `Write` atomicity for multi-byte writes to `os.Stdout` unless the write fits within the OS `PIPE_BUF` (4096 bytes on Linux, POSIX minimum 512). A typical envelope for a 20-workflow repo is well under 4096 bytes, but we do not rely on that — the mutex is the correctness guarantee.

This contract documents the mutex requirement explicitly so the future parallelization PR cannot forget it.

---

## Streaming consumption patterns

### Consumer pattern 1: jq per-line filter

```bash
gh-aw-fleet upgrade --all -o json 2>/dev/null | jq -c 'select(.result.upgrade_ok == false)'
```

Streams each failed repo's envelope as it completes.

### Consumer pattern 2: Aggregate into a single object

```bash
gh-aw-fleet upgrade --all -o json 2>/dev/null | jq -s '{schema_version: 1, command: "upgrade_all", repos: .}'
```

Collects all envelopes into a list under a single aggregate object. Waits for the full run to finish — loses streaming, but recovers an aggregate shape for consumers that want one.

### Consumer pattern 3: Python line-by-line

```python
import json, subprocess
proc = subprocess.Popen(
    ["gh-aw-fleet", "upgrade", "--all", "-o", "json"],
    stdout=subprocess.PIPE,
    stderr=subprocess.DEVNULL,
)
for line in proc.stdout:
    envelope = json.loads(line)
    if envelope["result"] and envelope["result"]["conflicts"]:
        print(f"conflicts in {envelope['repo']}: {envelope['result']['conflicts']}")
```

Streams each repo's result to a Python handler as it completes.

---

## Edge cases

- **Zero repos in fleet.** If `upgrade --all` is run against an empty fleet, stdout is empty (0 lines). `jq` given empty input exits cleanly. This is identical to the text-mode behavior (the for-loop body never runs).
- **Interrupt (Ctrl-C / SIGTERM) during a run.** stdout contains complete envelopes for repos that completed before the signal; nothing partial. The Go runtime flushes `os.Stdout` on program exit; `json.Encoder.Encode` writes atomically per call. If the signal lands mid-`Encode` for repo K, the partial bytes of repo K's line may be present on stdout — this is an accepted failure mode. A consumer encountering a mid-line EOF can discard the trailing partial line and re-run.
- **Single-repo `upgrade <repo> -o json` (no `--all`).** NOT NDJSON — a single envelope, no trailing newline required beyond `json.Encoder.Encode`'s default. Consumers CAN still line-parse it (`json.loads(line)` works on a single-line input) but MUST NOT depend on the NDJSON framing semantics for single-repo invocations.

---

## Test (`cmd/output_test.go`)

| Test name | Covers |
|---|---|
| `TestUpgradeAll_NDJSONLineCount` | Given a mock fleet of 3 repos, stdout contains exactly 3 lines, each a valid JSON object. |
| `TestUpgradeAll_PerLineSelfContained` | Each line parses with `json.Unmarshal` into a `cmd.Envelope` independently; no cross-line state. |
| `TestUpgradeAll_ErrorRepoIncluded` | When one of the 3 repos fails before producing a result, its line is still emitted with `result: null` and diagnostics; the other 2 lines are unaffected. |
| `TestUpgradeAll_EmptyFleet` | Given an empty fleet, stdout is empty (no lines). Exit code is 0. |

Tests use an in-process mock of `fleet.Upgrade` to avoid subprocess and network dependencies.
