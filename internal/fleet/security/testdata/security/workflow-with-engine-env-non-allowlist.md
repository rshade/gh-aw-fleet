---
on:
  workflow_dispatch:
permissions:
  contents: read
engine:
  id: claude
  env:
    MY_SECRET: ${{ secrets.MY_SECRET }}
---

# Engine env non-allowlist

Body.
