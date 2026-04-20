# Feature Specification: `add <owner/repo>` Subcommand for Fleet Onboarding

**Feature Branch**: `001-add-subcommand`
**Created**: 2026-04-19
**Status**: Draft
**Input**: GitHub issue #9 — "feat(cmd): implement add <owner/repo> subcommand for fleet onboarding"

## Clarifications

### Session 2026-04-19

- Q: What format should `--extra-workflow` accept on the command line? → A: Reuse the existing `gh aw`-style spec syntax — `name` for local extras, `owner/repo/name@ref` (3-part agentics), or `owner/repo/.github/workflows/name.md@ref` (4-part gh-aw). A single parser extracts `Name`, `Source`, `Ref`, and `Path` from one string.
- Q: How should `--engine` be validated, given that `deploy` does not currently reject unknown engines? → A: Validate in `add` only, against the existing `EngineSecrets` map in `internal/fleet/deploy.go`. No changes to `deploy`'s current behavior; `add` becomes the stricter onboarding gate.
- Q: What shape should the dry-run preview output take? → A: Plain prose + indented workflow list, matching the style of `deploy`'s dry-run and `list`'s summary. Header line on stderr ("would add rshade/foo with profiles [default] (N workflows)"), then the workflow names as a bulleted list on stdout. No JSON output mode in v1.
- Q: When `fleet.local.json` does not yet exist and must be synthesized on `--apply`, what should it contain? → A: Rely on the existing merge code — write a minimal `fleet.local.json` containing only `version` and a `repos` map with the single new entry. Do not copy profiles, defaults, or peer repos from `fleet.json`. `LoadConfig`/`mergeConfigs` will continue to produce a correct merged view on every load, and `fleet.json` remains authoritative for shared state.

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Onboard a new repo declaratively with a profile (Priority: P1)

An operator wants to add a repository to their private fleet state
(`fleet.local.json`) and assign it the `default` profile so the next
`deploy` run installs the standard workflow bundle. Today they must
hand-edit JSON and guess at the exact schema.

**Why this priority**: This is the entire motivation for the feature.
Without it, onboarding remains schema-fragile and blocks the
`fleet-onboard-repo` skill from being a one-command operation. This
single story, shipped alone, replaces a manual 10-minute procedure with
a single command and delivers the full MVP value of issue #9.

**Independent Test**: On a fresh checkout with only `fleet.json`
present, run `gh-aw-fleet add rshade/new-example --profile default`
(no `--apply`) and confirm the dry-run preview lists the resolved
workflow set. Then run the same command with `--apply --yes`, confirm
`fleet.local.json` exists with the new entry, and confirm
`gh-aw-fleet list` now surfaces the new repo with its resolved
workflows.

**Acceptance Scenarios**:

