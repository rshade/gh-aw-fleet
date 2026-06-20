---
description: "Task list for Astro Starlight Documentation Site (014-starlight-docs-site)"
---

# Tasks: Astro Starlight Documentation Site (Reference Implementation)

**Input**: Design documents from `/specs/014-starlight-docs-site/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: No automated test suite is requested. This is a static docs site; the
"tests" are build-time gates (`starlight-links-validator`, the `prebuild`
submodule guard) and the manual Success-Criteria verifications (SC-001…SC-007).
No separate TDD test tasks are generated.

**Organization**: Tasks are grouped by user story (US1–US4 from spec.md) so each
story is an independently shippable increment. Note US2 and US4 are both P2.

## Format: `[ID] [P?] [MANUAL?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task)
- **[MANUAL]**: Operator-only acceptance check after push/merge. Do not execute
  during `/speckit-implement`; stop and hand the listed evidence collection to the
  maintainer.
- **[Story]**: US1 / US2 / US3 / US4 (Setup, Foundational, Polish have no story label)
- Implementation tasks name an exact file path. Manual validation tasks name the
  command/evidence the operator must collect.

## Path Conventions

This feature adds a self-contained Node/Astro project rooted at `docs/` plus one
workflow and three repo-root hygiene files. No Go files change. All paths are
relative to the repo root `/mnt/c/GitHub/go/src/github.com/rshade/gh-aw-fleet/`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Scaffold a buildable Astro project skeleton (default theme, no
branding yet) and the repo hygiene files. Versions match `../ax-go/docs/`.

- [X] T001 [P] Create `docs/package.json` — `name: "gh-aw-fleet-docs"`, `type: "module"`, `private: true`, scripts `dev`/`start`/`build`/`preview`/`astro`, dependency ranges matching `../ax-go/docs/`: `astro ^6.4.5`, `@astrojs/starlight ^0.40.0`, `@astrojs/sitemap ^3.7.3`, `starlight-links-validator ^0.24.1`, `sharp ^0.35.0`. The committed `docs/package-lock.json` pins resolved versions. Do NOT add the `prebuild` guard yet (added with the theme in US2).
- [X] T002 [P] Create `docs/tsconfig.json` — `extends: "astro/tsconfigs/strict"`, include `.astro/types.d.ts` + `**/*`, exclude `dist`.
- [X] T003 [P] Create `docs/.gitignore` — ignore `dist/`, `.astro/`, `node_modules/`, npm/yarn/pnpm debug logs, `.env`/`.env.production`, `.DS_Store`.
- [X] T004 [P] Update root `.gitignore` — add `node_modules/` and `docs/.astro/` (`/dist/` is already present).
- [X] T005 [P] Create root `.markdownlintignore` — exclude `docs/node_modules/`, `docs/theme/`, `docs/dist/`, `docs/.astro/`.
- [X] T006 Generate `docs/package-lock.json` — run `npm install` in `docs/` to produce the committed lockfile CI consumes via `npm ci` (depends on T001).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The routing/navigation substrate and content-collection wiring every
content page and every story depends on.

**⚠️ CRITICAL**: No content/story work can begin until this phase is complete.

- [X] T007 Create `docs/astro.config.mjs` — `site: 'https://rshade.github.io'`, `base: '/gh-aw-fleet'`; integrations `starlight({ title: 'gh-aw-fleet', social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/rshade/gh-aw-fleet' }], plugins: [starlightLinksValidator()], sidebar: [...] })` and `sitemap()`. Do NOT add `customCss` yet (added in US2). Sidebar groups are filled in T016 (depends on T001, T006).
- [X] T008 [P] Create `docs/src/content.config.ts` — `defineCollection({ loader: docsLoader(), schema: docsSchema() })` from `@astrojs/starlight/loaders` + `/schema` (modern `src/content.config.ts` location, not legacy `src/content/config.ts`) (depends on T001).

