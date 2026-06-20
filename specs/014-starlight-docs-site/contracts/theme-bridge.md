# Contract: Theme Bridge

**Feature**: `014-starlight-docs-site`

The bridge is the artifact downstream siblings copy (FR-010) and the locus of the
FR-005 light/dark accent correctness guarantee. This contract fixes exactly which
Starlight variables MUST be mapped, and under which selectors.

## File

`docs/src/styles/theme-bridge.css`, referenced as the sole `customCss` entry in
`astro.config.mjs`:

```js
customCss: ['./src/styles/theme-bridge.css'],
```

## Required mapping

```css
@import "../../theme/tokens.css";

/* fonts — theme-independent in Starlight */
:root {
  --sl-font: var(--font-sans);
  --sl-font-mono: var(--font-mono);
}

/* accent ramp — MUST target BOTH selectors */
:root,
:root[data-theme="light"] {
  --sl-color-accent-low: var(--color-accent-soft);
  --sl-color-accent: var(--color-accent);
  --sl-color-accent-high: var(--color-accent-hover);
}
```

## Variable contract

| Starlight var | Source token | Selector scope |
|---|---|---|
| `--sl-font` | `--font-sans` | `:root` |
| `--sl-font-mono` | `--font-mono` | `:root` |
| `--sl-color-accent-low` | `--color-accent-soft` | `:root, :root[data-theme="light"]` |
| `--sl-color-accent` | `--color-accent` | `:root, :root[data-theme="light"]` |
| `--sl-color-accent-high` | `--color-accent-hover` | `:root, :root[data-theme="light"]` |

## Invariants

- **B1 (both schemes, FR-005 — the defect this feature fixes)**: The accent ramp
  MUST be declared under both `:root` and `:root[data-theme="light"]`. Declaring
  only `:root` lets light mode revert to Starlight's default purple accent because
  `:root[data-theme="light"]` outranks `:root` on specificity. **This is the exact
  bug present in ax-go's checked-in bridge; do NOT copy that file — use the mapping
  above.**
- **B2 (low-accent included)**: `--sl-color-accent-low` MUST be mapped (ax-go omits
  it); it controls low-emphasis accent tints.
- **B3 (import path)**: The `@import "../../theme/tokens.css"` relative depth MUST
  be exact. A wrong path fails **silently** (no build error) and yields an
  un-branded site — verify by confirming branded fonts/accent actually render.
- **B4 (no vendored tokens, FR-006)**: The bridge MUST reference theme tokens via
  the submodule import; it MUST NOT inline literal token values.

## Verification (maps to SC-003)

Load the published site beside the blog and a sibling site; confirm sans + mono
typefaces and the accent color match. Toggle light and dark mode; confirm the
accent stays the shared brand color (`oklch(0.65 0.18 250)` family) in **both**,
never Starlight's default. Capture objective evidence by inspecting computed
values for `--sl-font`, `--sl-font-mono`, `--sl-color-accent-low`,
`--sl-color-accent`, and `--sl-color-accent-high` in both color modes.
