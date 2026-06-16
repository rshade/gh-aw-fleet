# Phase 0 Research: Renovate Config Conflict Scanner

All spec `[NEEDS CLARIFICATION]` markers were resolved during `/speckit-specify`.
This document records (1) the resolved decisions and (2) the Renovate-schema
knowledge the intent-based detection (FR-012) depends on.

## Decision 1 — Config file probe order

**Decision**: Probe these locations, in order, and inspect the **first** that
exists; do not merge across files:

1. `renovate.json`
2. `renovate.json5`
3. `.renovaterc`
4. `.renovaterc.json`
5. `.renovaterc.json5`
6. `.github/renovate.json`
7. `.github/renovate.json5`

**Rationale**: Covers the locations Renovate itself reads. This **extends** the
five-entry list in issue #100 with the two `.json5` variants (`.renovaterc.json5`,
`.github/renovate.json5`) so JSON5/JWCC syntax (Decision 2 / FR-002) is reachable in
*every* supported location, not only repo-root — otherwise a commented config under
`.github/` would be unreachable and the JSON5-tolerance guarantee would be partial.
First-match-wins mirrors the existing `load.go` `probeConfigPath` philosophy and
keeps the scanner deterministic and side-effect free.

**Alternatives considered**:
- *Renovate's exact precedence order* (`renovate.json` → `renovate.json5` →
  `.github/renovate.json` → `.github/renovate.json5` → `.gitlab/...` →
  `.renovaterc*` → `package.json`). Rejected as the v1 order because it keeps the
  issue #100 relative order (`.renovaterc` before `.github/renovate.json`); the
  divergence is immaterial for an advisory check whose only job is to find *a* config
  to advise on. The `.json5` variants are now folded into the list above; `.gitlab/*`
  and the `package.json` `renovate` key remain out of scope for v1.
- *Scanning all present files and merging*. Rejected — Renovate uses one config
  file, not a merge; scanning multiple invites contradictory advice.

## Decision 2 — JSON5 / comment tolerance via hujson

**Decision**: Read the file bytes, run `hujson.Standardize()`, then
`json.Unmarshal` into a minimal struct. Reuses the already-approved
`github.com/tailscale/hujson` dependency (no new dep, no constitution amendment).

**Rationale**: `hujson` handles JWCC (JSON With Commas and Comments): `//` line
comments, `/* */` block comments, and trailing commas — the syntax operators
actually use in `renovate.json5` / commented `renovate.json`. This is the same
read-path treatment `load.go` already applies to fleet config files.

**Known limitation (documented, acceptable)**: `hujson` standardizes JWCC, **not**
the full JSON5 grammar. A config using JSON5-only syntax — single-quoted strings,
unquoted object keys, hex/leading-`.` numbers — will fail `hujson.Standardize()`.
Per the Scanner contract that degrades gracefully to the malformed-config path
(one `INFO` finding, never a panic or error — FR-006). This is honest and safe: we
never block, and the common comment/trailing-comma cases are covered.

**Alternatives considered**: a full JSON5 parser. Rejected — would add a new
dependency for a long-tail syntax, violating the no-new-deps constraint for marginal
coverage.

## Decision 3 — Severity: `LOW` for conflicts, `INFO` for malformed

**Decision** (resolves FR-011): the two conflict findings emit at `SeverityLow`;
the unparseable-config finding emits at `SeverityInfo`.

**Rationale**: `LOW` (currently defined but unused — "reserved for future use" in
`finding.go`) is the right tier for an *actionable config gap the operator should
fix*, distinguishing it from passive `INFO` notes (scanner-skip, malformed
frontmatter). `render.go` already reserves a `LOW` bucket in `severityTally` and
`Severity.String()` handles it, so `LOW` surfaces correctly with no render change.
The malformed note is a passive "could not read" notice, matching the existing
`fleet.frontmatter.parse-error` `INFO` convention.

## Decision 4 — Intent-based, equivalence-aware rule-presence detection

**Decision** (resolves FR-012): a required rule counts as **present** when the
config expresses the *intent* to disable, in any of the recognized forms below.
Findings fire only when no recognized form is found.

### Renovate schema facts the detection relies on

- **`packageRules`**: top-level array of objects. Each object pairs matchers with
  actions. The action that disables updates is `"enabled": false`.
