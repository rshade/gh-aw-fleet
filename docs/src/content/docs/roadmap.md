---
title: Roadmap
description: Current gh-aw-fleet priorities for cost visibility, security, docs, and operator quality of life.
---

`gh-aw-fleet` is pre-1.0. The roadmap is biased toward operator ergonomics,
cost visibility, and supply-chain safety while preserving the thin-orchestrator
boundary: delegate to `gh aw`, `gh`, and `git` instead of re-implementing them.

See the full [ROADMAP.md](https://github.com/rshade/gh-aw-fleet/blob/main/ROADMAP.md)
for the live issue list and status notes.

## Release composition

Each feature-bearing release should include at least one cost-visibility item and
at least one security item. Cost matters because usage-based Copilot billing made
AI-credit spend a continuous operator concern. Security matters because fleet is
the layer that knows every workflow pin across every managed repo.

Single-fix hotfix releases are exempt.

## Recently shipped (v0.2.5)

The v0.2.5 release paired cost and security items:

- Read-only over-budget highlighting in the consumption rollup (`--budget`).
- Promotion of high-severity Layer 1 security findings from advisory to blocking
  under an opt-in `--strict` gate on `deploy`, `sync`, and `upgrade`.
- Interactive findings confirmation plus a `--yes` bypass for non-interactive use.
- A joined drift, run-health, no-op, and cost dashboard (`gh-aw-fleet overview`).

For what's next, see the live issue list in the full `ROADMAP.md` linked above.

## Near-term FinOps

Planned cost-visibility work includes:

- `gh-aw-fleet forecast`: aggregate projected AIC before spend happens.
- Pagination for Actions workflow discovery in the logs consumption source.
- Better cap-hit diagnostic hints when max AI credits or max turns are exceeded.

## Security

Security work continues through the scanner pipeline (the `--strict` blocking
gate for HIGH findings shipped in v0.2.5; see above):

- Layer 1 scanner coverage for secrets, compiled workflow hazards, and
  fleet-structural rules.
- Future deeper scans for prompt-injection signatures and risky trigger patterns.

## Documentation and operator quality of life

The roadmap also tracks install friction, dry-run examples, troubleshooting
hints, and packaging as a `gh` extension. Documentation work should stay close to
real operator workflows: installation, configuration, reconcile, troubleshooting,
and cost review.
