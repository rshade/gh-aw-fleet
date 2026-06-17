---
on:
  pull_request:
    types: [opened, synchronize]
engine: claude
description: "Review pull request changes."
permissions:
  contents: read
  pull-requests: read
---

# PR reviewer

Examine the pull request diff and summarise potential issues.
