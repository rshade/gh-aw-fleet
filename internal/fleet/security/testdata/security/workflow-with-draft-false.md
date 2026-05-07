---
on:
  workflow_dispatch:
permissions:
  contents: read
engine: claude
safe-outputs:
  create-pull-request:
    title-prefix: "[bot] "
    draft: false
    protected-files:
      - ".github/**"
---

# Draft false workflow

Body.
