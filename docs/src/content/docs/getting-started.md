---
title: Getting Started
description: Take an empty fleet to a reviewed pull request by installing a workflow profile onto a repository you own.
---

This tutorial walks you from nothing to a real pull request that adds the
`default` profile's agentic workflows to a repository you own. By the end you
will have registered a repo in your fleet, previewed the change, opened a PR, and
confirmed the repo is aligned.

Everything `gh-aw-fleet` does to a repository happens through a pull request you
review and merge yourself. Nothing is force-pushed and nothing is committed to
`main` for you, so this tutorial is safe to follow on a repository you control.

**You will need** the tools from the [Install](/gh-aw-fleet/install/) guide: the `gh` CLI
authenticated with `repo` and `workflow` scopes, the `gh aw` extension pinned to
`v0.79.2`, and the `gh-aw-fleet` binary on your `PATH`. Set aside about ten
minutes.

Throughout, replace `you/your-repo` with a repository you own.

## Step 1 — Confirm your tools are ready

Run these three checks:

```bash
gh auth status
gh extension list          # gh-aw should appear, pinned to v0.79.2
gh-aw-fleet list
```

`gh-aw-fleet list` prints the repos it already tracks and, on stderr, which
config files it loaded — `(loaded fleet.json)` on a fresh checkout. If all three
commands succeed, you are ready.

## Step 2 — Register your first repo

Add your repo to the fleet with the baseline `default` profile. Run it first
without `--apply` to see what it would write:

```bash
gh-aw-fleet add you/your-repo --profile default
```

This is a dry-run: it prints the planned entry and changes nothing. When the plan
looks right, write it for real:

```bash
gh-aw-fleet add you/your-repo --profile default --apply
```

The command asks you to confirm; answer `y`. It writes the entry to
**`fleet.local.json`** — your private, gitignored fleet state — not to the
committed `fleet.json`. Run `gh-aw-fleet list` again and your repo now appears in
the table.

## Step 3 — Preview the deploy

Now see exactly what would be installed, still without touching the repo:

```bash
gh-aw-fleet deploy you/your-repo
```

`deploy` is dry-run by default. It resolves the `default` profile into a concrete
set of workflows, pinned to the fleet's source refs, and prints the plan. Read
it: these are the workflow files the pull request will add.

## Step 4 — Open the pull request

When the plan looks right, apply it:

```bash
gh-aw-fleet deploy you/your-repo --apply
```

This clones the repo to a scratch directory, runs `gh aw` to add and compile the
workflows, commits them, pushes a branch, and opens a PR. When it finishes it
prints the PR URL. Open it — you will see the added `.github/workflows/` files,
ready for review.

If the commit stops on a gpg signing prompt or error, that is expected on
signing-required setups. Follow
[Recover from a gpg signing failure](/gh-aw-fleet/recover-from-gpg-failure/) to finish the
commit in your own shell, then come back here.

## Step 5 — Confirm it worked

Review and **merge** the pull request on GitHub. Once it is merged, ask the fleet
whether the repo now matches its declared state:

```bash
gh-aw-fleet status you/your-repo
```

The repo reports `aligned` — the workflows on `main` match what the `default`
profile declares. For a fuller picture of the repo's recent run health and
credit spend, run:

```bash
gh-aw-fleet overview you/your-repo
```

## What you did, and where to go next

You registered a repo, previewed a change, opened a reviewable PR, and confirmed
the repo is aligned — the full `gh-aw-fleet` loop, end to end.

From here:

- **Solve a specific problem** — the [How-to guides](/gh-aw-fleet/gate-ci-on-drift/) cover
  gating CI on drift, resuming an interrupted apply, and reviewing cost.
- **Look up a command or flag** — the [CLI reference](/gh-aw-fleet/cli/) lists every
  command, flag, and exit code.
- **Understand the model** — [Reconcile workflow](/gh-aw-fleet/reconcile/) and
  [Consumption and FinOps](/gh-aw-fleet/consumption/) explain how and why the tool behaves
  as it does.
