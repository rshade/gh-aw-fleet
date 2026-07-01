---
title: Recover from a gpg signing failure
description: Finish a deploy, sync, or upgrade by hand after gpg signing blocks the automated commit.
---

`gh-aw-fleet` runs `git commit` non-interactively during `--apply`. If your git
config requires gpg signing and the agent needs a passphrase, the commit has
nowhere to prompt and fails. This is the most common `--apply` failure.

The tool **never bypasses signing** — no `--no-gpg-sign`, no
`commit.gpgsign=false`. Instead it preserves the in-progress clone so you can
finish the commit in your own shell, where gpg-agent can prompt you.

## Before you start

You need the scratch clone the failed run left behind. It is preserved at:

```text
/tmp/gh-aw-fleet-<owner>-<repo>-<timestamp>/
```

with all workflow changes already staged on the deploy branch. Do not delete it.

## Finish the commit by hand

Change into the preserved clone and complete the commit, push, and PR yourself:

```bash
cd /tmp/gh-aw-fleet-<owner>-<repo>-<timestamp>
git commit -m 'ci(workflows): add <N> agentic workflows via gh-aw-fleet'
git push -u origin <branch-name>       # branch is fleet/deploy-<timestamp>
gh pr create \
  --title "ci(workflows): add <N> agentic workflows via gh-aw-fleet" \
  --body "Adds the declared workflow set via gh-aw-fleet."
```

Running `git commit` in your own shell lets gpg-agent prompt for your passphrase.
Commit messages follow Conventional Commits with the `ci(workflows)` scope; keep
the subject at or under 72 characters with no trailing period.

## Or let the tool resume

If you would rather the tool finish the push and PR, re-run the same command
against the preserved clone. It detects the state and picks up where it stopped
(see [Resume an interrupted apply](/gh-aw-fleet/resume-interrupted-apply/)):

```bash
gh-aw-fleet deploy <owner>/<repo> --apply --work-dir /tmp/gh-aw-fleet-<owner>-<repo>-<timestamp>
```

## Full template

The complete PR-body template, commit-message validation rules, and edge cases
(`gh aw init` output, skipped workflows) live in the
[`fleet-deploy` skill](https://github.com/rshade/gh-aw-fleet/blob/main/skills/fleet-deploy/SKILL.md).
