# Quickstart: `gh-aw-fleet add <owner/repo>`

**Audience**: operators onboarding a repo into the fleet for the
first time, or adding a second/third/Nth repo.
**Time**: ~30 seconds for a standard onboarding.

This is the operator-facing walkthrough. For the full command
contract, see `contracts/cli.md`. For implementation details, see
`plan.md`.

## Prerequisites

- `gh-aw-fleet` built and on your `PATH` (or run via `go run .`).
- A working directory with at least `fleet.json` present (a
  `fleet.local.json` is optional — if it's absent, `add` will
  create a minimal one on your first `--apply`).
- You know which profile you want the repo to use (`default` is
  the standard starting point).

## Step 1 — Dry-run

Preview what `add` would write, without touching anything on disk:

```bash
gh-aw-fleet add rshade/my-new-repo --profile default
```

Expected output (example):

```text
# stderr:
would add rshade/my-new-repo with profiles [default] (12 workflows)
next: re-run with --apply to persist

# stdout:
- ci-doctor
- security-guardian
- perf-guardian
- ... (9 more)
```

If the header shows an unexpected profile list, workflow count, or
missing workflows, **stop** — you have a flag wrong. Re-run with
corrected flags. Nothing has been written.

## Step 2 — Apply

When the dry-run preview looks right:

```bash
gh-aw-fleet add rshade/my-new-repo --profile default --apply --yes
```

Expected output:

```text
# stderr:
added rshade/my-new-repo with profiles [default] (12 workflows)
next: gh-aw-fleet deploy rshade/my-new-repo
```

If `fleet.local.json` did not exist before this run, you will also
see (on stderr):

```text
creating fleet.local.json (minimal; profiles/defaults still resolved from fleet.json)
```

This is expected and correct. See the FAQ below for what that
minimal file looks like.

## Step 3 — Verify

```bash
gh-aw-fleet list
```

Your new repo should appear alongside any others, with the
resolved workflow count matching what Step 1 / Step 2 reported.

## Step 4 — Deploy (optional, separate command)

`add` only writes the declarative state. To actually install the
workflows onto the target repo, follow up with:

```bash
gh-aw-fleet deploy rshade/my-new-repo         # dry-run
gh-aw-fleet deploy rshade/my-new-repo --apply # opens PR
```

## Customizing at onboarding time

### Override the engine

```bash
gh-aw-fleet add rshade/my-new-repo --profile default --engine claude --apply --yes
```

Accepted engines: `copilot`, `claude`, `codex`, `gemini`. (Whatever
`EngineSecrets` declares is the authoritative list; any other value
is rejected up front.)

### Exclude a workflow the profile includes

```bash
gh-aw-fleet add rshade/my-new-repo \
    --profile default \
    --exclude perf-guardian \
    --apply --yes
```

If `perf-guardian` isn't actually in `default`, you'll see a
warning — but the add still succeeds (the exclusion is a no-op).

### Add an extra workflow the profile doesn't include

Three forms, depending on where the workflow comes from:

```bash
# Local workflow (lives in the target repo):
--extra-workflow my-repo-specific-thing

# Agentics 3-part form:
--extra-workflow githubnext/agentics/security-guardian@v0.4.1

# gh-aw 4-part form:
--extra-workflow github/gh-aw/.github/workflows/custom.md@v0.68.3
```

## Duplicate-repo error — what it means

```text
error: rshade/my-new-repo already exists in fleet.json + fleet.local.json
```

The repo is already tracked (in either file). `add` refuses to
overwrite. Options:

1. Hand-edit `fleet.local.json` if you intend to shadow a
   `fleet.json` entry.
2. Run `gh-aw-fleet list` to confirm you didn't already add this
   repo.
3. Use a different slug (double-check casing — `Foo/Bar` normalizes
   to `foo/bar`).

## Manual integration test (for reviewers of PR #9)

Steps to verify this feature end-to-end after merging:

1. In a fresh worktree, delete `fleet.local.json` if it exists.
2. Run `gh-aw-fleet list` and confirm the pre-existing repo set.
3. Run `gh-aw-fleet add testowner/testrepo --profile default`.
   Confirm dry-run preview lists the expected workflow set; no
   file changes.
4. Run `gh-aw-fleet add testowner/testrepo --profile default
   --apply --yes`. Confirm:
   - `fleet.local.json` now exists.
   - Its contents are **minimal**: exactly
     `{"version": 1, "repos": {"testowner/testrepo": {"profiles": ["default"]}}}`.
   - `fleet.json` is byte-identical to before.
5. Run `gh-aw-fleet list` again. Confirm `testowner/testrepo`
   appears with a resolved workflow count.
6. Run `gh-aw-fleet add testowner/testrepo --profile default`.
   Confirm it fails with the duplicate-repo error.
7. Clean up: `git checkout -- .` (the test repo entry is in
   `fleet.local.json` which is gitignored, so it stays; delete it
   manually if you want a clean state).

## FAQ

### Why is `fleet.local.json` so small after the first `add`?

By design. `add` writes a minimal delta: only `version` and the
new `repos` entry. Profiles, defaults, and peer repos continue to
resolve from `fleet.json` via `LoadConfig`'s merge logic. This
keeps `fleet.json` authoritative for shared state; your
`fleet.local.json` stays a focused "what I changed locally" file.

### Why can't I just re-run `add` to change a repo's profile?

v1 scope. `add` is strictly additive: it refuses if the repo
already exists. Editing an existing entry requires hand-editing
`fleet.local.json`. A future `edit` command may lift this.

### Can I `add` without a profile?

No. `--profile` is required in v1. A repo with zero profiles is
not meaningfully tracked by the fleet — `deploy` wouldn't know
what to install. If you want a repo tracked but deployed with only
`--extra-workflow` entries, that's out of scope for v1.

### What happens if two operators run `add --apply` at the same time?

One will win, the other's entry will be lost. `add` doesn't take a
lock. The atomic-rename write pattern guarantees the file is never
partially written, but last-write-wins applies. This is acceptable
for a local single-operator CLI; documented as an assumption in
the spec.
