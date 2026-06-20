# Quickstart: Astro Starlight Documentation Site

**Feature**: `014-starlight-docs-site` | **Date**: 2026-06-20

Two audiences: (A) anyone running/building this site locally, and (B) a downstream
sibling maintainer reproducing the wiring (the FR-010 "how this is wired" note,
which satisfies SC-005).

---

## A. Run the docs site locally

Prerequisites: Node.js (CI uses 24), npm, and the `docs/theme` submodule checked
out.

```bash
# 1. Get the shared theme (required — the build fails fast without it)
git submodule update --init --recursive

# 2. Install + run, from the docs project root
cd docs
npm ci
npm run dev        # local dev server with hot reload

# 3. Production build (what CI runs) — base-aware, link-validated
npm run build      # runs the prebuild submodule guard, then `astro build`
npm run preview    # serve docs/dist exactly as published (under /gh-aw-fleet)
```

What the build enforces for you:
- **`prebuild` guard** aborts with a clear message if `docs/theme/tokens.css` is
  missing (forgotten submodule init).
- **`starlight-links-validator`** fails the build on any broken internal link
  (this is your SC-006 gate — fix links until the build is green).

---

## B. Reproduce this wiring in a sibling project (FR-010 / SC-005)

Copy these artifacts and change only the project-specific values
(`<project>` = your repo name):

1. **Add the theme submodule** at `docs/theme`:
   ```bash
   git submodule add https://github.com/rshade/rshade-theme docs/theme
   ```
   This writes `.gitmodules`. Pin it by commit (no tracking branch).

2. **Copy `docs/package.json`** (and run `npm install` to regenerate the lockfile).
   Keep the pinned versions: `astro ^6.4.5`, `@astrojs/starlight ^0.40.0`,
   `@astrojs/sitemap ^3.7.3`, `starlight-links-validator ^0.24.1`, `sharp ^0.35.0`.
   Keep the `prebuild` submodule guard verbatim.

3. **Copy `docs/astro.config.mjs`**, changing only:
   - `site` (your Pages origin, e.g. `https://rshade.github.io`)
   - `base` → `/<project>`
   - `title`, `social.href`, and the `sidebar` entries.

4. **Copy the theme bridge** `docs/src/styles/theme-bridge.css` **verbatim** — and
   make sure it sets the accent under **both** `:root` and
   `:root[data-theme="light"]`:
   ```css
   @import "../../theme/tokens.css";
   :root { --sl-font: var(--font-sans); --sl-font-mono: var(--font-mono); }
   :root, :root[data-theme="light"] {
     --sl-color-accent-low: var(--color-accent-soft);
     --sl-color-accent: var(--color-accent);
     --sl-color-accent-high: var(--color-accent-hover);
   }
   ```
   > **The one pitfall this reference exists to prevent**: if you map the accent
   > only under `:root`, light mode silently reverts to Starlight's default purple.
   > Always target both selectors. (See `contracts/theme-bridge.md`, invariant B1.)
   >
   > **Second pitfall**: a wrong `@import` relative path fails *silently* — count
   > the `../` hops from your bridge file to `theme/tokens.css` and confirm branded
   > fonts/accent actually render.

5. **Copy `docs/public/`**: the `favicon.svg`, plus `robots.txt` and `llms.txt`
   with the `<project>` base spelled out in their absolute URLs (these static files
   are NOT rewritten by Astro's `base`).

6. **Copy `.github/workflows/docs.yml`**, changing nothing structural — keep
   `submodules: recursive`, the `paths: ['docs/**', ...]` scope, `npm ci && npm run
   build` from `docs/`, and `path: docs/dist`. Enable Pages-from-Actions on the
   repo once.

7. **Copy the hygiene files**: add `node_modules/` + `docs/.astro/` to root
   `.gitignore`; copy `docs/.gitignore` and `.markdownlintignore`
   (excludes `docs/theme`, `docs/dist`, `docs/.astro`, `docs/node_modules`).

That set is sufficient to stand up a brand-consistent site with no further design
decisions — which is exactly what SC-005 measures.

---

## C. Verify before you ship

| Check | How | Maps to |
|---|---|---|
| Every top-level page returns 200 | `curl` loop in `contracts/published-routes.md` | SC-001 |
| Zero internal 404s | green `npm run build` (links-validator) + post-deploy crawl | SC-006 |
| Install/config/reconcile ≤2 clicks from home | manual nav | SC-002 |
| Accent correct in light AND dark | toggle color mode beside blog + sibling site | SC-003 |
| Docs change deploys without a release; release doesn't deploy docs | push one of each kind, watch Actions | SC-004 |
| Migrated `finops.md` content reachable | open `/gh-aw-fleet/consumption/` | SC-007 |
