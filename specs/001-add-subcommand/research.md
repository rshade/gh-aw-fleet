# Research: `add <owner/repo>` Subcommand

**Phase**: 0 (outline + research)
**Feature**: `specs/001-add-subcommand`

All items from `spec.md`'s `## Clarifications` section are resolved
upstream. This document resolves the items `/speckit.clarify`
deferred to the planning phase, plus a small number of questions
that surfaced only once we started enumerating the implementation.

Each entry follows: **Decision** / **Rationale** / **Alternatives
considered**.

---

## 1. `--profile` flag mechanics (cobra `StringSliceVar` vs `StringArrayVar`)

**Decision**: Use `cmd.Flags().StringSliceVar(&flagProfiles, "profile", ...)`.

**Rationale**: `StringSliceVar` accepts both repeated flags
(`--profile a --profile b`) and comma-separated values
(`--profile a,b`). Operators using this from interactive shells will
prefer the comma form; operators using it from scripts will prefer
the repeated form. `StringSliceVar` gives both for free. No other
subcommand in this repo takes a multi-value flag yet, so there is no
project convention to match — this decision establishes the
convention.

**Alternatives considered**:

- `StringArrayVar`: forces repetition (`--profile a --profile b`);
  rejects commas. Fine for programmatic callers, unfriendly for
  interactive use. Rejected because comma-separated is a universal
  CLI idiom.
- Custom parsing of a single `StringVar`: would re-implement what
  cobra gives us for free. Rejected per Principle I (don't
  re-implement upstream behavior).

## 2. `SaveConfig` → `SaveLocalConfig` rename

**Decision**: Delete the existing `SaveConfig(dir, c *Config) error`
function in `internal/fleet/load.go:96-99` and replace it with
`SaveLocalConfig(dir string, c *Config) error`, which writes
unconditionally to `fleet.local.json` (not `fleet.json`). Also
export a small `BuildMinimalLocalConfig(repo string, spec RepoSpec)
*Config` helper for the FR-015 synthesis path.

**Rationale**:

- `SaveConfig` has zero callers today (`grep -r SaveConfig` finds
  only its own definition in the source tree — all other hits are in
  the spec docs). Deleting it is safe.
- Renaming the function (instead of adding a new one alongside)
  satisfies FR-021's "impossible by construction" constraint: after
  the rename, no function in the package can write to `fleet.json`.
  If a future caller wants that ability, they must consciously add
  it and justify it — which is exactly the friction FR-021 asks for.
- The small synthesis helper keeps `cmd/add.go` free of schema
  construction logic. It also makes the "minimal file" contract
  (from the 4th clarification) easy to unit-test in isolation.

**Alternatives considered**:

- Keep `SaveConfig` and add `SaveLocalConfig` alongside. Rejected
  because `SaveConfig`'s continued existence contradicts FR-021 —
  the impossibility must be structural, not merely "no current
  caller uses the dangerous function."
- Make `SaveLocalConfig` unexported. Rejected because unit tests in
  `internal/fleet/add_test.go` want to exercise it; same-package
  tests would still have access, but exporting it leaves the door
  open for reuse by a future `remove`/`edit` command without needing
  a refactor.

## 3. Warning emission rules for FR-013

**Decision**: Emit warnings on stderr (between the header line and
the `next step` hint) in exactly three cases:

1. `--exclude <name>` where `<name>` does not appear in any workflow
   list of any selected profile.
2. `--extra-workflow <spec>` whose resolved `Name` already appears
   in one of the selected profiles' workflow lists (the existing
   `ResolveRepoWorkflows` dedup at `internal/fleet/load.go:183-185`
   will silently drop it; operators need to know).
3. The resolved workflow count is zero. This almost always indicates
   a typoed `--profile` name whose resolution succeeded but yielded
   nothing, or an overly aggressive `--exclude`. Exit remains zero
   but the warning is loud.

**Rationale**:

- These are the only cases where the user's command-line intent
  silently differs from the written config. Every other class of
  error (unknown profile, malformed slug, duplicate repo, unknown
  engine) is a hard error that exits non-zero.
- No warning for `--engine` overriding a fleet default — that's an
  intentional user gesture, not a silent divergence.
- Warnings on stderr keep stdout pristine for the workflow-list
  output (relevant if an operator pipes stdout through `grep` /
  `wc -l`).

**Alternatives considered**:

- Also warn for `--extra-workflow` with a non-"local" source that
  lacks a corresponding profile source pin. Rejected because
  `ExtraWorkflow.Ref` is set inline from the flag value — extras
  don't depend on profile pins.
- Error (not warn) on zero resolved workflows. Rejected because a
  profile could legitimately be defined with zero workflows during
  onboarding of a new profile; forcing `add` to fail would block
  that workflow.

## 4. TTY detection for `--apply` confirmation

**Decision**: Use `term.IsTerminal(int(os.Stdin.Fd()))` from
`golang.org/x/term`. Add the dependency only if not already
present; prefer an existing dep if one provides the same primitive.

**Rationale**:

- This is the standard Go idiom for "am I attached to a terminal."
  `os.Stdin.Stat()` + `Mode()&os.ModeCharDevice` also works but is
  less readable and has subtle portability footguns.
- `golang.org/x/term` is a common indirect transitive dependency in
  Go projects that use cobra; we should confirm it isn't already in
  the indirect-requires section before adding it explicitly.
- If we do add it, it is a tiny, well-maintained package from the
  Go team — low dependency risk.

**Alternatives considered**:

- `github.com/mattn/go-isatty`: larger dep, vestigial from older Go
  versions that lacked `x/term`. Rejected.
- Skip TTY detection entirely; always require `--yes`. Rejected
  because the spec's FR-003 explicitly allows interactive prompt in
  TTY mode, and removing that would break the "friendly CLI" UX
  this feature is built to deliver.

**Verification step**: Before adding `golang.org/x/term` to
`go.mod`'s `require` block, run `go mod graph | grep 'golang.org/x/term'`
to check if cobra or another direct dep already pulls it in as a
transitive. If so, promote it to a direct require (no new dep) rather
than adding a new one.

## 5. Interactive prompt wording and response parsing

**Decision**: Prompt text: `Write fleet.local.json? [y/N] `.
Accept responses: `y`, `Y`, `yes`, `YES` → proceed; anything else →
abort with exit 1 and the message
`aborted: re-run with --apply --yes to confirm`.

**Rationale**:

- Single-char default `N` is the standard Unix CLI convention for
  "destructive operation confirmation."
- Case-insensitive y/yes keeps the prompt forgiving without
  accepting ambiguous input like `maybe`.
- The abort message tells the operator exactly what to type to
  re-run non-interactively, closing the UX loop.

**Alternatives considered**:

- Full-word-only prompt (`yes`/`no`, no single-letter). Rejected as
  unidiomatic for Unix CLIs.
- Abort silently on anything-not-`y`. Rejected because the operator
  might have hit Enter by accident; the error message points them
  at the remedy.

## 6. Slug normalization rules (FR-004)

**Decision**: `validateSlug(s string) (string, error)` implements:

- Trim leading/trailing whitespace.
- Reject empty string.
- Split on `/`. Require exactly 2 parts, both non-empty and
  non-whitespace.
- Lowercase both halves.
- Allowed character set: `[a-z0-9._-]+` for each half (matches
  GitHub's slug rules — no spaces, no `/` inside a half after
  the split).
- Return the lowercased `owner/repo` form.

**Rationale**: These are GitHub's repo-slug rules. Using a stricter
superset than GitHub allows (e.g., forbidding `.`) would reject
valid repo names like `owner/foo.bar`.

**Alternatives considered**:

- Validate against the exact GitHub regex (which requires owner
  name to start with an alphanumeric and not end with a hyphen).
  Rejected as over-engineering — this is local validation to catch
  typos, not a security boundary. If the slug slips past
  `validateSlug` but is later rejected by GitHub, `deploy` will
  surface that.

## 7. `--extra-workflow` parse implementation (elaborates FR-008)

**Decision**: `parseExtraWorkflowSpec(s string) (ExtraWorkflow, error)`
with the following algorithm:

```text
1. If s contains no "/", treat as bare name:
     return ExtraWorkflow{Name: s, Source: "local"}
2. Otherwise split on "@":
     lhs is everything before the first "@" (required)
     ref is everything after the first "@" (required; empty ref → error)
3. Split lhs on "/":
     a. len == 2 (unexpected — looks like "owner/repo" with no path):
        → error, suggest 3-part or 4-part form
     b. len == 3: treat as "owner/repo/name"
        → ExtraWorkflow{Name: parts[2], Source: "parts[0]/parts[1]", Ref: ref}
     c. len >= 4 AND parts[2] == ".github" AND parts[3] == "workflows":
        path = ".github/workflows/" + parts[4:].join("/")
        name = basename of parts[-1] with ".md" suffix stripped
        → ExtraWorkflow{Name: name, Source: parts[0]+"/"+parts[1],
                        Ref: ref, Path: path}
     d. otherwise: → error with example
```

