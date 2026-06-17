---
on:
  push:
    branches: [main]
  skip-if-match:
    - "skip-ci"
engine: claude
description: "Analyse new commits and update docs."
permissions:
  contents: read
---

# Push analyser with guard

Examine each new commit to main and update the documentation index.
Only runs when the commit message does not contain skip-ci.
