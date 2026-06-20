# Phase 0 Research: Astro Starlight Documentation Site

**Feature**: `014-starlight-docs-site` | **Date**: 2026-06-20

This effort is a *reference implementation* whose wiring is prescribed by issue
#138 and proven in the sibling repo `../ax-go/`. The primary research activity
was a verbatim extraction of the ax-go documentation site (a dispatched research
agent read every relevant file). Each decision below records what ax-go does,
what we copy, and where we deliberately diverge.

There were **no open `NEEDS CLARIFICATION` items** in the Technical Context — the
toolchain is a fixed input (spec Assumptions). Research therefore resolves *exact
versions and wiring*, not technology choices.

---

## Decision 1 — Framework & pinned versions

**Decision**: Astro Starlight, with `docs/package.json` dependencies pinned to the
exact set `../ax-go/docs/package.json` uses:

| Package | Range | ax-go lockfile resolves |
|---|---|---|
| `astro` | `^6.4.5` | `6.4.8` |
| `@astrojs/starlight` | `^0.40.0` | `0.40.0` |
| `@astrojs/sitemap` | `^3.7.3` | `3.7.3` |
| `starlight-links-validator` | `^0.24.1` | `0.24.1` |
| `sharp` | `^0.35.0` | `0.35.2` |

`name: "gh-aw-fleet-docs"`, `type: "module"`, `private: true`, no `engines` field
(Node version pinned only in CI). No `devDependencies` block — everything sits in
`dependencies`, matching ax-go.

**Rationale**: The spec fixes the framework; matching ax-go's pins means the four
sibling sites share one known-good dependency set, so a bug fixed once (e.g. a
Starlight minor) propagates by copy rather than per-site re-discovery (US3).

**Alternatives considered**: Newer Starlight/Astro majors — rejected to stay
lockstep with the established reference; drifting versions defeats the
"reproduce verbatim" goal. A `devDependencies` split — rejected; ax-go keeps a
flat list and the site has no runtime/build dependency distinction worth modeling.

---

## Decision 2 — Theme consumption: `rshade-theme` as a git submodule

**Decision**: Add `rshade-theme` as a git submodule at `docs/theme`, declared in a
new root `.gitmodules`:

```ini
[submodule "docs/theme"]
	path = docs/theme
	url = https://github.com/rshade/rshade-theme
```

Pin by commit (no `branch =` line), exactly like ax-go. The submodule provides
`tokens.css` — the single source of truth for `--font-sans`, `--font-mono`, and
the accent ramp (`--color-accent`, `--color-accent-hover`, `--color-accent-soft`).

**Rationale**: FR-006 requires the brand styling be consumed as a
centrally-updatable, versioned dependency rather than copied token values. A
submodule satisfies this and is the mechanism the spec prescribes. Pinning by
commit (not a moving branch) makes theme upgrades explicit and reviewable.

**Alternatives considered**: npm-publishing `rshade-theme` and depending on it via
`package.json` — cleaner in theory, but the theme is not published to a registry
and ax-go already standardized on the submodule; diverging would break the
"copy verbatim" contract. Vendoring `tokens.css` into this repo — rejected by
FR-006 (it forbids copying token values).

---

## Decision 3 — Theme bridge CSS: use the CORRECTED both-scheme snippet (the core defect fix)

**Decision**: `docs/src/styles/theme-bridge.css` maps the shared tokens onto
Starlight's `--sl-*` variables, setting the accent ramp under **both** `:root` and
`:root[data-theme="light"]`, and mapping the low-emphasis accent:

```css
@import "../../theme/tokens.css";

/* fonts are theme-independent in Starlight */
:root {
  --sl-font: var(--font-sans);
  --sl-font-mono: var(--font-mono);
}

/* accent ramp MUST target both selectors, or light mode keeps Starlight's
   default — :root[data-theme="light"] outranks :root on specificity */
:root,
:root[data-theme="light"] {
  --sl-color-accent-low: var(--color-accent-soft);
  --sl-color-accent: var(--color-accent);
  --sl-color-accent-high: var(--color-accent-hover);
}
```