**Checkpoint**: Astro project scaffolds and routes under `/gh-aw-fleet`. User-story work can begin.

---

## Phase 3: User Story 1 - A reader finds and uses the published documentation (Priority: P1) 🎯 MVP

**Goal**: A navigable, published site at `https://rshade.github.io/gh-aw-fleet/`
carrying the core operator content, with every internal link resolving under the
subpath.

**Independent Test**: Visit the published URL; the home page returns 200 and shows
nav; install/quickstart, the `fleet.json` configuration reference, and the
reconcile-workflow page are each reachable in ≤2 clicks; in-page links resolve
under `/gh-aw-fleet` (no domain-root 404s).

### Implementation for User Story 1

- [X] T009 [P] [US1] Author the splash landing `docs/src/content/docs/index.mdx` — `template: splash`, `hero` (tagline + a "View on GitHub" action), and a `<CardGrid>` of value props, seeded from the README intro.
- [X] T010 [P] [US1] Author `docs/src/content/docs/install.md` (FR-002a) from the README "Install" section — one-liner installer (`install.sh`/`install.ps1`), `go install`, and the `main` fallback.
- [X] T011 [P] [US1] Author `docs/src/content/docs/configuration.md` (FR-002b) — a hand-authored `fleet.json` reference from `fleet.example.json` + `fleet.json`: `version`, `profiles` (`description`, `sources`, `workflows`, `tier`), `repos` (`profiles`, `engine`, `extra`, `exclude`, `cost_center`), and the `fleet.json` vs `fleet.local.json` overlay.
- [X] T012 [P] [US1] Author `docs/src/content/docs/reconcile.md` (FR-002c) — `deploy`/`sync`/`upgrade`, dry-run-by-default + `--apply`, and the three-turn mutation pattern, seeded from `AGENTS.md`/`CLAUDE.md`.
- [X] T013 [US1] Migrate `docs/finops.md` → `docs/src/content/docs/consumption.md` (FR-002d, FR-011, SC-007) — add Starlight frontmatter (`title`, `description`); rewrite the relative links `../AGENTS.md` and `../skills/fleet-budget-review/SKILL.md` to absolute `https://github.com/rshade/gh-aw-fleet/blob/main/...` URLs (they would otherwise fail `starlight-links-validator`); delete the original `docs/finops.md`.
- [X] T014 [P] [US1] Author `docs/src/content/docs/roadmap.md` (FR-002e) from `ROADMAP.md`.
- [X] T015 [P] [US1] Create discoverability assets — `docs/public/robots.txt` (allow all; `Sitemap: https://rshade.github.io/gh-aw-fleet/sitemap-index.xml`) and `docs/public/llms.txt` (base-absolute `https://rshade.github.io/gh-aw-fleet/...` links to the landing + 5 pages). These are static files Astro does NOT rewrite, so the `/gh-aw-fleet` base is spelled out (Decision 5; new vs ax-go).
- [X] T016 [US1] Fill the `sidebar` in `docs/astro.config.mjs` with the authored pages (install, configuration, reconcile, consumption, roadmap) so each is ≤2 clicks from home (SC-002) (edits T007's file; depends on T009–T014).
- [X] T017 [US1] Verify subpath link integrity (SC-006, contracts/published-routes.md C1) — run `cd docs && npm run build`; ensure `starlight-links-validator` passes; fix any broken internal links (depends on T009–T016).
- [X] T018 [US1] Create `.github/workflows/docs.yml` (FR-001, FR-007, FR-012) — `on: push` to `main` with `paths: ['docs/**', '.gitmodules', '.github/workflows/docs.yml']` + `workflow_dispatch`; `permissions: { contents: read, pages: write, id-token: write }`; `concurrency: { group: pages, cancel-in-progress: false }`; **build** job: `actions/checkout@v7` with `submodules: recursive` (no-op until US2) + the commented private-theme PAT note, `actions/setup-node@v4` (`node-version: 24`, npm cache on `docs/package-lock.json`), `npm ci` + `npm run build` (`working-directory: docs`), `actions/upload-pages-artifact@v3` (`path: docs/dist`); **deploy** job: `needs: build`, `environment: github-pages`, `actions/deploy-pages@v5`.
- [ ] T019 [MANUAL] [US1] Operator-only post-merge validation for SC-001/SC-002 — do not execute during `/speckit-implement` or from the Claude/Codex Bash tool. After the maintainer enables GitHub Pages (Pages-from-Actions) and merges the PR to `main`, collect evidence that `.github/workflows/docs.yml` deployed, the curl loop in `contracts/published-routes.md` returns HTTP 200 for home + each top-level page under `/gh-aw-fleet`, and install/configuration/reconcile are reachable from home in no more than two clicks (depends on T018).

**Checkpoint**: The published site is reachable, navigable, content-complete, and subpath-correct — MVP delivered (default Starlight theme; branding follows in US2).

---

## Phase 4: User Story 2 - The site looks like part of the family (Priority: P2)

**Goal**: The site adopts the shared brand fonts + accent from `rshade-theme`, and
the accent renders as the brand color in **both** light and dark mode (never the
framework default).

**Independent Test**: Load the site beside the blog and a sibling site; sans+mono
typefaces and accent match. Toggle light/dark; the accent stays the shared brand
color in both, not Starlight's default.

### Implementation for User Story 2

- [X] T020 [US2] Add `rshade-theme` as a git submodule at `docs/theme` — `git submodule add https://github.com/rshade/rshade-theme docs/theme` (writes root `.gitmodules`; pin by commit, no tracking branch). Provides `docs/theme/tokens.css` (FR-006, data-model Entity 3).
- [X] T021 [US2] Create `docs/src/styles/theme-bridge.css` — the **corrected** bridge (FR-005, contracts/theme-bridge.md B1/B2): `@import "../../theme/tokens.css";` fonts (`--sl-font`, `--sl-font-mono`) under `:root`; the accent ramp under **`:root, :root[data-theme="light"]`** mapping `--sl-color-accent-low/accent/high` ← `--color-accent-soft/accent/hover`. Do NOT copy ax-go's single-scheme `:root`-only file (that is the bug this feature fixes).
- [X] T022 [US2] Add `customCss: ['./src/styles/theme-bridge.css']` to `docs/astro.config.mjs` (edits T007's file; depends on T021).
- [X] T023 [US2] Add the `prebuild` submodule guard to `docs/package.json` — fails `npm run build` with a `git submodule update --init --recursive` hint if `theme/tokens.css` is absent (FR-009, Decision 8; edits T001's file).
- [X] T024 [P] [US2] Add the brand favicon `docs/public/favicon.svg` (light/dark via embedded `prefers-color-scheme`; auto-discovered by Starlight, no `favicon:` config key).
- [X] T025 [US2] Verify branding (SC-003, US2 acceptance) — `cd docs && npm run build && npm run preview`; confirm Inter/JetBrains-Mono stacks and the brand accent (`oklch(0.65 0.18 250)` family) render, and inspect computed values for `--sl-font`, `--sl-font-mono`, `--sl-color-accent-low`, `--sl-color-accent`, and `--sl-color-accent-high` in BOTH light and dark mode so the evidence proves the site is not using Starlight's default purple (depends on T020–T023).

**Checkpoint**: Site is brand-consistent in both color modes; US1 + US2 both work.

---

## Phase 5: User Story 4 - Documentation and software ship on independent tracks (Priority: P2)

**Goal**: The docs pipeline and the Go release pipeline never cross-trigger; the
docs pipeline always checks out the theme so deployed output is branded.

**Independent Test**: Push a docs-only change → docs pipeline runs, release pipeline
does not. Push a Go-only change → docs pipeline does not deploy. A release event does
not start a docs deploy outside its intended trigger.

> The path-scoped `docs.yml` was authored in T018 (US1 needs publishing); this story
> verifies the separation properties and the theme-checkout guarantee.

### Implementation for User Story 4

- [X] T026 [US4] Confirm the out-of-scope guard — review `.github/workflows/release.yml` (trigger `release: created` + `workflow_dispatch`) and `.github/workflows/ci.yml` (unscoped Go vet/fmt/test/lint) and verify this feature did NOT modify either; document why a docs push cannot trigger a release and why CI running Go-only on docs pushes is acceptable (FR-007, Decision 9).
- [X] T027 [US4] Verify FR-009/US4.3 — confirm `.github/workflows/docs.yml` checkout uses `submodules: recursive` so the deployed `docs/dist` carries the branded styling (cross-check the theme added in T020).
- [ ] T028 [MANUAL] [US4] Operator-only SC-004 validation — do not push or merge from `/speckit-implement` or from the Claude/Codex Bash tool. After the maintainer performs the docs-only and Go-only pushes, record evidence that the docs-only push runs and deploys `.github/workflows/docs.yml` while `.github/workflows/release.yml` does NOT fire, and that the Go-only push does NOT deploy `docs.yml`.

**Checkpoint**: Pipelines proven independent; branded styling reaches the deploy.

---

## Phase 6: User Story 3 - A downstream maintainer reproduces the wiring (Priority: P3)

**Goal**: A minimal, documented integration set a sibling project (finfocus, ax-go,
canonical-syndicate) can copy verbatim, changing only project-specific values.

**Independent Test**: A maintainer follows only the "how this is wired" note plus the
integration files (no access to the implementer) and reproduces a brand-consistent
site for a different project.

### Implementation for User Story 3

- [X] T029 [US3] Author `docs/README.md` — the FR-010 "how this is wired" note: list the integration file set (`astro.config.mjs`, `src/styles/theme-bridge.css`, `public/`, `.github/workflows/docs.yml`, root `.gitmodules`, `package.json` `prebuild` guard); the `git submodule add … docs/theme` command; and the two pitfalls — the both-scheme accent rule (`:root, :root[data-theme="light"]`) and the silently-failing `@import` relative path. State exactly which values change per project: `base`, `site`, `title`, `social.href`, and the absolute URLs in `robots.txt`/`llms.txt`.
- [X] T030 [US3] Verify reproducibility (SC-005) — confirm the integration set requires only project-specific edits (no design decisions); enumerate those touch-points in `docs/README.md` and sanity-check that nothing else is repo-specific (depends on T029).

**Checkpoint**: All four user stories independently functional; the wiring is copy-ready.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Repo-wide tie-off and the full Success-Criteria sweep.

- [X] T031 [P] Add a pointer to the live docs site (`https://rshade.github.io/gh-aw-fleet/`) from the root `README.md`.
- [X] T032 [P] Confirm the Go module is unaffected (FR-008) — `make ci` green; verify `go.mod` gained no dependencies. Do not substitute `go build`/`go vet`/`make test` for the full local gate.
- [ ] T033 [MANUAL] Run the `quickstart.md` "Verify before you ship" table end-to-end after post-merge deployment — sweep SC-001…SC-007 (routes 200, zero internal 404s, no more than two-click nav, computed accent/font evidence in both modes, pipeline independence evidence, migrated finops content reachable). Do not execute live push/merge checks during `/speckit-implement`.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately. T006 depends on T001.
- **Foundational (Phase 2)**: Depends on Setup. T007/T008 depend on T001 (T007 also on T006). BLOCKS all user stories.
- **User Stories (Phases 3–6)**: All depend on Foundational. Recommended order by priority: US1 (P1) → US2 (P2) → US4 (P2) → US3 (P3).
  - US1 is self-contained (default theme).
  - US2 depends on Foundational; edits the Setup/Foundational files (package.json, astro.config.mjs) additively.
  - US4 verifies `docs.yml` (authored in US1, T018) + the theme checkout (US2, T020).
  - US3 documents the integration set, so it is most complete after US1+US2+US4 land.
- **Polish (Phase 7)**: After all desired stories.

### User Story Dependencies

- **US1 (P1)**: Only Foundational. No dependency on other stories. → MVP.
- **US2 (P2)**: Foundational. Independent of US1's content (but shares the same project files; sequence US1 → US2 to avoid edit conflicts on `astro.config.mjs`).
- **US4 (P2)**: Needs `docs.yml` from US1 (T018) and the theme from US2 (T020) for the FR-009 cross-check; the SC-004 push tests (T028) need a deployed pipeline.
- **US3 (P3)**: Reads the integration files produced across US1/US2/US4; best done last.

### Within Each Story

- US1: content pages (T009–T014) → sidebar wiring (T016) → link-validate build (T017) → pipeline (T018) → stop for operator publish + verify evidence (T019).
- US2: submodule (T020) → bridge (T021) → config wiring (T022) + guard (T023) → branding verify (T025); favicon (T024) parallel.
- US4: T026/T027 (static verification) → stop for operator SC-004 evidence (T028).
- US3: note (T029) → reproducibility check (T030).

### Parallel Opportunities

- Setup T001–T005 all `[P]` (distinct files).
- Foundational T008 `[P]` with T007 (distinct files).
- US1 content T009–T014 and assets T015 are `[P]` (distinct files); T016/T017 serialize after them.
- US2 favicon T024 `[P]` alongside the bridge chain.
- Polish T031/T032 `[P]`.

---

## Parallel Example: User Story 1

```bash
# Author all content pages + discoverability assets together (distinct files):
Task: "Author splash landing in docs/src/content/docs/index.mdx"
Task: "Author install/quickstart in docs/src/content/docs/install.md"
Task: "Author fleet.json reference in docs/src/content/docs/configuration.md"
Task: "Author reconcile workflow in docs/src/content/docs/reconcile.md"
Task: "Author roadmap in docs/src/content/docs/roadmap.md"
Task: "Create docs/public/robots.txt and docs/public/llms.txt"
# (T013 migrate finops.md runs alongside; then T016 sidebar → T017 link-validate)
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Complete Phase 1 (Setup) and Phase 2 (Foundational).
2. Complete the executable Phase 3 (US1) work: author content, validate subpath links, and add `docs.yml`.
3. **STOP for operator validation**: the maintainer enables Pages, merges to `main`, and collects SC-001 (200s), SC-002 (no more than two clicks), and SC-006 (no internal 404s) evidence via T019. A usable published site already delivers the core value.

### Incremental Delivery

1. Setup + Foundational → buildable skeleton.
2. US1 → ready-to-publish, navigable, subpath-correct site; T019 confirms live publication after maintainer merge (**MVP**).
3. US2 → brand fonts + correct light/dark accent (the defect this reference fixes).
4. US4 → proven pipeline independence + theme reaches the deploy.
5. US3 → `docs/README.md` wiring note so siblings copy verbatim.
6. Polish → README pointer, Go-module sanity, then operator full-SC sweep handoff.

### Notes

- `[P]` = different files, no incomplete-task dependency.
- `[MANUAL]` = operator-only post-push/post-merge acceptance evidence; stop and hand
  off rather than executing it from `/speckit-implement`.
- US2 and US4 are both P2; deliver US2 first (branding) since US4's FR-009 check references the theme.
- The two deliberate divergences from `../ax-go/` are in US2 (corrected both-scheme bridge, T021) and US1 (added `robots.txt`/`llms.txt`, T015); everything else mirrors ax-go.
- Do not commit `docs/dist`, `docs/.astro`, or `docs/node_modules` (gitignored in T003/T004).
- `git submodule add` (T020) and `git`/`commit` operations are performed by the operator in their shell — not via the Claude Bash tool (repo invariant).
