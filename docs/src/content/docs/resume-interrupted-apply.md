---
title: Resume an interrupted apply
description: Re-run deploy, sync, or upgrade against a preserved clone after a mid-pipeline failure, without re-cloning.
---

When an `--apply` fails partway through — a gpg signing error, a network blip, a
`gh aw` failure — `gh-aw-fleet` preserves the scratch clone instead of deleting
it. You can point a re-run at that clone with `--work-dir` to continue from where
it stopped rather than starting over.

## Find the preserved clone

Failed applies leave their clone at:

```text
/tmp/gh-aw-fleet-<owner>-<repo>-<timestamp>/
```

These directories are breadcrumbs — do not delete them while you are still
recovering.

## Re-run against the clone

Pass the clone to the same command with `--work-dir`. This skips `git clone`, the
auto-cleanup, and (when the clone is already prepared) the earlier pipeline
stages:

```bash
gh-aw-fleet deploy <owner>/<repo> --apply --work-dir /tmp/gh-aw-fleet-<owner>-<repo>-<timestamp>
```

The same flag works for `sync` and `upgrade`.

## Where the resume picks up

`deploy --apply --work-dir` inspects the clone and resumes at the right gate:

- **Staged workflow changes, no commit yet** → resumes at the commit gate,
  skipping clone, `gh aw init`, and `gh aw add`.
- **You committed by hand (e.g. after a gpg failure) but did not push** → the
  tool detects the unpushed commit and resumes at the push gate, going straight
  to `git push` and `gh pr create`.

## If the run produced security findings

An `--apply` that surfaced findings pauses for an interactive confirmation. When
resuming non-interactively, add `--yes` to proceed past that prompt (the findings
still print on stderr and land in the PR body):

```bash
gh-aw-fleet deploy <owner>/<repo> --apply --work-dir <clone> --yes
```

## See also

- [Recover from a gpg signing failure](/gh-aw-fleet/recover-from-gpg-failure/) — the most
  common reason to resume.
- [Reconcile workflow](/gh-aw-fleet/reconcile/) — how the apply pipeline is staged.