Referenced as the single `customCss: ['./src/styles/theme-bridge.css']` entry in
`astro.config.mjs`.

**Rationale — this is the reason the feature exists (FR-005, US2)**: ax-go's
*checked-in* `theme-bridge.css` sets the accent only under bare `:root` and omits
`--sl-color-accent-low`. Because Starlight declares its accent under the
higher-specificity `:root[data-theme="light"]` selector too, mapping `:root`
alone themes **dark** mode correctly while **light** mode silently falls back to
Starlight's stock purple accent. The spec's Assumptions make the issue-#138
reviewed snippet (both-theme targeting, including the low-accent mapping)
**authoritative over** any single-scheme version. We ship the corrected bridge so
the three downstream siblings inherit a working pattern, not the bug.

**Gotcha captured for quickstart**: a missing CSS `@import` fails *silently* (no
build error) — the `../../` relative depth from the bridge file to `tokens.css`
must be counted exactly. The `prebuild` guard (Decision 8) backstops the
submodule-missing case but not a wrong relative path, so quickstart calls this out.

**Alternatives considered**: Copying ax-go's live `theme-bridge.css` verbatim —
rejected; it reproduces the exact defect this effort is chartered to fix.

---

## Decision 4 — Routing: GitHub Pages project subpath

**Decision**: `astro.config.mjs` sets `site: 'https://rshade.github.io'` and
`base: '/gh-aw-fleet'`. Publishing target is GitHub Pages from Actions, serving at
`https://rshade.github.io/gh-aw-fleet/`.

**Rationale**: Project Pages sites are served from a subpath; FR-003/SC-006 require
every internal link and asset to resolve under `/gh-aw-fleet`. Astro bakes `base`
into all generated links, sitemap URLs, and asset references when set correctly.

**Validation mechanism**: `starlight-links-validator` runs at build and fails on
broken internal links, giving SC-006 ("zero internal links 404") a build-time
gate rather than a post-deploy check.

**Alternatives considered**: A custom domain (domain-root serving) — explicitly Out
of Scope. Hardcoding absolute `/gh-aw-fleet/...` links in content — rejected;
brittle and defeats Astro's `base` handling.

---

## Decision 5 — Static assets: favicon, plus NEW `robots.txt` and `llms.txt`

**Decision**: Ship `docs/public/favicon.svg` (brand glyph, light/dark via
`prefers-color-scheme`, auto-discovered by Starlight — copied from ax-go).
**Additionally** ship two files ax-go does **not** have:

- `docs/public/robots.txt` — allows all crawlers and points at the base-aware
  sitemap: `Sitemap: https://rshade.github.io/gh-aw-fleet/sitemap-index.xml`.
- `docs/public/llms.txt` — an agent-discoverability index (per the emerging
  `llms.txt` convention) linking the top-level pages with the full
  `https://rshade.github.io/gh-aw-fleet/...` URLs.

**Rationale**: The feature request explicitly listed `robots.txt` and `llms.txt`.
Research found ax-go ships **neither** — its `docs/public/` contains only
`favicon.svg`, relying solely on the auto-generated sitemap. Rather than copy that
gap, we add both files (base-path-aware) so this reference *improves* on ax-go and
the siblings inherit SEO/agent-discoverability wiring. Files in `docs/public/` are
copied to the site root verbatim, so the `Sitemap:`/`llms.txt` URLs must be
written with the `/gh-aw-fleet` base spelled out (Astro does not rewrite static
`public/` file contents).

**Sitemap**: generated by `@astrojs/sitemap`; with `site` + `base` set, the index
is served at `https://rshade.github.io/gh-aw-fleet/sitemap-index.xml` with the base
path baked into every entry. Mirror ax-go's `filter` only if a path needs
excluding (none at launch).

