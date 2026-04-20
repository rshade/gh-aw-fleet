# CLI Contract: `gh-aw-fleet add`

**Phase**: 1 (contracts)
**Feature**: `specs/001-add-subcommand`

This document is the authoritative reference for the CLI surface
introduced by this feature. Anything an operator (or a script, or a
skill) sees at the command line is covered here.

## Synopsis

```text
gh-aw-fleet add <owner/repo>
    --profile <name>[,<name>...]      (required, repeatable)
    [--engine <name>]                 (optional, single-value)
    [--exclude <workflow-name>]       (optional, repeatable)
    [--extra-workflow <spec>]         (optional, repeatable)
    [--apply]                         (default: dry-run)
    [--yes]                           (required with --apply in non-TTY)
    [--dir <path>]                    (inherited from root; working dir)
```

## Arguments

### Positional: `<owner/repo>`

- **Required.** Exactly one argument.
- Must be in `owner/repo` form: two non-empty `[a-z0-9._-]+` halves
  separated by a single `/`.
- Normalized to lowercase before any comparison or write.
- Examples (valid): `rshade/gh-aw-fleet`, `acme/foo.bar`,
  `Owner/Repo` (normalized to `owner/repo`).
- Examples (invalid): `` (empty), `justaword` (no slash),
  `owner/` (empty half), `/repo` (empty half),
  `owner/repo/extra` (too many slashes), `owner with space/repo`.

## Flags

### `--profile <name>` (required, repeatable or comma-separated)

- At least one profile **must** be specified.
- May be passed multiple times (`--profile a --profile b`) or as a
  single comma-separated value (`--profile a,b`). Both forms
  produce identical behavior (see `research.md §1`).
- Every name must exist in the merged `cfg.Profiles` map. A name
  that does not exist causes a non-zero exit with a message listing
  available profile names.

### `--engine <name>` (optional, single-value)

- Default: absent (repo inherits `cfg.Defaults.Engine`).
- Accepted values: the keys of `EngineSecrets` in
  `internal/fleet/deploy.go`. Current values at time of writing:
  `copilot`, `claude`, `codex`, `gemini`.
- Unknown value causes a non-zero exit with a message listing
  accepted engine names.

### `--exclude <workflow-name>` (optional, repeatable)

- Each occurrence adds one name to `RepoSpec.ExcludeFromProfiles`.
- No validation against the selected profiles' workflow lists; a
  name that matches nothing triggers a **warning** on stderr but
  does NOT fail (see `research.md §3`).

### `--extra-workflow <spec>` (optional, repeatable)

- Each occurrence adds one entry to `RepoSpec.ExtraWorkflows`.
- Accepted forms (FR-008):
  - `name` → local workflow (`Source: "local"`, no `Ref`, no `Path`).
  - `owner/repo/name@ref` → agentics 3-part form (`Source:
    "owner/repo"`, `Ref: ref`, no `Path`).
  - `owner/repo/.github/workflows/name.md@ref` → gh-aw 4-part form
    (`Source: "owner/repo"`, `Ref: ref`, `Path:
    ".github/workflows/name.md"`).
- Any other form causes a non-zero exit with a message showing the
  three valid forms.

### `--apply` (boolean, default: false)

- Default (false): dry-run. Validates, resolves, prints preview.
  No file mutation.
- `--apply`: after validation succeeds AND confirmation is
  obtained, write `fleet.local.json`.

### `--yes` (boolean, default: false)

- Confirms `--apply` without an interactive prompt.
- Required when stdin is not a TTY (CI, piped shells). Attempting
  `--apply` in a non-TTY without `--yes` exits 1 with the message:
  `--apply requires --yes in a non-interactive shell`.
- Ignored in dry-run mode (log once as "ignored: --yes has no
  effect without --apply" on stderr).

### `--dir <path>` (inherited from root)

- Working directory containing `fleet.json` and/or
  `fleet.local.json`. Default: `"."`.

## Stdin

- Used **only** for the interactive confirmation prompt when
  `--apply` is passed without `--yes` and stdin is a TTY.
- Not consumed for any other purpose.
- Prompt text: `Write fleet.local.json? [y/N] `.
- Accepted responses (case-insensitive): `y`, `yes`.
- Any other input (including empty / EOF) aborts with exit 1 and
  the message `aborted: re-run with --apply --yes to confirm`.

## Stdout

### Dry-run mode

- One line per resolved workflow, format: `- <workflow-name>`.
- Order: profile-member workflows (in profile resolution order),
  then extras.
- No other output on stdout.

### `--apply` success

- Same workflow list as dry-run (for consistency — operators who
  pipe stdout get the same payload regardless of mode).

### Error paths

- Nothing on stdout when validation fails before resolve.

## Stderr

### Dry-run mode

1. Header: `would add <repo> with profiles [<csv>] (<N> workflows)`
2. Engine override line (only if `--engine` passed):
   `engine override: <name>`
3. Zero or more warnings, each prefixed `warning: `.
4. Final hint: `next: re-run with --apply to persist`

### `--apply` success

1. File-transition notice (only when synthesizing a new
   `fleet.local.json` from a `fleet.json`-only baseline):
   `creating fleet.local.json (minimal; profiles/defaults still resolved from fleet.json)`
2. Header: `added <repo> with profiles [<csv>] (<N> workflows)`
   (tense shift from "would add" to "added" signals success).
3. Engine override line (if applicable).
4. Zero or more warnings, same format as dry-run.
5. Final hint: `next: gh-aw-fleet deploy <repo>`

### Error paths

- Single `error: <message>` line followed by any applicable
  remediation hint (engine list, profile list, slug example, etc.).

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Dry-run success OR `--apply` success |
| 1 | Any validation or write failure |

`add` never returns exit codes other than 0 or 1. Future commands
(`remove`, `status`) MAY introduce codes 2+ for distinct outcomes;
`add` deliberately stays binary to keep scripting trivial.

## Environment variables

None read or written by `add`. `LoadConfig` honors the existing
`--dir` flag; no new env vars are introduced.

## Idempotence

`add` is **not** idempotent. Re-running `add owner/repo --profile X
--apply --yes` after a successful first run returns exit 1 with
the duplicate-repo error. This is intentional: shadowing or
re-assigning profile membership is not a well-defined operation in
v1 (see spec.md's "Out of Scope" and the shadowing edge case).

## Example invocations

```bash
# Dry-run a simple onboarding:
gh-aw-fleet add rshade/foo --profile default

# Onboard with customization:
gh-aw-fleet add rshade/foo \
    --profile default \
    --engine claude \
    --exclude ci-doctor \
    --extra-workflow my-local-thing \
    --apply --yes

# Onboard a repo that needs a pinned remote extra:
gh-aw-fleet add rshade/foo \
    --profile default \
    --extra-workflow githubnext/agentics/security-guardian@v0.4.1 \
    --apply --yes
```

## Changes to other commands

None. `deploy`, `list`, `sync`, `upgrade`, `template` are
unchanged by this feature.

## Tests required to cover this contract

See `research.md §9` for the full test matrix. Every row in every
table above (arguments, flags, stdout, stderr, exit codes) MUST
be exercised by a case in either `TestAdd_DryRun` or `TestAdd_Apply`
in `internal/fleet/add_test.go`, except for:

- The interactive TTY prompt path (not easily unit-testable;
  covered by manual integration verification in `quickstart.md`).
- The `--dir` flag (inherited unchanged from root; already tested
  transitively by other subcommands).