1. **Given** `fleet.local.json` already exists with no entry for
   `owner/repo`, **When** the operator runs `gh-aw-fleet add owner/repo
   --profile default --apply --yes`, **Then** the file is rewritten with
   the new `repos.owner/repo` entry containing `profiles: ["default"]`,
   and the command prints a "next step: run `gh-aw-fleet deploy
   owner/repo`" hint.
2. **Given** only `fleet.json` exists (no `fleet.local.json`), **When**
   the operator runs `gh-aw-fleet add owner/repo --profile default
   --apply --yes`, **Then** `fleet.local.json` is created as a minimal
   file containing only `version` and a `repos` map with the new entry
   (profiles, defaults, and peer repos are NOT copied — they remain in
   `fleet.json` and are picked up by `LoadConfig`'s merge), and the
   public `fleet.json` is left unchanged.
3. **Given** any fleet config, **When** the operator runs
   `gh-aw-fleet add owner/repo --profile default` (no `--apply`),
   **Then** no file on disk changes and the command prints a preview
   showing the count and names of workflows that would be deployed,
   plus the profile list.

---

### User Story 2 — Validate before writing (Priority: P1)

The operator wants the tool to refuse unsafe or incoherent additions —
duplicate repos, unknown profiles, malformed slugs — with clear
actionable errors, not silent overwrite or cryptic JSON decoder
messages. This is the guardrail that makes story 1 trustworthy.

**Why this priority**: Hand-editing JSON is error-prone; replacing that
with a command that silently overwrites or produces subtly-broken
configs would be worse than the current workflow. Validation must ship
with the happy path.

**Independent Test**: Invoke the command with each of the failure
conditions below; verify each exits non-zero and prints an operator-
readable error that names the offending input.

**Acceptance Scenarios**:

1. **Given** `owner/repo` already exists in the merged fleet config
   (whether from `fleet.json`, `fleet.local.json`, or both), **When**
   the operator runs `gh-aw-fleet add owner/repo --profile default`,
   **Then** the command exits non-zero with an error naming the repo
   and the source file, and no preview is printed.
2. **Given** the operator passes `--profile nonexistent`, **When** the
   command resolves profiles, **Then** it exits non-zero listing the
   profiles available in the merged config.
3. **Given** the operator passes an argument that is not in
   `owner/repo` slug form (e.g., `just-a-name`, `owner/`, `/repo`, an
   empty string), **When** the command parses positional arguments,
   **Then** it exits non-zero with a validation error.

> **Note**: Two additional failure conditions — unknown `--engine`
> value and no-op `--exclude` — are intrinsically tied to flags
> introduced in User Story 3 and are therefore specified as part of
> US3's acceptance scenarios. US2 covers only the failure conditions
> testable with the US1 flag surface (`--profile`, positional slug).

---

### User Story 3 — Express per-repo customization at onboarding time (Priority: P2)

The operator sometimes needs a new repo to diverge from pure profile
membership on day one — excluding a workflow the profile includes,
adding one the profile doesn't, or overriding the default engine. They
want to express all of that on the `add` command line rather than
running `add` and then hand-editing.

**Why this priority**: Most onboardings will use the `--profile
default` case only (story 1). Per-repo divergence is common enough to
require command-line flags but not so dominant that story 1 is blocked
on it.

**Independent Test**: Add a repo with `--profile default --exclude
ci-doctor --extra-workflow custom-thing --engine claude`; inspect the
written `fleet.local.json` and confirm all four fields are populated.

**Acceptance Scenarios**:

1. **Given** `--exclude` is passed one or more times, **When** the
   repo is written, **Then** each excluded workflow name appears once
   in the `exclude` array under the new `repos.owner/repo` entry.
2. **Given** `--extra-workflow` is passed one or more times, **When**
   the repo is written, **Then** each extra workflow name appears as
   an entry in the `extra` array. Default source for an extra workflow
   is `local` unless a `name@source` (or `name@source@ref`) form is
   used.
3. **Given** `--engine claude`, **When** the repo is written, **Then**
   the `engine` field is set on the new repo entry (overriding the
   fleet default).
4. **Given** none of `--exclude`, `--extra-workflow`, `--engine` are
   passed, **When** the repo is written, **Then** those JSON fields are
   omitted (they have `omitempty`) so the entry is minimal.
5. **Given** the operator passes `--engine unknown-engine-xyz`,
   **When** the command validates the engine override, **Then** it
   exits non-zero with an error naming the unsupported engine and
   listing the accepted values (the keys of `EngineSecrets`).
6. **Given** the operator passes `--exclude workflow-x` where
   `workflow-x` is not a member of any of the selected profiles,
   **When** the command resolves workflows, **Then** it emits a
   warning on stderr naming the offending exclusion but exits zero
   (the exclusion is a no-op; the operation still proceeds).

---

### Edge Cases

- **Neither `fleet.json` nor `fleet.local.json` exists.** The command
  should exit non-zero with the same "no config found" error
  `LoadConfig` already emits. Bootstrapping an empty fleet from the
  `add` command is out of scope for this issue.
- **`fleet.json` and `fleet.local.json` disagree on a profile
  definition.** The merged view (local wins) is what gets validated
  against. The operator's `--profile` must resolve in the merged view.
- **`--apply` is passed without `--yes` in a non-interactive shell.**
  The command must either prompt (when stdin is a TTY) or fail with an
  actionable "re-run with `--yes` to confirm" message (when stdin is
  not a TTY). Silent non-interactive writes are forbidden.
- **Concurrent invocations.** Two `add --apply` calls racing on the
  same file could lose one entry. The tool already uses atomic rename
  in `writeJSON`, so the loser gets clobbered but no partial file is
  produced. Acceptable for a local single-operator CLI; documented in
  assumptions.
- **Slug casing.** `Owner/Repo` and `owner/repo` are treated as
  distinct keys by Go's `map[string]RepoSpec`. The command should
  normalize to lowercase at parse time (GitHub URLs are
  case-insensitive) to avoid silent duplicates.
- **Repo already exists in `fleet.json` (public example) but not in
  `fleet.local.json`.** This is NOT a duplicate from the operator's
  perspective — they may legitimately want to shadow the public
  example with a local entry. The command should treat presence in the
  merged view as a duplicate, however, because overriding via `add`
  without an explicit gesture is surprising. Exit non-zero; document
  the workaround ("hand-edit `fleet.local.json` if you want to shadow
  a `fleet.json` entry").

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The CLI MUST expose `gh-aw-fleet add <owner/repo>` as a
  top-level subcommand, replacing the existing stub in `cmd/stubs.go`.
- **FR-002**: The command MUST be dry-run by default, matching the
  `deploy` / `sync` / `upgrade` convention, and MUST require an explicit
  `--apply` flag to write to disk.
- **FR-003**: When `--apply` is passed, the command MUST additionally
  require confirmation via either a `--yes` flag or an interactive
  stdin prompt; it MUST exit non-zero if neither is available.
- **FR-004**: The command MUST accept a single positional argument in
  `owner/repo` form. It MUST normalize casing to lowercase before
  comparing or writing. It MUST reject malformed slugs (missing slash,
  empty halves, whitespace).
- **FR-005**: The command MUST accept `--profile <name>` (repeatable or
  comma-separated, matching how `RepoSpec.Profiles` is modeled as a
  list). At least one profile MUST be specified unless the operator
  explicitly passes `--no-profile` (out of scope for v1 — v1 requires
  `--profile`).
- **FR-006**: The command MUST accept `--engine <name>` (zero or one);
  if provided, it MUST validate the name against the keys of the
  existing `EngineSecrets` map in `internal/fleet/deploy.go`, rejecting
  unknown values with an error that lists the accepted names. The
  `deploy` path's current permissive behavior (skip secret check on
  unknown engine) is unchanged; `add` is deliberately stricter so that
  onboarding-time typos never reach `deploy`.
- **FR-007**: The command MUST accept `--exclude <workflow-name>` as a
  repeatable flag and populate `RepoSpec.ExcludeFromProfiles`.
- **FR-008**: The command MUST accept `--extra-workflow <spec>` as a
  repeatable flag and populate `RepoSpec.ExtraWorkflows`. The flag
  value MUST accept the existing `gh aw`-style spec syntax operators
  already use in `fleet.json`:
  - Bare `name` → `Name=name`, `Source="local"`, `Ref=""`, `Path=""`.
  - `owner/repo/name@ref` (3-part agentics layout) →
    `Name=name`, `Source="owner/repo"`, `Ref=ref`, `Path=""`.
  - `owner/repo/.github/workflows/name.md@ref` (4-part gh-aw layout) →
    `Name=name` (derived from the basename), `Source="owner/repo"`,
    `Ref=ref`, `Path=".github/workflows/name.md"`.
  A single parser MUST populate all four `ExtraWorkflow` fields from
  one flag value; invalid forms MUST be rejected with an error that
  shows an example of the correct syntax.
- **FR-009**: Before any validation, the command MUST load the fleet
  config via the existing `LoadConfig` routine (base + local merge
  semantics).
- **FR-010**: The command MUST reject adding a repo that already exists
  in the merged fleet view, naming the source file (`fleet.json`
  and/or `fleet.local.json`) in the error message.
- **FR-011**: The command MUST validate that every `--profile` name
  exists in the merged `Profiles` map; on failure, it MUST list the
  available profiles in the error output.
- **FR-012**: The command MUST resolve the candidate `RepoSpec` through
  the existing `ResolveRepoWorkflows` logic to produce the workflow set
  used in the preview, exercising the same code path `deploy` will use.
- **FR-013**: In dry-run mode, the command MUST print a preview in
  plain text using the same style as `deploy`'s dry-run and `list`'s
  summary. The preview MUST contain:
  - A header line on **stderr** naming the target repo slug, the
    profile list, and the resolved workflow count, e.g.:
    `would add rshade/foo with profiles [default] (12 workflows)`.
  - A bulleted list of the resolved workflow names on **stdout**, one
    per line, in resolution order (profile membership first, then
    extras), with each line prefixed by a visual marker (e.g., `- `).
  - Any resolution warnings (e.g., an `--exclude` that didn't match
    anything in the selected profiles) printed on **stderr** after the
    header.
  - A final "next step" hint on stderr telling the operator to re-run
    with `--apply` to persist.
  No machine-readable (JSON) output mode is provided in v1; the
  stdout/stderr split is the only structure acceptance tests should
  rely on.
- **FR-014**: In `--apply` mode, after confirmation, the command MUST
  write the updated config exclusively to `fleet.local.json` — never
  to `fleet.json`. It MUST NOT modify the public `fleet.json` under any
  circumstance, including when `fleet.local.json` did not previously
  exist.
- **FR-015**: When `fleet.local.json` does not exist and only
  `fleet.json` is present, the command on `--apply` MUST synthesize a
  **minimal** `fleet.local.json` containing exactly:
  - `version` set to the current `SchemaVersion`.
  - `repos` populated with a single entry for the new repo.
  It MUST NOT copy `defaults`, `profiles`, or peer `repos` entries
  from `fleet.json` into the new local file. The existing
  `mergeConfigs` helper in `internal/fleet/load.go` supplies those
  fields at load time by merging `fleet.json` underneath, so the
  minimal local file remains correct for all downstream commands and
  `fleet.json` stays authoritative for shared state. The command MUST
  surface this transition in the output ("creating fleet.local.json
  (minimal; profiles/defaults still resolved from fleet.json)").
- **FR-016**: The command MUST write `fleet.local.json` atomically
  (temp file + rename), matching the existing `writeJSON` pattern, so a
  crash mid-write cannot leave a partial file.
- **FR-017**: After a successful `--apply`, the command MUST print the
  next step: `gh-aw-fleet deploy <owner/repo>`.
- **FR-018**: On any error, the command MUST exit non-zero; on dry-run
  success it MUST exit zero; on `--apply` success it MUST exit zero.
- **FR-019**: The `fleet-onboard-repo` skill at
  `skills/fleet-onboard-repo/SKILL.md` MUST be updated so the JSON-
  editing step is replaced with a `gh-aw-fleet add` invocation.
- **FR-020**: The README's CLI / Quickstart section MUST be updated to
  document the `add` command alongside `deploy`, `sync`, and `upgrade`.
- **FR-021**: The existing `SaveConfig` helper at
  `internal/fleet/load.go` MUST be adjusted or supplemented so that
  saving via the `add` path always targets `fleet.local.json`. Calling
  `SaveConfig` with intent to modify `fleet.json` from the `add` flow
  MUST be impossible by construction, not just by convention.

### Key Entities

- **RepoSpec (existing)**: The per-repo desired state in
  `internal/fleet/schema.go` — profiles, engine override, extra
  workflows, exclusions, overrides. `add` constructs a new instance and
  inserts it into `Config.Repos`.
- **Config (existing)**: The top-level declarative state. `add` reads
  it via `LoadConfig` and writes a modified version to
  `fleet.local.json`.
- **AddResult (new)**: Return value from `fleet.Add()`. Carries the
  target repo, selected profiles, resolved workflow list, engine
  override (if any), non-fatal warnings, and — in `--apply` mode —
  flags recording whether and where `fleet.local.json` was written
  (including whether the file had to be synthesized from a
  `fleet.json`-only baseline). Used to render the dry-run preview
  AND as the test-assertion surface. See `data-model.md` for the
  exact field list.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can onboard a new repo with the default
  profile in a single command (no text editor, no JSON hand-typing).
  Measured by: the documented quickstart procedure reduces from
  ≥6 manual steps (edit file, re-read schema, save, run list, etc.) to
  1 command.
- **SC-002**: 100% of invocations that would produce an invalid
  `fleet.local.json` (duplicate key, unknown profile, malformed slug,
  unknown engine) are rejected before any file write occurs.
- **SC-003**: 0 invocations of `add` modify `fleet.json`. Verifiable by
  inspection of the command's file-write call sites and by integration
  test.
- **SC-004**: The `fleet-onboard-repo` skill's post-update instructions
  fit in ≤5 lines and contain no JSON editing guidance.
- **SC-005**: Dry-run preview output is understandable on first read:
  the repo slug, profile list, and workflow count are all visible
  without scrolling on an 80-column terminal for profiles with ≤20
  workflows.
- **SC-006**: The command completes (dry-run or `--apply`) in under 2
  seconds on a warm cache, with no network calls required beyond what
  `LoadConfig` and `ResolveRepoWorkflows` already perform (which today
  is zero).

## Assumptions

- The operator already has a valid `fleet.json` or
  `fleet.local.json` checked out. Bootstrapping an empty fleet
  (`fleet init`) is out of scope and tracked separately.
- The set of valid engine names comes from the keys of the
  `EngineSecrets` map declared in `internal/fleet/deploy.go`. `add`
  validates against that map; `deploy` continues its current permissive
  behavior (unknown engine → skip secret check, no rejection). Keeping
  the allowlist in one place (`EngineSecrets`) means adding a new
  supported engine still only requires one edit.
- The operator onboards one repo per `add` invocation. Bulk add (e.g.,
  reading a list of slugs from stdin) is out of scope.
- `Overrides` (the per-workflow `map[string]string` field on
  `RepoSpec`) is not exposed via `add` flags in v1 — it is rarely set
  at onboarding time, and the few operators who need it can hand-edit
  `fleet.local.json` after running `add`.
- Because `LoadConfig` merges local over base, duplicate detection
  uses the merged view: if the repo exists anywhere (base or local),
  `add` treats it as a duplicate and refuses. Shadowing a public
  `fleet.json` entry with a private local override is an intentionally-
  restricted workflow that still requires hand-editing.
- `--apply` confirmation in non-TTY environments (CI, hooks, piped
  shells) requires `--yes`. This is intentionally stricter than the
  interactive flow because automated use of `add` is unusual and the
  write target is a source-of-truth file.
- The JSON output format of `fleet.local.json` after `add --apply`
  uses the same `MarshalIndent("", "  ")` style that `writeJSON`
  already produces; key ordering follows Go's struct field order for
  `Config` (stable) and alphabetical order for the `Repos` /
  `Profiles` maps (from `encoding/json`'s map handling).
