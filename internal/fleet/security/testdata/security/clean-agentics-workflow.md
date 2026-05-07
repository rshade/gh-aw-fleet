---
on:
  workflow_dispatch:
permissions:
  contents: read
  issues: read
engine: claude
description: "Diagnose CI failures and report root causes."
safe-outputs:
  create-issue:
    title-prefix: "[ci-doctor] "
---

# CI Doctor

Investigate the most recent failed workflow run on this repository.

1. List recent workflow runs and pick the most recent failure.
2. Inspect the failed job's logs.
3. Summarize the failure mode in a new GitHub issue.

Be terse and factual. Cite log lines when describing a failure.
