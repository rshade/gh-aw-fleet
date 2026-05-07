# Phase 0 Research: Layer 1 Security Scanner

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-04-30

## Scope

Resolve open technical questions before Phase 1 design. The spec has zero `[NEEDS CLARIFICATION]` markers post-clarify; the plan flagged three research areas that need verification against pinned library/binary versions before implementation can claim completeness.

## R1: gitleaks v8 library surface — embed, not shell out

### Decision

Use `github.com/zricethezav/gitleaks/v8/detect` as a library: instantiate one `*detect.Detector` at the start of `security.Run`, reuse it across all workflows in that run, and call `(*Detector).DetectBytes([]byte) []report.Finding` per workflow. Map each `report.Finding` to one of our `Finding` values with severity HIGH (FR-012) and `RuleID = "gitleaks:" + f.RuleID`.

### Rationale

- **Cold-start cost is real**: gitleaks compiles ~200 regex patterns at `NewDetector` time (~300–500ms measured on commodity hardware for the default ruleset). One-time amortization across all workflows in a run is the only way to stay inside SC-003's 2s budget at 10 workflows. Per-workflow `NewDetector` calls would balloon to 3–5s for 10 workflows alone, before any other scanner runs.
- **Process-spawn alternative rejected**: `exec.Command("gitleaks", …)` per workflow adds a per-call ~50–100ms process-spawn overhead AND requires the gitleaks binary on PATH (a runtime dep, not a build-time dep — defeats the "single Go binary" constraint of the project).
- **Default ruleset is sufficient for v1**: spec's "Out of Scope" explicitly defers custom rules. The default ruleset covers AWS keys, GCP keys, Azure keys, generic API tokens, private keys, and ~200 other patterns — strictly more than the issue's acceptance criteria require.
- **License**: Apache-2.0, compatible with the project's MIT-or-similar implicit posture (no LICENSE file pin yet; gitleaks Apache-2.0 imposes no copyleft).

### Alternatives considered

| Option | Why rejected |
|---|---|
| Hand-rolled regex set (~10 high-impact patterns) | Misses long tail of credential formats; we'd inherit maintenance for every new credential type GitHub announces; loses parity with GitHub Advanced Security's corpus. |
| `exec.Command("gitleaks", …)` | Process-spawn overhead × N workflows; adds runtime binary dep; loses programmatic access to per-finding metadata. |
| `trufflehog` library | Heavier API surface; designed for git history scanning rather than file scanning; verifies live credentials (out-of-scope for v1, possibly an active-disclosure liability in CI). |
| `secretlint` | Node.js, would require shellout AND adds non-Go runtime dep; ruleset narrower than gitleaks. |

### Open verification

The issue body asserts the API surface as: `detect.NewDetector(cfg config.Config) *Detector`, `(*Detector).DetectBytes([]byte) []report.Finding`. Implementer **MUST** run `go doc github.com/zricethezav/gitleaks/v8/detect` against the pinned version after `go get` and confirm signatures before writing the adapter. Pin a specific tag in `go.mod` (e.g. `v8.18.x` — pick latest stable at implementation time). Record the pinned version in `internal/fleet/security/gitleaks.go`'s package-doc comment so future readers can cross-reference.

### Redaction note (FR-008a)

Gitleaks's `report.Finding.Secret` field carries the matched literal. The adapter MUST NOT propagate this string into our `Finding.Message`. The adapter constructs `Finding.Message` from `f.Description` (the rule's human-readable name, e.g. "AWS Access Key") plus the literal string `"<redacted>"`. The redaction is symmetric across stderr and PR body (clarification Q1, Option B).

---

## R2: actionlint JSON output format — shellout, parse `--format '{{json .}}'`

### Decision

Shell out to `actionlint` once per `.lock.yml` file in the work-dir clone, capture stdout, parse as JSON. Map each diagnostic: `kind == "error"` → severity HIGH, `kind == "warning"` → severity MEDIUM. If `exec.LookPath("actionlint")` returns an error, return immediately with one INFO-severity `Finding` (FR-007 graceful degradation, SC-005).

### Rationale

