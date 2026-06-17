---
on:
  push:
    branches: [main]
engine: claude
description: "Analyse new commits and update docs."
permissions:
  contents: read
---

# Push analyser

Examine each new commit to main and update the documentation index.