- **Package matchers** (any of these may carry the gh-aw-actions names): The
  GitHub-Action dependency is identified by its `owner/repo` (and sub-action path),
  e.g. `github/gh-aw-actions`. Renovate's current matcher is `matchPackageNames`
  (which also accepts `/regex/` and glob entries); deprecated-but-seen equivalents
  include `matchPackagePatterns`, `matchPackagePrefixes`, `matchDepNames`,
  `matchDepPatterns`.
- **File matchers**: `matchFileNames` (current) selects rules by file glob;
  `matchPaths` is the deprecated predecessor.
- **`ignoreDeps`**: top-level array of dependency names Renovate ignores entirely —
  equivalent to disabling those packages.
- **`ignorePaths`**: top-level array of path globs Renovate excludes — equivalent to
  disabling those files.
- **Root `"enabled": false`**: disables Renovate for the whole repo — no conflict is
  possible.

### Recognized "present" forms

**Rule A (gh-aw-actions disabled)** is present if any of:
- a `packageRules[]` entry with `enabled: false` whose any package matcher value
  (`matchPackageNames` / `matchPackagePatterns` / `matchPackagePrefixes` /
  `matchDepNames` / `matchDepPatterns`) **contains the substring `gh-aw-actions`**;
  or
- top-level `ignoreDeps[]` contains an entry with the substring `gh-aw-actions`; or
- root `enabled: false`.

**Rule B (lock files excluded)** is present if any of:
- a `packageRules[]` entry with `enabled: false` whose any file matcher value
  (`matchFileNames` / `matchPaths`) **contains the substring `.lock.yml`**; or
- top-level `ignorePaths[]` contains an entry with the substring `.lock.yml`; or
- root `enabled: false`.

**Rationale**: Substring matching on the package marker (`gh-aw-actions`) and the
lock-file marker (`.lock.yml`) recognizes the canonical block *and* the common
alternate expressions (different match key, `ignoreDeps`/`ignorePaths`, broader
globs like `**/*.lock.yml`) without simulating Renovate's full glob/regex engine.
For an advisory check that runs on every deploy, minimizing false positives on
healthy-but-differently-written configs matters more than catching pathological
near-misses; a false negative here is low cost (the operator simply doesn't get a
nudge they didn't need).

**Alternatives considered**:
- *Strict canonical-block match*. Rejected per the operator's FR-012 decision — it
  false-positives on equivalent configs and nags healthy repos.
- *Full Renovate config evaluation* (resolve `extends`, run the matcher engine).
  Rejected — re-implements Renovate (Constitution Principle I), needs network for
  remote presets, and is far beyond an advisory nudge.

## Decision 5 — Scope exclusions confirmed

- **No preset/`extends` resolution**: only the local file's inline rules are
  inspected. A repo whose rules are entirely inherited may get an advisory finding
  even when its effective config is correct (documented assumption in spec). This is
  an accepted false-positive source for v1.
- **No `package.json` `renovate` key**: outside the probe list; out of scope.
- **No Renovate-activation check**: the scanner reports on file contents, not on
  whether Renovate is installed/enabled on the repo.

## Decision 6 — Canonical remediation blocks (the exact text findings quote)

These are emitted verbatim in each conflict finding's remediation so the operator
can copy-paste. Mirrored in [contracts/scanner-contract.md](./contracts/scanner-contract.md).

**Rule A** — disable gh-aw-actions package bumps:

```json
{
  "matchPackageNames": [
    "github/gh-aw-actions",
    "github/gh-aw-actions/setup",
    "github/gh-aw/actions/setup-cli"
  ],
  "enabled": false,
  "description": "gh-aw-actions version is coupled to the gh aw compiler version baked into lock file metadata. Renovate bumping it directly breaks hash validation. Managed atomically by the fleet tool via: gh aw upgrade"
}
```

**Rule B** — keep Renovate off the generated lock files:

```json
{
  "description": "gh-aw generates .github/workflows/*.lock.yml via 'gh aw compile'. Their pinned action SHAs and container tags are managed by gh-aw (recompiling bumps them), so Renovate must not touch them: edits would be reverted on the next compile and can break the gh-aw integrity manifest.",
  "matchFileNames": [
    ".github/workflows/*.lock.yml"
  ],
  "enabled": false
}
```