**Alternatives considered**: Copy ax-go verbatim (no robots/llms) — rejected
against the explicit request. Generating `robots.txt` dynamically via an
integration — over-engineered for a static, rarely-changing file.

---

## Decision 6 — Content model & seed pages

**Decision**: Use the Astro content-layer config (`docs/src/content.config.ts` with
`docsLoader()` + `docsSchema()` — the modern `src/content.config.ts` location, not
the legacy `src/content/config.ts`). Author a `template: splash` landing
(`index.mdx`, hero + `CardGrid`) plus five reference pages, sourced from committed
material:

| Page (slug) | FR | Seed source |
|---|---|---|
| `index.mdx` (splash) | FR-001 | hero/value-prop from README intro |
| `install` | FR-002(a) | README "Install" section (one-liner installer, `go install`, fallback) |
| `configuration` | FR-002(b) | `fleet.example.json` + `fleet.json` field semantics (hand-authored reference) |
| `reconcile` | FR-002(c) | `AGENTS.md`/`CLAUDE.md` deploy/sync/upgrade + three-turn pattern |
| `consumption` | FR-002(d) | migrated `docs/finops.md` |
| `roadmap` | FR-002(e) | `ROADMAP.md` |

Sidebar groups them so install, configuration, and reconcile are each reachable
from the landing page within two clicks (SC-002).

**Rationale**: FR-002 enumerates exactly these five content areas. The
configuration reference is hand-authored from the committed example config (spec
Assumptions: not an auto-generated schema dump — no generator exists).

**Alternatives considered**: Auto-generating the config reference from a JSON
Schema — rejected; no schema generator exists and the spec assumes hand-authoring.

---

## Decision 7 — Migrating the existing `docs/finops.md` (FR-011 / SC-007)

**Decision**: Move `docs/finops.md` into `docs/src/content/docs/consumption.md`
(adding Starlight frontmatter: `title`, `description`), then remove the original
`docs/finops.md`. **Rewrite its relative links** — the file currently references
`../AGENTS.md` and `../skills/fleet-budget-review/SKILL.md`, which do not resolve
from the published site and would fail `starlight-links-validator`. Repoint them
to GitHub `blob/main` URLs (or inline the relevant content).

