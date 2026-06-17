# Phase 0 Research: Dependabot Config Conflict Scanner

All spec `[NEEDS CLARIFICATION]` markers were resolved during `/speckit-specify`
(severity and matching strategy were inherited from the Renovate sibling, 012). This
document records (1) the resolved decisions and (2) the Dependabot-schema knowledge
the intent-based detection (FR-012) depends on — including the **one structural
asymmetry** that makes this scanner simpler than Renovate's.

## Decision 1 — Config file probe order

**Decision**: Probe these two locations, in order, and inspect the **first** that
exists; do not merge across files:

1. `.github/dependabot.yml`
2. `.github/dependabot.yaml`

**Rationale**: Dependabot reads its config from **exactly** `.github/dependabot.yml`
(or `.yaml`) — there is no repo-root or alternate location to probe (unlike Renovate's
seven candidate paths). `.yml` is the canonical/documented extension and by far the
common case, so it is probed first; `.yaml` is accepted because GitHub honors both.
First-match-wins mirrors the existing `load.go` `probeConfigPath` philosophy and keeps
the scanner deterministic and side-effect free.

**Alternatives considered**:
- *Probing repo-root `dependabot.yml`*. Rejected — Dependabot does not read it there;
  a root file would be inert, so flagging on it would be misleading.
- *Both extensions present at once*. Not specially rejected — first-match (`.yml`
  wins) is deterministic and harmless; unlike the loader's `<base>.hujson`/`.json`
  ambiguity rule, there is no operator-authored write path here to protect.

## Decision 2 — YAML parsing via yaml.v3 (not hujson)

**Decision**: Read the file bytes and `yaml.Unmarshal` into a minimal struct, using
`gopkg.in/yaml.v3` — already an approved direct dependency (used by
`internal/fleet/frontmatter`). Any read/unmarshal failure short-circuits to the
malformed-config `INFO` finding (FR-006). Do **not** set `KnownFields(true)` —
unknown keys must be ignored, not rejected.

**Rationale**: Dependabot config is YAML, not JSON, so the Renovate sibling's
`hujson.Standardize()` path does not apply. `yaml.v3` is the project's existing YAML
parser; reusing it adds a new *import* to the `security` package but **no new
`go.mod` `require()` entry** → no constitution amendment (Third-Party Dependencies
§). YAML natively supports comments, so there is no JWCC-tolerance concern as there
was for Renovate.

**Known limitation (documented, acceptable)**: a file that is valid YAML but does not
match the expected `updates:` list shape (e.g. a legacy `version: 1` file, or a typo
that makes `updates` a scalar) simply yields no `github-actions` entry → no finding,
**not** a malformed-config note. Only a YAML *syntax* error produces the `INFO`
finding. This is honest and safe: we never block, and we never invent findings from a
structure we cannot interpret.

**Alternatives considered**: re-using `frontmatter`'s helpers. Rejected — those are
shaped for workflow-markdown frontmatter extraction, not a standalone YAML document;
a direct `yaml.Unmarshal` into a local struct is clearer and dependency-free.

## Decision 3 — Severity: `LOW` for conflict, `INFO` for malformed

**Decision** (inherits the Renovate FR-011 resolution): the conflict finding emits at
`SeverityLow`; the unparseable-config finding emits at `SeverityInfo`.

**Rationale**: `LOW` is the right tier for an *actionable config gap the operator
should fix*, distinguishing it from passive `INFO` notes. `render.go` already reserves
a `LOW` bucket in `severityTally` and `Severity.String()` handles it (the Renovate
scanner activated this path in 012), so `LOW` surfaces correctly with no render
change. The malformed note is a passive "could not read" notice, matching the existing
`fleet.frontmatter.parse-error` / `fleet.renovate.parse-error` `INFO` convention.

## Decision 4 — Intent-based, equivalence-aware protection detection

**Decision** (resolves FR-012): a `github-actions` update entry counts as
**protected** (no finding) when it expresses the *intent* to keep Dependabot off the
gh-aw action family, in any of the recognized forms below. The finding fires only when
an unprotected `github-actions` entry is found.

### Dependabot schema facts the detection relies on

- **`updates`**: top-level list. Each element is one *update entry* with a
  `package-ecosystem` (e.g. `github-actions`, `gomod`, `npm`), a `directory` (or the
  newer `directories` list/glob form), a `schedule`, and optional tuning keys.
- **`ignore`**: a per-entry list of `{dependency-name: <name-or-glob>, versions: [...]}`
  objects. Dependabot matches `dependency-name` against the action's `owner/repo`
  (and sub-path) identifier and supports `*` wildcards. **There is no file-path / glob
  ignore** — `ignore` keys on dependency *names* only.
- **`open-pull-requests-limit`**: an integer per entry. Setting it to `0` disables
  version-update PRs for that ecosystem entirely — so the entry cannot open a bump PR.

### The gh-aw action family (two distinct identifiers)

The remedy names three identifiers that span **two** different repos:

- `github/gh-aw-actions` and `github/gh-aw-actions/setup` — the gh-aw-actions action repo.
- `github/gh-aw/actions/setup-cli` — a path **inside** the `github/gh-aw` compiler repo.

All three share the lineage substring **`gh-aw`**. (Note `github/gh-aw/actions/setup-cli`
does **not** contain `gh-aw-actions`, so the Renovate sibling's `gh-aw-actions` marker
would miss it.) The spec's edge-case bullet commits the detection to keying on the
shared `gh-aw` lineage so a single reasonable ignore/wildcard covers the family.

### Recognized "protected" forms

A `github-actions` entry is **protected** if any of:
- any `ignore[].dependency-name` value **contains the substring `gh-aw`** (covers the
  three exact names, `github/gh-aw-actions*`, and `github/gh-aw*` wildcards); or
- the entry's `open-pull-requests-limit` is `0` (cannot open bump PRs → conflict
  impossible).

**Rationale**: Substring matching on the lineage marker (`gh-aw`) recognizes the
canonical block *and* the common wildcard/shorthand forms without simulating
Dependabot's glob engine. For an advisory check that runs on every deploy, minimizing
false positives on healthy-but-differently-written configs matters more than catching
pathological near-misses; a false negative here is low cost (the operator simply
doesn't get a nudge they didn't need). This mirrors the Renovate sibling's Decision 4.

**Accepted residual (documented)**: an entry that ignores *only*
`github/gh-aw/actions/setup-cli` and not the primary `gh-aw-actions` would be treated
as protected (substring `gh-aw` present) even though the primary bump risk is
uncovered. This is a contrived configuration; the false negative is acceptable per the
"favor fewer false positives" philosophy above.

**Alternatives considered**:
- *Marker `gh-aw-actions`* (matching the Renovate scanner verbatim). Rejected — it
  would false-positive on a broad `github/gh-aw*` wildcard that legitimately covers the
  family, contradicting the spec edge-case decision to key on the `gh-aw` lineage.
- *Require all three exact names present*. Rejected per FR-012 — strict matching
  false-positives on equivalent wildcard configs and nags healthy repos.
- *Full Dependabot config evaluation* (resolve org-level config, simulate the matcher).
  Rejected — re-implements Dependabot (Constitution Principle I), needs network/org
  context, and is far beyond an advisory nudge.

## Decision 5 — One conflict rule, not two (the asymmetry); per-entry findings

**Decision**:
- This scanner has exactly **one** conflict rule (gh-aw-actions-not-ignored). There is
  **no** lock-file-exclusion rule because Dependabot **cannot ignore by file glob** —
  there is no `*.lock.yml` analog to Renovate's Rule B (`matchFileNames`/`ignorePaths`).
- The remedy **must** state this explicitly (FR-004): protection is name-only, so the
  generated lock files remain reachable by Dependabot if any action they reference is
  independently named.
- Findings are emitted **per `github-actions` entry**: each unprotected entry yields
  one finding whose `Message` names the entry's `directory`, so multiple unprotected
  entries produce distinct findings. (The common case — including `rshade/finfocus#1246`
  — is a single `github-actions` entry → one finding.)

**Rationale**: The asymmetry is a real, user-facing limitation of Dependabot, not a
gap in the scanner. Educating the operator about it in the finding text is the honest
thing to do — and it is the entire reason the issue exists as a *sibling* rather than
a copy. Per-entry findings keep the advice actionable when a repo configures
`github-actions` updates for multiple directories independently.

## Decision 6 — Scope exclusions confirmed

- **No org-level config resolution**: only the repo-local `.github/dependabot.yml` is
  inspected. A repo whose protection is inherited from organization-level Dependabot
  config may get an advisory finding even when its effective config is correct
  (documented assumption in spec). Accepted false-positive source for v1.
- **No Dependabot-activation check**: the scanner reports on file contents, not on
  whether Dependabot is enabled on the repo.
- **No auto-patching**: detect-and-warn only. A comment-preserving YAML writer does not
  exist in the codebase (hujson is JSON-only), and rewriting managed-repo files
  conflicts with the thin-orchestrator value. Out of scope (spec Out of Scope).

## Decision 7 — Canonical remediation text (the exact text the finding quotes)

Emitted verbatim in the conflict finding's `Remedy` so the operator can copy-paste
(FR-010), and reproduced in [contracts/scanner-contract.md](./contracts/scanner-contract.md).
The `ignore:` block is added under the existing `github-actions` entry:

```yaml
    ignore:
      - dependency-name: "github/gh-aw-actions"
      - dependency-name: "github/gh-aw-actions/setup"
      - dependency-name: "github/gh-aw/actions/setup-cli"
```

The remedy MUST also carry the name-only caveat (FR-004), e.g.:

> Dependabot can only ignore by dependency NAME — it has no file-glob equivalent to a
> `*.lock.yml` exclusion, so the generated lock files remain reachable if any action
> they reference is independently named. `gh-aw-actions` is coupled to the `gh aw`
> compiler version and is managed atomically by the fleet via `gh aw upgrade`.
