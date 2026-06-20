# Feature Specification: Astro Starlight Documentation Site (Reference Implementation)

**Feature Branch**: `014-starlight-docs-site`  
**Created**: 2026-06-19  
**Status**: Draft  
**Input**: User description: "Scaffold Astro Starlight docs site consuming rshade-theme (reference implementation / first test)" (issue #138)

## User Scenarios & Testing *(mandatory)*

<!--
  Stories are ordered by importance. Each is independently testable: shipping
  just User Story 1 already delivers a usable, published documentation site.
-->

### User Story 1 - A reader finds and uses the published documentation (Priority: P1)

A fleet operator (or a prospective adopter who arrived from the existing
introductory blog post) opens the project's documentation URL and lands on a
navigable site. From the landing page they can reach, within a click or two,
how to install the CLI, how to author and structure a `fleet.json`, and how the
reconcile workflow (`deploy` / `sync` / `upgrade` via `gh aw`) actually works.
Internal links between pages all resolve — nothing 404s because the site is
served from a project subpath rather than the domain root.

**Why this priority**: This is the entire point of the effort. The project is
public, released, and already has an audience pointed at it by a published blog
post, but today the documentation URL returns 404. A reachable site carrying the
core operator content is the minimum that delivers value; everything else
(branding, reproducible wiring) is refinement on top of it.

**Independent Test**: Visit the published documentation URL, confirm the home
page loads successfully, and navigate to the install page, the configuration
reference, and the reconcile-workflow page — verifying each loads and that
in-page links resolve under the subpath.

**Acceptance Scenarios**:

1. **Given** the documentation site has been published, **When** a reader opens
   the project documentation URL, **Then** the home page loads successfully
   (HTTP 200) and presents site navigation.
2. **Given** the reader is on the home page, **When** they look for installation
   instructions, **Then** they can reach an install/quickstart page within two
   clicks and follow it to install the CLI.
3. **Given** the reader is on any documentation page, **When** they follow an
   internal link to another page, **Then** the link resolves correctly under the
   project subpath (no broken links, no domain-root 404s).
4. **Given** the reader wants to understand fleet configuration, **When** they
   open the configuration reference, **Then** the meaning of the `fleet.json`
   fields and the reconcile workflow are documented and understandable without
   reading the source code.

---

### User Story 2 - The site looks like part of the family (Priority: P2)

A reader who has seen the introductory blog post or a sibling project's site
(e.g. finfocus) opens this documentation and immediately recognizes it as the
same brand: the same typefaces, the same accent color. The accent is correct
whether the reader's browser/OS is in light mode or dark mode — it does not fall
back to a generic framework default in either mode.

**Why this priority**: Brand consistency across the project's web properties is a
stated goal, and getting the light/dark accent correct is the specific defect
this effort exists to get right the first time (so downstream sites inherit a
working pattern rather than a broken one). It is high value but not required for
the documentation to be *useful*, so it ranks below US1.

**Independent Test**: Load the published site side-by-side with the blog and a
sibling site; confirm the fonts and accent color match. Toggle between light and
dark color modes and confirm the accent remains the shared brand color in both —
not the framework's stock accent.

**Acceptance Scenarios**:

1. **Given** the site is published, **When** a reader views it alongside the blog
   and a sibling site, **Then** the typefaces (sans + monospace) and accent color
   visually match across all three.
2. **Given** the reader's environment is in **light** mode, **When** they view
   any page, **Then** interactive/accent elements use the shared brand accent
   color, not the framework default.
3. **Given** the reader's environment is in **dark** mode, **When** they view any
   page, **Then** interactive/accent elements use the shared brand accent color.

---

### User Story 3 - A downstream maintainer reproduces the wiring (Priority: P3)

A maintainer of a sibling project (finfocus, ax-go, canonical-syndicate) needs to
stand up the same kind of documentation site. They look at this project as the
reference, copy the small, documented set of integration files and steps, and get
a brand-consistent site of their own — without re-deriving how the shared theme
plugs in or re-discovering the light/dark accent pitfall.

**Why this priority**: This effort is explicitly the *first* of four; its reason
for existing is to prevent each sibling from diverging on its own wiring. Capturing
the pattern cleanly and documenting it is what makes the other three cheap and
consistent. It is valuable leverage but trails the two reader-facing stories.

**Independent Test**: A maintainer (or a reviewer acting as one) follows only the
documented "how this is wired" note plus the integration files, with no access to
the implementer, and reproduces a brand-consistent site for a different project.

**Acceptance Scenarios**:

1. **Given** the reference site is complete, **When** a downstream maintainer reads
   the wiring note and the integration files, **Then** the steps are sufficient to
   replicate the setup without further design decisions.
2. **Given** the downstream maintainer copies the wiring verbatim and adjusts only
   project-specific values (project name / subpath), **When** they publish, **Then**
   their site is brand-consistent and the accent renders in both light and dark mode.

---

### User Story 4 - Documentation and software ship on independent tracks (Priority: P2)

The project maintainer edits a documentation page and pushes the change. The docs
site updates on its own track without triggering a software release. Conversely,
cutting a CLI release does not kick off a documentation rebuild beyond what is
intended. The two pipelines do not interfere with each other.

**Why this priority**: The CLI release pipeline (goreleaser, driven by
release-please) is load-bearing and must not be perturbed by documentation churn,
and vice versa. Cross-firing pipelines would be an operational hazard, so this
ranks alongside branding as important but not the core reader value.

**Independent Test**: Push a docs-only change and confirm the docs pipeline runs
while the release pipeline does not. Confirm a release event does not start the
docs pipeline outside its intended trigger.

**Acceptance Scenarios**:

1. **Given** a change touches only documentation, **When** it is pushed, **Then**
   the documentation pipeline runs and the software release pipeline does not.
2. **Given** a change touches only Go/CLI sources, **When** it is pushed, **Then**
   the documentation pipeline does not run a deploy.
3. **Given** the documentation pipeline runs, **When** it builds the site, **Then**
   it checks out the shared theme dependency so the branded styling is present in
   the published output.

### Edge Cases

- **Shared theme not present at build time**: If the documentation pipeline builds
  without checking out the shared theme dependency, the published site would lose
  its branded fonts/accent (or fail to build). The pipeline MUST always make the
  shared theme available during the build.
- **Accent set for only the default color scheme**: If the brand accent is applied
  only to the default scheme and not also to the explicit light scheme, light mode
  silently reverts to the framework's stock accent. Both schemes MUST be covered.
- **Subpath not configured**: If the site is built for the domain root instead of
  the project subpath, internal links and asset references break (404s) once
  deployed under `/<project>/`. The build MUST target the project subpath.
- **Upstream dependency not yet landed**: The shared theme's design tokens must
  exist before this site can render the brand. This is the only external blocker
  (see Dependencies).
- **Existing raw documentation file**: A pre-existing raw markdown document already
  lives under `docs/`; it must be carried into the new site's content rather than
  left stranded or silently lost.
- **Docs build failure does not block releases**: A failing documentation build
  must not prevent or gate a software release (independent tracks).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The project MUST publish a browsable documentation site reachable at
  the project's documentation URL (a project subpath of the existing pages domain),
  returning a successful response for the home page and each top-level page.
- **FR-002**: The site MUST present documentation seeded from existing project
  material, covering at minimum: (a) installation and quickstart, (b) a `fleet.json`
  configuration reference derived from the committed example configuration, (c) the
  `gh aw` reconcile workflow (`deploy` / `sync` / `upgrade`), (d) the FinOps /
  consumption material, and (e) the roadmap.
- **FR-003**: All internal navigation and in-page links MUST resolve correctly when
  the site is served from the project subpath (no domain-root assumptions).
- **FR-004**: The site MUST adopt the shared brand styling — the shared sans and
  monospace typefaces and the shared accent color — so it is visually consistent
  with the project blog and sibling project sites.
- **FR-005**: The brand accent MUST render correctly in **both** light and dark
  color modes; it MUST NOT fall back to the framework's default accent in either
  mode.
- **FR-006**: The site MUST consume the shared brand styling from the shared theme
  as a centrally-updatable, versioned dependency (so a theme change can propagate to
  all consuming sites) rather than by copying token values into this repository.
- **FR-007**: The documentation build/deploy MUST run as a pipeline separate from
  the Go release / goreleaser pipeline, with independent triggers and path scoping so
  documentation changes and software releases do not cross-trigger each other.
- **FR-008**: The documentation project MUST be self-contained within the `docs/`
  directory and independent of the Go module and its release tooling (its own
  dependency manifest; not entangled with the Go build or goreleaser).
- **FR-009**: The documentation pipeline MUST make the shared theme dependency
  available during the build (e.g. checking out the submodule) so the published
  output carries the branded styling.
- **FR-010**: The shared-theme integration MUST be minimal and documented (a short
  "how this is wired" explanation plus the small set of integration files) so that
  sibling projects can replicate it verbatim, changing only project-specific values.
- **FR-011**: Any pre-existing documentation content under `docs/` MUST be carried
  into the published site rather than orphaned by the new project layout.
- **FR-012**: Once enabled, publishing MUST be automated — a merged documentation
  change reaches the live site through the pipeline without manual build/upload steps.

### Key Entities *(include if feature involves data)*

- **Documentation site**: The published, navigable set of pages served at the
  project subpath; the primary deliverable.
- **Content pages**: The individual documentation pages (install/quickstart,
  configuration reference, reconcile workflow, FinOps/consumption, roadmap), seeded
  from existing project material.
- **Shared theme**: The externally-maintained source of brand design tokens (fonts,
  accent ramp) consumed as a versioned dependency; the single source of truth for
  cross-site visual consistency.
- **Theme bridge**: The small, documented mapping that connects the shared theme's
  tokens to the documentation framework's styling, covering both light and dark
  modes; the artifact downstream sites copy.
- **Documentation pipeline**: The automation that builds and publishes the site on
  its own trigger and path scope, independent of the software release pipeline, and
  that makes the shared theme available at build time.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The documentation home page and every top-level page return a
  successful response (HTTP 200) at the project documentation URL.
- **SC-002**: A reader can reach install instructions, the `fleet.json`
  configuration reference, and the reconcile-workflow page from the landing page in
  no more than two clicks each.
- **SC-003**: In a side-by-side comparison with the blog and a sibling site, the
  typefaces and accent color match across all three, and computed CSS inspection
  shows the Starlight font variables and accent variables resolve to shared theme
  token values in **both** light and dark mode (not the framework default).
- **SC-004**: A documentation-only change reaches the live site through the
  documentation pipeline without triggering a software release, and a software
  release does not trigger a documentation deploy outside its intended scope —
  verified across at least one change of each kind.
- **SC-005**: A person who is not the implementer can reproduce the theme wiring for
  a different project using only the documented note and integration files, with no
  additional design decisions required.
- **SC-006**: Zero internal links 404 when the site is served from the project
  subpath (verified by a full-site link check of the published output).
- **SC-007**: The pre-existing documentation content previously under `docs/` is
  present and reachable in the published site.

## Assumptions

- **Toolchain is prescribed, not chosen**: The documentation framework (Astro
  Starlight) and the shared-theme mechanism (a git submodule of `rshade-theme`,
  wired via a CSS token bridge) are fixed inputs from issue #138. This effort is the
  designated *reference implementation*, so the value is in establishing the exact
  wiring the three sibling sites will copy — the framework is a constraint, not an
  open design decision. The reviewed bridge snippet in the issue (both-theme accent
  targeting, including the low-accent mapping) is authoritative over any older
  single-scheme version in the theme's README.
- **Publishing target**: GitHub Pages served from Actions, at the project subpath
  `rshade.github.io/gh-aw-fleet/` (`base: '/gh-aw-fleet'`, `site:
  'https://rshade.github.io'`). Project Pages sites are served from a subpath, which
  is why subpath-correct links (FR-003) matter.
- **Pages enablement**: Enabling GitHub Pages (Pages-from-Actions) on the repository
  is a one-time setup action that accompanies this effort; it is assumed to be
  performed as part of landing the feature.
- **Configuration reference is hand-authored**: The `fleet.json` reference is written
  documentation derived from the committed `fleet.json` / `fleet.example.json`, not
  an auto-generated schema dump, unless a generator already exists.
- **Existing raw doc migrates**: `docs/finops.md` is carried into the new content
  tree (it currently sits at the path that becomes the Astro project root), and any
  external references to it are acceptable to redirect or update.
- **Built-in site features are sufficient**: Standard documentation-framework
  capabilities (navigation, search, responsive layout) are used as shipped; bespoke
  search, multi-version docs, and localization are out of scope for this first site.
- **Independent dependency surface**: Because the documentation project is a separate
  Node/Astro project under `docs/`, the Go module's "no new third-party Go
  dependencies" constitutional rule is not implicated; the docs project manages its
  own dependencies independently of `go.mod`.

## Dependencies

- **Shared theme tokens must land first**: The `rshade-theme` design tokens
  (`tokens.css`) must exist and be consumable before this site can render the brand.
  This is the single stated external blocker for issue #138.
- **Existing release pipeline**: The Go release pipeline (`release.yml` /
  goreleaser, driven by release-please) and CI (`ci.yml`) already exist; the new
  documentation pipeline must coexist with them without cross-triggering.

## Out of Scope

- Migrating sibling projects (finfocus, ax-go, canonical-syndicate) — they copy this
  pattern in their own subsequent efforts.
- Changes to the CLI, the Go build, goreleaser, or the release-please configuration.
- A custom domain for the documentation site (the project subpath is the target).
- Multi-version documentation, localization, or a bespoke search backend.