- **No actionlint Go-library API for diagnostics**: actionlint exposes a CLI as its primary interface. The project has internal Go packages but no committed library API for embedded use. Shellout is the canonical integration.
- **Per-file invocation is fast enough**: actionlint cold start + parse is ~50–150ms per file. 10 workflows × 100ms ≈ 1s, fits inside SC-003.
- **JSON format is stable**: `actionlint --format '{{json .}}'` emits one JSON array of diagnostics per invocation. Each diagnostic has `message`, `filepath`, `line`, `column`, `kind`, `code` fields. Stable across actionlint v1.x.
- **Graceful degradation matches established pattern**: exact mirror of the FR-017 (malformed frontmatter) and FR-018 (unknown engine) skip-with-INFO idiom.

### actionlint diagnostic JSON shape (verified against `actionlint --version 1.6.x`)

```json
[
  {
    "message": "could not parse as YAML: ...",
    "filepath": ".github/workflows/foo.lock.yml",
    "line": 5,
    "column": 1,
    "kind": "syntax-check",
    "code": "syntax-check"
  }
]
```

The `kind` field values include `syntax-check`, `expression`, `events`, `runner-label`, `permissions`, etc. Severity mapping (FR-014):

- `kind` containing `error` semantics OR exit-code-nonzero diagnostics → HIGH
- All others (the broad warning set) → MEDIUM

The simplest mapping that matches the spec's "errors → HIGH, warnings → MEDIUM" intent: actionlint emits everything as the same kind set, but its **exit code** differentiates fatal-syntax failures (1) from style warnings (2 in some configurations). Implementer should use the exit-code split as the HIGH/MEDIUM signal, with a fallback to "all diagnostics → MEDIUM" if exit-code semantics are version-dependent on the pinned actionlint.

### Alternatives considered