**Rationale**: The existing raw doc sits at `docs/finops.md` — exactly the path
that becomes the Astro project root — so it must be carried into the content tree
rather than orphaned (FR-011, SC-007, spec edge case "Existing raw documentation
file"). The link rewrite is mandatory: `starlight-links-validator` will fail the
build on the existing `../AGENTS.md`-style links once they live inside the site.

**Alternatives considered**: Leaving `docs/finops.md` in place beside the Astro
project — rejected; it would be a stranded, unpublished file (violates SC-007) and
clutter the project root. Disabling the link validator to tolerate the broken
links — rejected; it would forfeit the SC-006 build-time guarantee.

---

## Decision 8 — Build-time safety: `prebuild` submodule guard + `submodules: recursive`

**Decision**: Carry ax-go's `prebuild` npm script that aborts the build with a
clear message if `theme/tokens.css` is absent:

```json
"prebuild": "node -e \"const fs=require('fs');if(!fs.existsSync('theme/tokens.css')){console.error('ERROR: docs/theme submodule missing; run: git submodule update --init --recursive');process.exit(1)}\""
```

Pair it with `submodules: recursive` on the deploy workflow's checkout (Decision 9).

**Rationale**: FR-009 and the "shared theme not present at build time" edge case
demand the theme always be available, and demand a *loud* failure if it is not
(rather than a silently un-branded site). The guard catches a forgotten local
`git submodule update`; the `recursive` checkout ensures CI always has it.

**Alternatives considered**: Relying on CI checkout alone — rejected; local
`npm run build` would silently produce an un-branded site without the guard.

---

## Decision 9 — Pipeline separation (FR-007, US4)

**Decision**: Add `.github/workflows/docs.yml` mirroring ax-go: triggered on
`push` to `main` scoped with `paths: ['docs/**', '.github/workflows/docs.yml']`
plus `workflow_dispatch`; `actions/checkout@v7` with `submodules: recursive`;
`actions/setup-node@v4` (`node-version: 24`, npm cache on `docs/package-lock.json`);
`npm ci` + `npm run build` with `working-directory: docs`;
`actions/upload-pages-artifact@v3` (`path: docs/dist`); a separate `deploy` job
using `actions/deploy-pages@v5` with `environment: github-pages`;
`permissions: { contents: read, pages: write, id-token: write }`;
`concurrency: { group: pages, cancel-in-progress: false }`.

**Leave the Go pipelines unchanged**: `release.yml` triggers on `release: created`
+ `workflow_dispatch` only, so a docs push never fires a release. `ci.yml` runs Go
vet/fmt/test/lint on every PR/push and is *not* path-scoped — a docs-only push
will still run Go CI, but CI performs **no** docs build or deploy and is **not**
"the software release pipeline." This matches ax-go and honors the spec's Out of
Scope ("Changes to … the Go build" are excluded), so we do not add `paths-ignore`
to `ci.yml`.

**Rationale**: FR-007 / US4 require the docs and release pipelines not to
cross-trigger. The path-scoped `docs.yml` guarantees Go changes never deploy docs
(US4.2). `release.yml`'s `release`-only trigger guarantees a docs change never
triggers a software release (US4.1). A failing docs build cannot block a release
because they are different workflows on different triggers (edge case "Docs build
failure does not block releases").

**Note on `configure-pages`**: ax-go omits the explicit `actions/configure-pages`
step, relying on the `github-pages` environment + `id-token` permission. We mirror
that. Enabling Pages-from-Actions on the repo is the one-time setup that
accompanies landing this feature (spec Assumptions, "Pages enablement").

**Alternatives considered**: Path-scoping `ci.yml` with `paths-ignore: ['docs/**']`
— rejected; it edits the Go pipeline (Out of Scope) for no functional gain, since
CI never deploys docs. A single combined CI+docs workflow — rejected; it couples
the two tracks the spec requires to stay independent.

---

## Decision 10 — Repo hygiene (gitignore, markdownlint)

**Decision**:
- Root `.gitignore`: add `node_modules/` and `docs/.astro/` (the root already
  ignores `/dist/`); `docs/.gitignore` additionally ignores `dist/`, `.astro/`,
  `node_modules/`.
- Add a root `.markdownlintignore` excluding `docs/node_modules/`, `docs/theme/`,
  `docs/dist/`, and `docs/.astro/` so the vendored theme and build output are not
  linted against this repo's markdown rules.

**Rationale**: Build output and installed deps must not be committed; the vendored
`docs/theme/` submodule (Apache-2.0 README/CHANGELOG) should not be held to this
repo's markdownlint config. Mirrors ax-go's hygiene exactly.

**Alternatives considered**: No `.markdownlintignore` — rejected; markdownlint
would flag the submodule's and build output's markdown, producing noise unrelated
to this repo's content.

---

## Cross-cutting note: where we diverge from `../ax-go/`

The instruction was to mirror ax-go's styling/versions/themes/sitemaps/etc.
exactly. Two intentional divergences, both spec-mandated or spec-requested:

1. **Theme bridge** (Decision 3): we use the *corrected* both-scheme accent bridge,
   not ax-go's checked-in single-scheme file — required by FR-005 and authoritative
   per the spec's Assumptions.
2. **`robots.txt` + `llms.txt`** (Decision 5): we add both; ax-go ships neither.
   Requested explicitly in the feature input.

Everything else — Astro/Starlight/sitemap versions, the submodule mechanism, the
splash landing pattern, the deploy workflow shape, gitignore/markdownlint hygiene,
the `prebuild` guard — is copied to match ax-go.
