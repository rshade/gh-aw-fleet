---
title: Reconcile Workflow
description: How deploy, sync, and upgrade compute drift, dry-run changes, and open PRs.
---

`gh-aw-fleet` is a thin orchestrator around `gh aw`, `git`, and `gh`. It does not
rewrite workflow markdown. It resolves desired state from `fleet.json` and
`fleet.local.json`, then delegates the actual workflow operations upstream.

## The loop

1. Load config and resolve each repo's profiles, extras, exclusions, engine, and
   source refs.
2. Compare desired state with the target repo's current workflow files.
3. Run the relevant `gh aw` operation in a scratch clone.
4. Commit workflow changes in the target repo clone.
5. Open a pull request for review.

All mutating commands are dry-run by default. Nothing is pushed unless you pass
`--apply`.

## deploy

`deploy` installs the declared workflow set into a repo that may not already have
it.

```bash
gh-aw-fleet deploy acme/widgets
gh-aw-fleet deploy acme/widgets --apply
```

The dry-run exercises the same upstream surface used by apply, including
`gh aw init` and `gh aw add`, but does not push a branch or open a PR.

## sync

`sync` reconciles a repo back to the declared profile set. It adds missing
workflows and reports drift where reality no longer matches the fleet definition.

```bash
gh-aw-fleet sync acme/widgets
gh-aw-fleet sync acme/widgets --apply
```

Use `--force` when you intentionally want the fleet pin to overwrite existing
workflow frontmatter.

## upgrade

`upgrade` refreshes installed workflows and fleet init artifacts.

```bash
gh-aw-fleet upgrade acme/widgets
gh-aw-fleet upgrade --all
gh-aw-fleet upgrade --all --apply
```

One asymmetry matters: `gh aw update` follows each workflow's own frontmatter
`source:` line. Editing `fleet.json` pins alone does not re-pin already-installed
workflows during `upgrade`; use `sync --apply --force` when you need installed
workflow frontmatter to match current fleet refs.

## Security strict gate

`deploy`, `sync`, and `upgrade` accept `--strict` when HIGH Layer 1 security
findings should block the run. The flag is opt-in per invocation and is not
stored in `fleet.json` or `fleet.local.json`.

This is different from `gh aw compile --strict`. Compile-strict validates
generated GitHub Actions syntax and is controlled by `compile_strict` repo config.
The `gh-aw-fleet --strict` gate consumes the existing security scanner findings.

When the gate blocks:

- findings are still emitted on stderr and in JSON `warnings[]`;
- `findings.json` is written at the work-dir clone root;
- the clone is preserved for inspection, including dry-run temp clones;
- commit, push, and PR creation do not run.

Lower-severity findings and `promptinj:` findings remain advisory. For
`upgrade --all --strict --output json`, NDJSON records are emitted through the
blocked repo and then processing stops.

## The three-turn pattern

Any command that mutates external repositories follows this operator flow:

1. Dry-run and read the plan.
2. Give explicit approval in the next turn.
3. Run the same command with `--apply`.

That pattern keeps branch pushes and PR creation auditable. The tool never
force-pushes or commits directly to `main`.

## Failure breadcrumbs

Scratch clones under `/tmp/gh-aw-fleet-*` are preserved after an apply failure.
Do not delete them while debugging. Re-run with `--work-dir <clone>` to resume an
interrupted apply once the cause is fixed.

Strict security aborts preserve clones even during dry-run and add
`findings.json` to the clone root so the operator can inspect the exact scanner
output that caused the block.

## Diagnostics

Known upstream failure patterns are surfaced with actionable hints. Examples
include unknown workflow properties, missing upstream refs, GPG signing failures,
and GitHub API 404s.

Use debug logging when you need the subprocess trace:

```bash
gh-aw-fleet deploy acme/widgets --log-level=debug --log-format=json
```
