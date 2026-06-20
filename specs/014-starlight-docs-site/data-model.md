# Phase 1 Data Model: Astro Starlight Documentation Site

**Feature**: `014-starlight-docs-site` | **Date**: 2026-06-20

This is a static-site feature: the "entities" are content/config artifacts and
the relationships between them, not database records. Each entity below maps to
the spec's Key Entities and to concrete files. "Validation rules" are the
build-time checks (`starlight-links-validator`, the `prebuild` guard, Astro's
schema) that enforce correctness.

---

## Entity 1 — Documentation site

The published, navigable set of pages served at `https://rshade.github.io/gh-aw-fleet/`.

| Attribute | Value / source | Validation |
|---|---|---|
| `site` | `https://rshade.github.io` (`astro.config.mjs`) | Must be the Pages origin (no trailing path). |
| `base` | `/gh-aw-fleet` (`astro.config.mjs`) | Must equal the repo name; every emitted URL carries it (FR-003). |
| `title` | `gh-aw-fleet` (Starlight `title`) | Brand name shown in header. |
| build output | `docs/dist/` | Gitignored; uploaded as the Pages artifact. |
| search | Starlight default (Pagefind) | Used as shipped (spec Assumptions). |

**State**: built by `astro build` → published by the deploy job. No runtime state.

**Relationships**: composed of *Content pages* (Entity 2), styled by the *Theme
bridge* (Entity 4) which pulls from the *Shared theme* (Entity 3), published by the
*Documentation pipeline* (Entity 5).

---

## Entity 2 — Content pages

Individual documentation pages under `docs/src/content/docs/`, registered through
the content collection (`docs/src/content.config.ts`: `docsLoader()` + `docsSchema()`).

| Slug / file | Template | FR | Seed source | Required frontmatter |
|---|---|---|---|---|
| `index.mdx` | `splash` | FR-001 | README intro | `title`, `description`, `template: splash`, `hero` |
| `install` (`install.md`) | doc | FR-002(a) | README "Install" | `title`, `description` |
| `configuration` (`configuration.md`) | doc | FR-002(b) | `fleet.example.json` + `fleet.json` | `title`, `description` |
| `reconcile` (`reconcile.md`) | doc | FR-002(c) | `AGENTS.md` deploy/sync/upgrade | `title`, `description` |
| `consumption` (`consumption.md`) | doc | FR-002(d) | migrated `docs/finops.md` | `title`, `description` |
| `roadmap` (`roadmap.md`) | doc | FR-002(e) | `ROADMAP.md` | `title`, `description` |

**Validation rules**:
- Each file's frontmatter MUST satisfy `docsSchema()` (Astro fails the build
  otherwise). At minimum `title`; `description` recommended for SEO/`llms.txt`.
- All in-page and cross-page links MUST resolve under `/gh-aw-fleet`
  (`starlight-links-validator` fails the build on broken internal links → SC-006).
- The migrated `consumption.md` MUST have its former relative links
  (`../AGENTS.md`, `../skills/...`) rewritten to absolute GitHub URLs or inlined
  (Decision 7) — otherwise the validator fails.

**State transition**: `docs/finops.md` (raw, pre-existing) → moved + frontmatter
added + links rewritten → `docs/src/content/docs/consumption.md`; original
removed (FR-011 / SC-007).

---

## Entity 3 — Shared theme (`rshade-theme`)

The externally-maintained source of brand design tokens, consumed as a git
submodule at `docs/theme` (declared in `.gitmodules`). Single source of truth for
cross-site visual consistency (FR-006).

| Token (in `theme/tokens.css`) | Value (current pin) | Consumed as |
|---|---|---|
| `--font-sans` | `"Inter", ui-sans-serif, system-ui, …` | `--sl-font` |
| `--font-mono` | `"JetBrains Mono", ui-monospace, …` | `--sl-font-mono` |
| `--color-accent` | `oklch(0.65 0.18 250)` | `--sl-color-accent` |
| `--color-accent-hover` | `oklch(0.72 0.18 250)` | `--sl-color-accent-high` |
| `--color-accent-soft` | `oklch(0.65 0.18 250 / 0.12)` | `--sl-color-accent-low` |

**Validation rules**:
- `theme/tokens.css` MUST exist at build time — enforced by the `prebuild` guard
  (fails with a `git submodule update --init --recursive` hint) and by
  `submodules: recursive` checkout in CI (FR-009, Decision 8).
