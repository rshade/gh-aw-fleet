---
on:
  pull_request_target:
    types: [opened, synchronize]
engine: claude
description: "Review pull request changes from forks."
permissions:
  contents: read
  pull-requests: read
---

# Fork PR reviewer

Examine the pull request diff from a fork and summarise potential issues.
