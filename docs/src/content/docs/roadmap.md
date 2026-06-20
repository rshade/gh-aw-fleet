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

## Immediate focus

The current release focus pairs one cost item with one security item:

- Read-only over-budget highlighting in the consumption rollup (`--budget`).
- Promotion of high-severity Layer 1 security findings from advisory to blocking
  under an opt-in `--strict` flag.

## Near-term FinOps

Planned cost-visibility work includes:

- `gh-aw-fleet forecast`: aggregate projected AIC before spend happens.
- Pagination for Actions workflow discovery in the logs consumption source.
- Better cap-hit diagnostic hints when max AI credits or max turns are exceeded.

## Security

Security work continues through the scanner pipeline:

- Layer 1 scanner coverage for secrets, compiled workflow hazards, and
  fleet-structural rules.
- `--strict` promotion for high findings.
- Future deeper scans for prompt-injection signatures and risky trigger patterns.

## Documentation and operator quality of life

The roadmap also tracks install friction, dry-run examples, troubleshooting
hints, and packaging as a `gh` extension. Documentation work should stay close to
real operator workflows: installation, configuration, reconcile, troubleshooting,
and cost review.
