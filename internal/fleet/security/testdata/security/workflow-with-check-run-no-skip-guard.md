---
on:
  check_run:
    types: [completed]
engine: claude
description: "React to CI check completions."
permissions:
  contents: read
---

# Check run handler

Examine the completed check run and file a report.
