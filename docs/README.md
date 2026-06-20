# gh-aw-fleet Docs Site Wiring

This directory is a self-contained Astro Starlight documentation site. It is the
reference wiring for sibling projects that need the same branded docs setup.

## Integration file set

Copy this set when reproducing the site in another repository:

- `docs/package.json`
- `docs/package-lock.json`
- `docs/astro.config.mjs`
- `docs/tsconfig.json`
- `docs/src/content.config.ts`
- `docs/src/styles/theme-bridge.css`
- `docs/public/favicon.svg`
- `docs/public/robots.txt`
- `docs/public/llms.txt`
- `.github/workflows/docs.yml`
- `.gitmodules`
- root `.gitignore` additions
- root `.markdownlintignore`

## Add the shared theme

Add the shared theme as a submodule at `docs/theme`:

```bash
git submodule add https://github.com/rshade/rshade-theme docs/theme
```

Pin it by commit. Do not add a tracking branch.

Local builds and CI both expect `docs/theme/tokens.css` to exist. The
`prebuild` script fails fast with a `git submodule update --init --recursive`
hint when it is missing. The Pages workflow uses `submodules: recursive` so the
deployed output has access to the same tokens.

## Project-specific values

Change only these values per project:

- `base` in `docs/astro.config.mjs`
- `site` in `docs/astro.config.mjs`
- Starlight `title` in `docs/astro.config.mjs`
- `social.href` in `docs/astro.config.mjs`
- Sidebar labels/slugs in `docs/astro.config.mjs`
- Absolute URLs in `docs/public/robots.txt`
- Absolute URLs in `docs/public/llms.txt`
- Content under `docs/src/content/docs/`

For `gh-aw-fleet`, the Pages origin is `https://rshade.github.io` and the
project base is `/gh-aw-fleet`.

## Theme bridge pitfalls

The bridge file must import the theme tokens with the exact relative path from
`docs/src/styles/theme-bridge.css`:

```css
@import "../../theme/tokens.css";
```

A wrong `@import` path can fail silently and leave the site unbranded. Confirm
the rendered site resolves `--sl-font`, `--sl-font-mono`, and the accent
variables to the shared theme tokens.

The accent ramp must target both selectors:

```css
:root,
:root[data-theme="light"] {
  --sl-color-accent-low: var(--color-accent-soft);
  --sl-color-accent: var(--color-accent);
  --sl-color-accent-high: var(--color-accent-hover);
}
```

Using only `:root` themes dark mode but lets light mode fall back to Starlight's
default accent because `:root[data-theme="light"]` has higher specificity.

## Pipeline separation

`.github/workflows/docs.yml` is separate from the Go release pipeline. It runs
on pushes to `main` only when these paths change:

- `docs/**`
- `.gitmodules`
- `.github/workflows/docs.yml`

The Go release workflow, `.github/workflows/release.yml`, fires only on
`release: created` or manual dispatch. A docs-only push cannot trigger a release
because it does not create a GitHub release event.

The Go CI workflow, `.github/workflows/ci.yml`, is intentionally not path-scoped.
It may run on docs-only pushes, but it performs Go vet/fmt/test/lint only. It
does not build or deploy docs and it is not the software release pipeline.

## Local commands

```bash
git submodule update --init --recursive
cd docs
npm ci
npm run dev
npm run build
npm run preview
```

`npm run build` is the local gate for this docs project. It runs the submodule
guard first, then `astro build`, then `starlight-links-validator`.
