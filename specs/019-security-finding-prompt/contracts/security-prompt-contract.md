# Contract: Interactive security-finding confirmation + `--yes`

This contract defines the CLI surface, the confirmation decision, the abort
behavior, and the per-command placement for the security-finding prompt. It is
the authority the unit and placement tests assert against.

## CLI surface

### `--yes` flag (new on `deploy`, `sync`, `upgrade`)

```text
--yes   Skip the interactive security-findings confirmation. Does NOT suppress
        findings from stderr output or the generated PR body — it only bypasses
        the y/N prompt.
```

- Boolean, default `false`. Registered on `deploy`, `sync`, and `upgrade` with
  identical help text.
- Wired as `fleet.SecurityOpts{Strict: flagStrict, Yes: flagYes}`.
- No effect without `--apply` (the prompt only exists on the apply path). Passing
  `--yes` on a dry run is silently inert (no prompt would have fired anyway). It
  is NOT an error.
- Exposed automatically by the hidden `__schema` command via the existing schema
  generator; not separately documented.

## Confirmation decision (`PromptUser`)

`PromptUser(findings, yes, in, out) (proceed bool, err error)`:

| # | Condition | Output to `out` | Returns |
|---|-----------|-----------------|---------|
| 1 | `len(findings) == 0` | nothing | `(true, nil)` |
| 2 | `yes == true` | nothing | `(true, nil)` |
| 3 | stdout not a terminal | nothing | `(true, nil)` |
| 4 | interactive, input `y`/`Y`/`yes` (trimmed, case-insensitive) | summary + prompt | `(true, nil)` |
| 5 | interactive, input `n` / anything else / empty line | summary + prompt | `(false, nil)` |
| 6 | interactive, EOF or read error before an answer | summary + prompt | `(false, err)` |

- **Summary + prompt line** (rows 4–6): a single line of the form
  `⚠  <severity summary>. Proceed with commit? [y/N]` where `<severity summary>`
  is `security.SeveritySummary(findings)` (e.g. `2 HIGH, 1 MEDIUM`). The `[y/N]`
  capitalization signals N is the default.
- **Default**: empty input (bare Enter) → decline (row 5). The safe default is No.
- **Order of checks** is exactly 1→2→3 before any terminal interaction, so the
  empty / `--yes` / non-TTY fast-paths never read stdin and never hang.

## Interactivity gate

- "Interactive" ⇔ `stdoutIsTerminal()` is true, computed from
  `os.Stdout.Stat()` + `os.ModeCharDevice` (stdlib; no `golang.org/x/term`).
- Detection is on **stdout**, not stdin: `tool | tee` (stdout redirected, stdin a
  TTY) does NOT prompt; `tool < script` (stdin redirected, stdout a TTY) still
  prompts (and an EOF answer declines per row 6).
- `--output json` mode is treated as non-interactive (no prompt) per **FR-018**,
  even when stdout is a TTY, because a prompt on stdout would corrupt the JSON
  envelope and a machine cannot answer.

## Abort behavior on decline

When `confirmSecurityFindings` observes a decline (rows 5 or 6):

1. No commit, no branch push, no PR is created (the apply boundary is not
   entered).
2. The work-dir clone is preserved (`cleanupClone=false`; resume paths already
   disable cleanup) as a breadcrumb for `--work-dir … --yes` resumption.
3. The fleet function returns an `*OperatorDeclinedError` (wrapping the read
   error for row 6).
4. `cmd/output.go` recognizes it (`IsOperatorDeclinedError`) and prints a clean
   `aborted by operator … re-run with --yes` message, exiting non-zero. It is NOT
   routed through the hint engine and is NOT rendered as a crash/stack.

## Surface coexistence (defense in depth)

For a single apply with findings, all three surfaces are produced independently:

| Surface | Mechanism | Affected by `--yes`? | Affected by decline? |
|---------|-----------|----------------------|----------------------|
| 1. stderr findings | existing `emitSecurityFindingWarnings` (cmd layer) | No | Still emitted |
| 2. interactive prompt | `PromptUser` (this feature) | **Skipped** | The decline itself |
| 3. PR body `## Security Findings` | existing `security.RenderPRSection` | No | Not reached (no PR) |

`--yes` suppresses surface 2 only. A decline suppresses the PR (surface 3) only
because no PR is created; surface 1 has already been produced.

## Per-command placement (after strict gate, apply only, commit pending)

### `deploy`

- **Fresh path** (`Deploy`): after `security.Run` + `evaluateStrictGatePreservingClone`,
  after the `if !opts.Apply { return }` dry-run return and the
  `len(res.Added)==0 && !staged` no-op guard, **before** `createDeployPR`. On
  decline, set the in-scope `cleanupClone=false`.
- **`--work-dir` resume** (`handleWorkDirResume`): after the re-run scan + strict
  gate, **before** the commit-gate `createDeployPR` call and **before** the
  push-gate `pushAndCreatePR` call. Cleanup is already disabled on resume, so no
  flag flip is needed.

### `sync`

- **Add path**: delegates to `Deploy` via `applyDeployOrPrune` with
  `Security: opts.Security` → inherits Deploy's single prompt. Sync MUST NOT add
  a second prompt here ("prompt exactly once").
- **Prune-only path** (`len(res.Missing)==0 && opts.Prune && len(res.Pruned)>0`):
  after the direct strict gate, **before** `commitAndPushPrune`.
- **Clean path** (nothing to add or prune): no commit, no prompt.

### `upgrade`

- After `security.Run` + strict gate and the `if !opts.Apply { return }` dry-run
  return, **before** `createUpgradePR` on the changed-files path.
- The no-change manifest-backfill path (`finishNoChangeUpgrade`) that can still
  open a PR is also guarded before its `createUpgradePR`.
- `upgrade --all`: per repo; a decline on one repo aborts that repo and the batch
  continues to stop/continue per the existing fail-fast behavior.

## Invariants (assertable)

- Dry-run (`--apply` absent) NEVER prompts.
- A `--strict` HIGH block returns before the prompt is reached.
- The prompt is reached at most once per repo per invocation.
- `cmd.SchemaVersion` and `fleet.SchemaVersion` are unchanged.
- No new entry in `go.mod`'s direct `require()` block.
- Zero findings ⇒ no prompt AND no `## Security Findings` PR section.
