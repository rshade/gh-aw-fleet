---
title: Gate CI on fleet drift
description: Fail a CI job when any repository has drifted from its declared fleet state.
---

`gh-aw-fleet status` and `gh-aw-fleet overview` exit non-zero when a repository
has drifted from `fleet.json`. That makes either one a drop-in CI gate: run it as
a step and let the exit code fail the job.

## Use `status` for a fast, clone-free gate

`status` diffs desired versus actual workflow refs over the API without cloning
anything, so it is the cheapest gate:

```bash
gh-aw-fleet status
```

- Exit `0` — every tracked repo is aligned.
- Exit `1` — at least one repo has drifted (or errored).

Drop it straight into a workflow step; a non-zero exit fails the job:

```yaml
- name: Fail on fleet drift
  run: gh-aw-fleet status
```

## Inspect drift in JSON

To report *which* repos drifted rather than only failing, read the JSON envelope:

```bash
gh-aw-fleet status -o json \
  | jq '.result.repos | map(select(.drift_state == "drifted")) | length'
```

Scope the check to a single repo by passing it as an argument (`status` accepts
at most one; omit it to check the whole fleet):

```bash
gh-aw-fleet status you/your-repo
```

## Use `overview` when you also want health and cost

`overview` shares the same drift-only exit contract but additionally reports run
health, no-op rate, and AI-credit spend. Its window defaults to the trailing 7
days:

```bash
gh-aw-fleet overview
```

- Exit `0` — every in-scope repo is aligned.
- Exit `1` — any in-scope repo is drifted or errored.

Run failures stay advisory: a fully aligned fleet with failing agentic runs still
exits `0`, so `overview` gates on drift, not on flaky runs. It reuses the
`gh aw logs` fan-out, so it is slower than `status` and can take minutes on large
fleets — prefer `status` when you only need the drift gate.

## See also

- [CLI reference](/gh-aw-fleet/cli/) — full exit-code table for `status` and `overview`.
- [Fleet Overview](/gh-aw-fleet/overview/) — column reference for the dashboard.
