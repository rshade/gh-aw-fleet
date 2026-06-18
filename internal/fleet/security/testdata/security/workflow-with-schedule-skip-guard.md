---
on:
  schedule:
    - cron: "0 6 * * *"
  skip-if-match:
    - "is:pr is:open in:title \"[daily-scan]\""
permissions:
  contents: read
engine: claude
---

# Scheduled workflow with guard

Body content.