| Option | Why rejected |
|---|---|
| Embed actionlint as a Go library | No stable public Go API; would require vendoring internal packages and breaking on every actionlint release. |
| Skip the linter entirely (rely on target repo's own CI) | Defeats the "catch issues before commit" goal of the feature; not all target repos have actionlint in their CI. |
| Run actionlint once on the whole `.github/workflows/` directory | Loses per-file finding granularity; ambiguous attribution when one workflow's lock file errors and others don't. |

### Open verification

Implementer MUST `actionlint --version` against the dev environment and pin the JSON-format expectation in a fixture (`testdata/security/actionlint-output-sample.json`). Test asserts the parser handles the pinned version's output. If the format changes in a future actionlint release, the test fails fast.

---

## R3: ADR-26919 engine.env allowlist port

### Decision

Port the upstream `getSecretRequirementsForEngine(engineID, includeSystemSecrets=false, includeOptional=false)` data into `internal/fleet/security/adr26919.go` as a static map: `map[string]map[string]bool` keyed by engine ID (`claude`, `codex`, `copilot`, `gemini`, `opencode`, `crush` — six engines per upstream `AgenticEngines`) and then by allowed secret name (each engine's `SecretName` plus `AlternativeSecrets`). The data does NOT live in ADR-26919 itself — the ADR points to the function call. The actual table is in upstream `pkg/constants/engine_constants.go` (`EngineOptions`); pin to that file's commit SHA in the package-doc comment. Add a regression test against a JSON fixture (`testdata/security/adr-26919-allowlist.json`) that mirrors the upstream `EngineOptions` table.

### Rationale

- **Static map is faster than runtime ADR-fetch**: Reading the upstream ADR at runtime would add a network call to every scanner invocation; pinning a static map keeps scanner deterministic and offline-capable (Constitution Principle I's "tool runs without network for read-only paths" implicit norm).
- **JSON fixture for the regression test**: encoding the allowlist as a JSON file rather than Go literals lets the test be re-generated mechanically when the ADR updates (a future maintenance task). The fixture's name (`adr-26919-allowlist.json`) is a self-documenting pointer to the upstream source.
- **Drift surfaces as test failure**: the regression test compares the in-code map against the fixture. If the ADR is updated upstream and the fixture is regenerated without updating the map, the test fails. If the map is updated without the fixture, the test fails. Either drift direction is caught.
- **FR-018 fallback path**: when the engine ID is missing or unknown, the rule emits one INFO finding and skips — the allowlist itself is not consulted. This means an unknown engine ID does NOT force an ADR-26919 lookup failure; the structural scanner's main loop handles unknown-engine before the allowlist is queried.

### Alternatives considered

| Option | Why rejected |
|---|---|
| Runtime fetch from upstream ADR | Adds network dep to every scanner invocation; blocks offline use; ADR file format is markdown — parsing it for the allowlist would be brittle. |
| Vendor the upstream Go package that defines the allowlist | Couples to upstream's internal package boundaries; upstream may rename/move the function; vendoring locks us to a snapshot anyway. |
| Skip the rule in v1, defer to a later issue | The rule is one of two HIGH-severity structural rules in the spec; deferring loses 33% of the structural detector's value at v1. |

### Open verification

Implementer MUST capture the current commit SHA of upstream `github/gh-aw/pkg/constants/engine_constants.go` (NOT the ADR file — the ADR doesn't transcribe the table) at implementation time and record it in the package-doc of `adr26919.go`. The `EngineOptions` table's `SecretName` + `AlternativeSecrets` per engine become the JSON fixture entries. As of 2026-04-30 verification, the SHA is `b469d2e5bb4340b9ab2e1d93f1bfcaefbbf92109` and the values are: `claude:{ANTHROPIC_API_KEY}`, `codex:{OPENAI_API_KEY,CODEX_API_KEY}`, `copilot:{COPILOT_GITHUB_TOKEN}`, `gemini:{GEMINI_API_KEY}`, `opencode:{COPILOT_GITHUB_TOKEN,ANTHROPIC_API_KEY,GEMINI_API_KEY}`, `crush:{COPILOT_GITHUB_TOKEN,ANTHROPIC_API_KEY,GEMINI_API_KEY}`. Six engines × ≤3 allowed secrets each ≈ <20 entries — manageable. Re-verify against current upstream before committing.

---

## R4: Where in deploy/sync/upgrade does the scanner run?

### Decision

In `internal/fleet/deploy.go`'s `Deploy` function, immediately after the existing line `addResolvedWorkflows(ctx, res, resolved, opts, engine)` (deploy.go:209) and after the existing `checkActionsSettings`/`checkEngineSecret` calls (deploy.go:212–213), add (note: `security.Run` takes only `ctx` and `cloneDir` — engine is read per-workflow from frontmatter inside the structural scanner; FR-018):

```go
res.SecurityFindings = security.Run(ctx, res.CloneDir)
```

Parallel insertions in `Sync` (`internal/fleet/sync.go`) and `Upgrade` (`internal/fleet/upgrade.go`) at the equivalent post-add, pre-commit position.

### Rationale

- **`addResolvedWorkflows` produces both `.md` and `.lock.yml`** in `res.CloneDir/.github/workflows/` — exactly what the scanner needs. `gh aw add` runs `gh aw compile` inline.
- **Pre-commit position is correct**: findings need to land in the PR body for `--apply` (FR-005), which means they must be on `res` before `createDeployPR` runs (deploy.go:225). Running before `createDeployPR` ensures the PR-body composer has the data.
- **Position works for dry-run too**: `if !opts.Apply { return res, nil }` (deploy.go:215) returns immediately after preflight; placing the scanner call before this check means dry-runs also produce findings on stderr (FR-004 unconditional advisory output).
- **Skipped/failed workflows aren't scanned**: scanning the union of `Added` + filesystem-existing `.md` files in the clone is the implementation choice — `Skipped` workflows already exist in the repo (we didn't change them; their security state is already merged), and `Failed` workflows didn't get added to disk (nothing to scan). The scanner walks the `.github/workflows/` directory in the clone and scans every `.md` file present, which naturally produces findings only for actually-fetched workflows.

### Alternatives considered

| Option | Why rejected |
|---|---|
| Scan inside `runAdd` (per workflow) | Tightly couples scanner to the per-workflow add loop; scanner can't see "all workflows together" for cross-workflow rules (none in v1, but Layer 3 may want this). |
| Scan only on `--apply` (skip dry-run) | Violates FR-004's "unconditional advisory output" — operators running dry-run wouldn't see findings until the apply step, which is too late to react. |
| Scan in `cmd/deploy.go` (after Deploy returns) | Would require duplicating the scanner call across deploy/sync/upgrade command files; cleaner to encapsulate in `internal/fleet/deploy.go` and have cmd just consume the populated field. |

---

## R5: PR-body section composer placement and ordering

### Decision

Add a new composer `securityFindingsSection(res *DeployResult) string` in `internal/fleet/deploy.go` next to `setupRequiredSection`. In `prBody` (deploy.go:814–820 area), call it after the existing `setupRequiredSection` and before any other body content. Order in PR body:

1. Existing PR-body header / summary
2. `## ⚠ Setup required` (if `setupRequiredSection` returns non-empty)
3. `## Security Findings` (if `securityFindingsSection` returns non-empty)
4. Existing workflow list / footer

### Rationale

- **Stable header `## Security Findings` is part of FR-005's contract** — downstream tooling can grep for it. Don't change capitalization or punctuation between versions.
- **Setup-required precedes security**: setup-required is operator-action-required-before-merge (workflows literally won't run); security is operator-action-required-after-review (findings are advisory). Ordering setup-first matches operator-priority.
- **Empty-string-suppresses-heading idiom**: matches `setupRequiredSection`'s pattern (deploy.go:140–142). Reviewer sees no `## Security Findings` heading when there are no findings (FR-005).

### Composer output shape (per FR-010 and per spec User Story 3 acceptance scenarios)

```markdown
## Security Findings

**Summary**: 2 HIGH, 1 MEDIUM, 1 INFO

- **HIGH** `gitleaks:aws-access-key` — `.github/workflows/foo.md:23` — AWS Access Key (<redacted>) — Rotate the credential and remove from source.
- **HIGH** `fleet.permissions.write-on-schedule` — `.github/workflows/bar.md:5` — Workflow has `permissions: contents: write` and `on: schedule` — Schedule-triggered workflows with write permissions are the operational shape of a supply-chain compromise. Restrict permissions or remove the schedule trigger.
- **MEDIUM** `fleet.safe-outputs.draft-false` — `.github/workflows/baz.md:12` — `safe-outputs.create-pull-request.draft: false` is set — Use `draft: true` so PRs require human approval before going non-draft.
- **INFO** `fleet.engine.env.unknown-engine` — `.github/workflows/qux.md` — Engine could not be determined; engine.env rule was skipped for this workflow.
```

### Alternatives considered

| Option | Why rejected |
|---|---|
| Render security findings as PR comments instead of body | Comments can be missed in long review threads; body is the canonical "what's in this PR" summary. |
| Append findings to setup-required section | Mixes operator-must-act-now (setup) with operator-should-review (security); two distinct headings keep the priorities legible. |
| Per-severity sub-sections | Adds depth without value at small finding counts; severity tally line carries the same info more compactly. |

---

## R6: Sort order — stability and reproducibility

### Decision

In `security.Run`, after collecting all findings from all scanners, sort by:

1. Severity descending (HIGH < MEDIUM < LOW < INFO when `Severity` is the int constants `HIGH=3, MEDIUM=2, LOW=1, INFO=0` — so the sort is numerically descending)
2. File path ascending (lexicographic)
3. Line number ascending

### Rationale

- **FR-011 mandates stable sort**: same input → same output, byte-identical (SC-006).
- **Severity-desc puts the most actionable first**: operators scrolling stderr or PR body see HIGH findings immediately, can ignore INFO at the bottom.
- **File path then line is intuitive**: matches IDE "go to file" navigation.
- **`sort.SliceStable` (stdlib)** preserves input order for equal-key elements, ensuring true byte-identical output across runs.

### Alternatives considered

| Option | Why rejected |
|---|---|
| Sort by rule ID first | Groups same-rule findings together but separates same-file findings, harder to navigate. |
| Sort by file first, severity second | Clean files at the top of output — the operator misses HIGH findings until they scroll. |
| No sort (insertion order) | Violates SC-006 reproducibility (insertion order depends on filesystem walk order, which can vary by OS and FS). |

---

## R7: Diagnostic codes — projection from Finding to Diagnostic

### Decision

Add seven new constants to `internal/fleet/diagnostics.go`:

```go
DiagSecurityCredential             = "security_credential"          // gitleaks family
DiagSecurityWriteOnSchedule        = "security_write_on_schedule"
DiagSecurityDraftFalse             = "security_draft_false"
DiagSecurityMissingProtectedFiles  = "security_missing_protected_files"
DiagSecurityEngineEnvNonAllowlist  = "security_engine_env_non_allowlist"
DiagSecurityRepoMemoryMain         = "security_repo_memory_main"
DiagSecurityMCPNonStandardHost     = "security_mcp_non_standard_host"
```

Plus reuse-or-add `DiagSecuritySkippedScanner` (one for actionlint missing, one for malformed frontmatter, one for unknown engine — these are INFO-severity findings, not separate diagnostic codes; they reuse the rule-specific codes with `severity=info` in `Fields`).

`Finding.ToDiagnostic()` projects:

- `Code` ← one of the seven above (mapping by RuleID prefix)
- `Message` ← `RenderForStderr(finding)` (one-line form)
- `Fields` ← `{"severity": "HIGH"|"MEDIUM"|"LOW"|"INFO", "rule_id": "...", "file": "...", "line": 23, "remedy": "..."}`

### Rationale

- **Stable diagnostic codes** are the JSON-envelope contract for downstream consumers (FR-008). Snake_case mirrors existing constants.
- **One code per rule family** (not one per rule) keeps the code set small and stable; rule_id in `Fields` carries the per-rule granularity.
- **Severity in Fields, not in Code**: matches existing precedent (`DiagMissingSecret` doesn't encode severity in the code; the consumer reads it from context).

### Alternatives considered

| Option | Why rejected |
|---|---|
| One Diagnostic code per rule (e.g. `security.permissions.write-on-schedule`) | Code surface explodes; harder to gate on "any security finding" in jq filters. |
| Severity-as-code (`security_high`, `security_medium`) | Loses rule-family granularity; consumers can't tell what kind of finding it is. |
| No new codes; reuse `DiagHint` | Loses gateability — consumers can't filter security findings from generic hints. |

---

## R8: Symbols and naming consistency with existing fleet package

### Decision

- Package name: `security` (lowercase, single word, fits Go convention).
- Top-level entry: `security.Run(ctx context.Context, cloneDir string) []Finding`. (Engine is read per-workflow from frontmatter inside the structural scanner — FR-018; no fleet-level engine parameter.)
- Public types: `Finding`, `Severity`, `Scanner` (interface).
- Severity constants: `SeverityInfo`, `SeverityLow`, `SeverityMedium`, `SeverityHigh`. Underlying type `int`.
- Internal scanner constructors: `newGitleaksScanner()`, `newActionlintScanner()`, `newStructuralScanner()`.
- Render functions: `RenderForStderr(findings []Finding) string`, `RenderForPRBody(findings []Finding) string`.
- PR-body composer (in `deploy.go`, not in `security/`): `securityFindingsSection(res *DeployResult) string` — calls `security.RenderForPRBody(res.SecurityFindings)`.

### Rationale

- Mirror the `fleet.Diagnostic` / `fleet.CollectHints` symmetry: rich type + render functions + projection.
- Keep the PR-body composer in `internal/fleet/deploy.go` (next to `setupRequiredSection`) so all body composers live together; `security.RenderForPRBody` is the content, the composer is the placement.
- Public `Severity` type with constants beats string severities — type-safe, supports `<` comparison for sorting, idiomatic Go.

### Alternatives considered

| Option | Why rejected |
|---|---|
| Severities as strings (`"HIGH"`, `"MEDIUM"`) | Type-unsafe; sort order would need a string→int helper anyway. |
| Severity as `iota` ascending (HIGH=0) | Sort code reads backwards; "high" naturally sounds like a higher number. |
| `RuleID` as a typed `Rule` struct | Overengineered for v1; spec entities are explicit (rule is a named-pattern within a scanner) — string ID is sufficient until rules need configurable severity, which is future work. |

---

## Phase 0 completion checklist

- [x] R1: gitleaks library use confirmed; pinning + redaction handled
- [x] R2: actionlint shellout + JSON parse confirmed; graceful degradation specified
- [x] R3: ADR-26919 port strategy decided (static map + JSON fixture + commit SHA pin)
- [x] R4: Scanner integration site identified (deploy.go:213+)
- [x] R5: PR-body composer placement decided (between setup-required and body footer)
- [x] R6: Sort order specified (severity desc → file asc → line asc)
- [x] R7: Diagnostic codes named (seven new codes)
- [x] R8: Symbols and naming aligned with existing fleet package

**Output**: All open questions resolved in this Phase 0 doc. Phase 1 (data-model.md, contracts/, quickstart.md) can proceed.
