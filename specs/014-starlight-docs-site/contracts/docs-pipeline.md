# Contract: Documentation Pipeline

**Feature**: `014-starlight-docs-site`

The build/deploy automation contract: triggers, the submodule-at-build-time
guarantee, and the non-cross-trigger guarantee with the Go pipelines.

## File

`.github/workflows/docs.yml` (new). Mirrors `../ax-go/.github/workflows/docs.yml`.

## Trigger contract

```yaml
on:
  push:
    branches: [main]
    paths:
      - 'docs/**'
      - '.gitmodules'
      - '.github/workflows/docs.yml'
  workflow_dispatch:
```

| Invariant | Statement | Maps to |
|---|---|---|
| **P1** | A push touching only `docs/**` or `.gitmodules` runs this workflow and deploys. | US4.1, SC-004 |
| **P2** | A push touching only Go/CLI sources does **not** run this workflow (path scope excludes it). | US4.2, SC-004 |
| **P3** | A docs change does **not** trigger the Go release pipeline (`release.yml` fires on `release: created` only). | US4.1, SC-004, FR-007 |
| **P4** | A failing docs build does **not** gate a release (separate workflows/triggers). | edge case, FR-007 |

`ci.yml` (Go vet/fmt/test/lint) is **not** path-scoped and will still run on
docs-only pushes, but performs no docs build/deploy and is not the release
pipeline — left unchanged to honor the spec's Out of Scope (Decision 9).

## Build contract

```yaml
permissions: { contents: read, pages: write, id-token: write }
concurrency: { group: pages, cancel-in-progress: false }
```

| Step | Requirement | Maps to |
|---|---|---|
| checkout | `actions/checkout@v7` with `submodules: recursive` | **FR-009** (theme present at build) |
| node | `actions/setup-node@v4`, `node-version: 24`, npm cache on `docs/package-lock.json` | — |
| install | `npm ci` (`working-directory: docs`) | reproducible install from lockfile |
| build | `npm run build` (`working-directory: docs`) → runs `prebuild` guard, then `astro build` | FR-009 fail-fast; SC-006 link check |
| upload | `actions/upload-pages-artifact@v3`, `path: docs/dist` | — |
| deploy | separate job, `actions/deploy-pages@v5`, `environment: github-pages` | FR-012 (automated publish) |

## Invariants

- **P5 (theme at build time, FR-009)**: The build MUST have `docs/theme` checked
  out. `submodules: recursive` guarantees it in CI; the `prebuild` guard makes a
  missing submodule a loud failure rather than a silently un-branded site.
- **P6 (subpath build, FR-003)**: The build runs with `base: '/gh-aw-fleet'`, so
  the uploaded `docs/dist` has subpath-correct links.
- **P7 (automated publish, FR-012)**: A merged docs change reaches the live site
  with no manual build/upload step.
- **P8 (private-theme escape hatch)**: If `rshade-theme` ever becomes private, the
  default `GITHUB_TOKEN` cannot read it; the checkout step needs a cross-repo PAT
  (`token: ${{ secrets.THEME_REPO_TOKEN }}`). Carry ax-go's commented note so the
  fix is discoverable.

## One-time setup (spec Assumptions, "Pages enablement")

Enabling GitHub Pages-from-Actions on the repository is a one-time manual action
that accompanies landing this feature; the workflow relies on the `github-pages`
environment + `id-token` permission rather than an explicit `actions/configure-pages`
step (mirroring ax-go).
