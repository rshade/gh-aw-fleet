# Quickstart: Interactive security-finding prompt before commit

Runnable validation scenarios. Unit/placement scenarios run offline against
existing seams; the live `--apply` scenario requires explicit user approval and
a scratch target repo (never run `--apply` unattended in a subagent).

## Build & gate

```bash
go build ./...
make ci          # fmt-check, vet, lint, test — required before "done"
```

## Unit scenarios (`internal/fleet/security_prompt_test.go`)

Table-driven over `PromptUser(findings, yes, in, out)` with `bytes.Buffer` for
`in`/`out` and the `stdoutIsTerminal` seam overridden per case:

| Case | findings | yes | TTY stub | stdin | expect |
|------|----------|-----|----------|-------|--------|
| a | empty | false | true | — | `(true, nil)`, no output |
| b | 1 HIGH | true | true | — | `(true, nil)`, no output |
| c | 1 HIGH | false | **false** | — | `(true, nil)`, no output |
| d | 1 HIGH | false | true | `"y\n"` | `(true, nil)`, summary written |
| e | 1 HIGH | false | true | `"n\n"` | `(false, nil)` |
| f | 1 HIGH | false | true | `""` (EOF) | `(false, err)` |

Also assert: empty stdin line (`"\n"`) → `(false, nil)` (default No); `"YES\n"`
and `"Y\n"` → `(true, nil)`; the written summary contains
`security.SeveritySummary(findings)` (e.g. `2 HIGH, 1 MEDIUM`).

## Placement scenarios (deploy/sync/upgrade, offline)

Use existing test seams + temp dirs + an injected non-TTY (so prompts never
block) and a forced finding fixture:

- **Dry-run never prompts**: `Deploy(..., opts{Apply:false})` with findings →
  `PromptUser` is not invoked; no `OperatorDeclinedError`.
- **Apply + non-TTY proceeds**: `opts{Apply:true}`, findings, TTY stub false →
  proceeds to the commit boundary; clone not preserved for this reason.
- **Apply + TTY + decline aborts**: TTY stub true, stdin `"n\n"` → returns
  `*OperatorDeclinedError`, no `createDeployPR` call, clone preserved.
- **`--yes` skips**: `opts.Security.Yes=true`, findings, TTY stub true → proceeds
  with no stdin read; stderr findings + PR-body section still produced.
- **Strict precedes prompt**: `opts.Security.Strict=true` with a HIGH finding →
  `*StrictSecurityError` returned before the prompt is reached.
- **Sync prompts once**: sync add path (Missing>0) prompts via the delegated
  `Deploy` exactly once; sync prune-only path (Missing==0, Pruned>0) prompts once
  before `commitAndPushPrune`; sync clean path prompts zero times.
- **Zero findings**: apply with no findings → no prompt, no `## Security
  Findings` PR section.

## Command flag scenarios (`cmd/*_test.go`)

- `--yes` is registered on `deploy`, `sync`, and `upgrade` with the documented
  help text; `__schema` lists it for all three.
- A decline surfaces as a clean non-zero failure (mapped via
  `IsOperatorDeclinedError` in `output.go`), not a hint-engine dump.

## Manual live validation (user-approved `--apply` only)

> Three-turn pattern: dry-run → explicit "go" → apply. Never apply unattended.

1. Prepare a scratch repo whose clone trips a scanner rule (e.g. an embedded
   fake secret in a workflow).
2. **Interactive decline**: `./gh-aw-fleet deploy <scratch> --apply` in a
   terminal → severity summary + `Proceed with commit? [y/N]`; answer `n` →
   assert no branch/commit/PR was created and the command exited non-zero.
3. **Interactive accept**: re-run, answer `y` → assert the PR was created and its
   body contains a `## Security Findings` section.
4. **`--yes` bypass**: `./gh-aw-fleet deploy <scratch> --apply --yes` → no prompt;
   PR created with the `## Security Findings` section still present, and findings
   still printed on stderr.
5. **Pipe bypass**: `./gh-aw-fleet deploy <scratch> --apply | cat` → no prompt,
   does not hang; findings still on stderr; PR body still has the section.
6. **Zero findings**: deploy a clean repo `--apply` → no prompt, no `## Security
   Findings` section.

## Success signals (maps to Success Criteria)

- SC-001/002: interactive decline blocks 100% (no remote change).
- SC-003: piped/CI never hangs.
- SC-004: confirmed or `--yes` apply always carries the PR-body section.
- SC-005: zero findings adds no friction.
- SC-006: identical behavior across `deploy`/`sync`/`upgrade`.
