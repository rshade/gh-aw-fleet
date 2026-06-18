---
on:
  pull_request:
    types: [opened, synchronize]
  skip-if-no-match:
    - "*.go"
    - "*.ts"
engine: claude
description: "Review pull request changes."
permissions:
  contents: read
  pull-requests: read
---

# PR reviewer with guard

Examine the pull request diff and summarise potential issues.
Only runs when Go or TypeScript files changed.