**Rationale**: Mirrors the layout distinctions encoded in
`SourceLayout` (from `internal/fleet/fetch.go`, referenced in
CLAUDE.md). The 4-part form must specifically start with
`.github/workflows/` to match how `gh-aw`-layout sources are
addressed everywhere else in the project.

**Alternatives considered**:

- Auto-detect based on whether the `Source` is a known key in any
  loaded profile's `Sources` map. Rejected: the operator might be
  adding an extra from a source not yet referenced by any profile,
  and the parse should work without loading the config.
- Accept both `@ref` and a ref-less form. Rejected for remote
  sources: a pin is mandatory to be reproducible (matches how
  `profile.Sources[source].Ref` is mandatory). Bare-name local
  extras legitimately have no ref.

## 8. Preview output formatting details (elaborates FR-013)

**Decision**:

- Header line on stderr, format:
  `would add <repo> with profiles [<csv>] (<N> workflows)`
  e.g., `would add rshade/foo with profiles [default] (12 workflows)`
- Workflow list on stdout, one per line, format: `- <name>`.
  Workflows appear in resolution order: profile membership order
  (from `ResolveRepoWorkflows`), then extras.
- Engine override line (only if `--engine` was passed): on stderr
  after the header, format: `engine override: <name>`.
- Warnings on stderr after any engine override line, format:
  `warning: <message>`.
- Final line on stderr (dry-run only): `next: re-run with --apply to persist`.
- Final line on stderr (`--apply` success): `next: gh-aw-fleet deploy <repo>`.

**Rationale**: Matches the cadence of `cmd/deploy.go`'s
`printDeploy` function (see `cmd/deploy.go:57-80`) — a header
summary followed by indented detail lines. Keeping stdout limited
to the workflow list is what enables pipe-friendliness without a
`--output` flag.

**Alternatives considered**:

- Put everything on stdout for simplicity. Rejected because
  operators who pipe stdout to `wc -l` expect it to equal the
  workflow count, not the count plus 3 status lines.

## 9. Test matrix (elaborates Testing Strategy in spec.md)

**Decision**: `internal/fleet/add_test.go` has four table-driven
test functions:

- `TestValidateSlug(t *testing.T)` — parsing/casing/invalid-form
  cases.
- `TestParseExtraWorkflowSpec(t *testing.T)` — all four branches
  of FR-008 syntax plus error cases.
- `TestAdd_DryRun(t *testing.T)` — happy path, duplicate-repo,
  unknown-profile, unknown-engine, no-op exclude, zero-workflow
  warning.
- `TestAdd_Apply(t *testing.T)` — actually writes to `t.TempDir()`;
  asserts file contents exactly (minimal file per FR-015), and
  confirms `fleet.json` at the temp dir is untouched.
- `TestBuildMinimalLocalConfig(t *testing.T)` — direct test of the
  synthesis helper to anchor the FR-015 contract.

Integration test (documented in `quickstart.md`, not automated in
this PR):

- Fresh checkout scenario, no `fleet.local.json` → `add --apply
  --yes` → inspect written file, run `gh-aw-fleet list`, confirm
  new entry appears with resolved workflow count.

**Rationale**: Unit tests cover all decision branches; the
integration test covers the one thing unit tests can't — that
`list` correctly surfaces a newly-added repo after round-tripping
through the file system. Matches the constitutional carve-out for
build-green + real-world dry-run: `add` exercises its own real
dry-run via the preview, and integration verifies round-trip.

**Alternatives considered**:

- Add a separate `cmd/add_integration_test.go` that shells out to
  the built binary. Rejected because the test adds a build
  dependency and duplicates what the `TestAdd_Apply` temp-dir test
  already covers. The integration scenario stays in the PR's
  manual-verification checklist.

---

**Research outcome**: No NEEDS CLARIFICATION remain. All deferred
items resolved with decisions justified against the constitution.
Ready for Phase 1 design.