- Pinned by commit (no tracking branch), so theme upgrades are explicit
  submodule-pointer bumps under `docs/**`.

**Relationships**: imported by the *Theme bridge* (Entity 4) via
`@import "../../theme/tokens.css"`. Fonts are **not** vendored by the theme;
consumers fall back to the system stacks unless they add font loading.

---

## Entity 4 — Theme bridge

The small, documented CSS mapping connecting the shared theme's tokens to
Starlight's `--sl-*` custom properties, covering **both** light and dark modes.
This is the artifact downstream sites copy (FR-010), and the locus of the FR-005
correctness fix.

| Attribute | Value |
|---|---|
| file | `docs/src/styles/theme-bridge.css` |
| referenced by | `astro.config.mjs` → `customCss: ['./src/styles/theme-bridge.css']` |
| font mapping | under `:root` (theme-independent) |
| accent mapping | under **`:root, :root[data-theme="light"]`** — both selectors |
| low/accent/high | `--sl-color-accent-low/accent/high` ← `--color-accent-soft/accent/hover` |

**Validation rules (the core invariant)**:
- The accent ramp MUST be declared under both `:root` and
  `:root[data-theme="light"]`; declaring only `:root` lets light mode revert to
  Starlight's default accent (FR-005, US2). This is *not* caught by any
  automated check — it is verified by visual inspection in both color modes
  (SC-003) and guarded by code review against the corrected snippet.
- The `@import "../../theme/tokens.css"` relative depth MUST be exact; a wrong
  path fails **silently** (no build error), so the import is verified by
  confirming branded fonts/accent actually render.

**Relationships**: imports Entity 3; styles Entity 1. The single source of the
two intentional divergences from ax-go is *not* here — ax-go's bridge is the buggy
version; ours is the corrected one (Decision 3).

---

## Entity 5 — Documentation pipeline

The automation (`.github/workflows/docs.yml`) that builds and publishes the site
on its own trigger and path scope, independent of the Go release pipeline, making
the shared theme available at build time.

| Attribute | Value |
|---|---|
| trigger | `push` to `main` with `paths: ['docs/**', '.github/workflows/docs.yml']`; `workflow_dispatch` |
| checkout | `actions/checkout@v7`, `submodules: recursive` (FR-009) |
| node | `actions/setup-node@v4`, `node-version: 24`, npm cache on `docs/package-lock.json` |
| build | `npm ci` then `npm run build`, `working-directory: docs` (runs `prebuild` guard first) |
| artifact | `actions/upload-pages-artifact@v3`, `path: docs/dist` |
| deploy | separate job, `actions/deploy-pages@v5`, `environment: github-pages` |
| permissions | `contents: read`, `pages: write`, `id-token: write` |
| concurrency | `group: pages`, `cancel-in-progress: false` |

**Validation rules / invariants**:
- MUST NOT cross-trigger the Go release pipeline; `release.yml` (`release: created`)
  and `ci.yml` (unscoped Go CI) are left unchanged (FR-007, US4, Decision 9).
- A failing docs build MUST NOT gate a release (separate workflows/triggers).
- The build MUST target `base: '/gh-aw-fleet'` so the published artifact has
  subpath-correct links (FR-003).

**Relationships**: consumes Entity 2 + Entity 3 (via the submodule checkout),
emits Entity 1.

---

## Entity 6 — Discoverability assets (new vs ax-go)

Static files served from the site root, written with the `/gh-aw-fleet` base
spelled out (Astro does not rewrite static `public/` file contents).

| File | Purpose | Key content |
|---|---|---|
| `docs/public/favicon.svg` | brand favicon | light/dark via `prefers-color-scheme` (copied from ax-go) |
| `docs/public/robots.txt` | crawler policy | `Sitemap: https://rshade.github.io/gh-aw-fleet/sitemap-index.xml` |
| `docs/public/llms.txt` | agent index | base-absolute links to the top-level pages |
| `sitemap-index.xml` (generated) | sitemap | emitted by `@astrojs/sitemap`, base baked in |

**Validation rules**: the `Sitemap:` URL in `robots.txt` and the links in
`llms.txt` MUST include the `/gh-aw-fleet` base (they are static, not rewritten by
Astro). The generated sitemap is served at
`https://rshade.github.io/gh-aw-fleet/sitemap-index.xml`.
