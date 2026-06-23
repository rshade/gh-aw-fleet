<!--
SYNC IMPACT REPORT
Version change: 1.2.0 -> 1.3.0
Rationale: Adds a documentation-currency obligation. MINOR bump: material
expansion of Development Workflow guidance (a new conditional MUST rule for
user-facing docs) plus the docs/ Starlight site added to the Governance
Layering section. No principle removed or redefined.

Modified principles: none (all four principles unchanged).

Sections added: none.

Sections modified:
  - Development Workflow: new conditional MUST rule requiring `README.md` and
    the docs/ Starlight site be updated in the same change that alters a
    surface they document; internal-only changes are exempt; deliberately
    hidden surfaces MUST NOT be documented; the assessment is recorded in each
    plan.md Documentation Impact gate.
  - Governance / Layering: docs/ Starlight site added alongside `README.md` as
    a human-facing layer; precedence sentence updated to match.

Sections removed: none.

Templates requiring updates:
  ✅ .specify/templates/plan-template.md — added a "Documentation Impact" gate
     after Constitution Check, mirroring the new rule.
  ✅ .specify/templates/constitution-template.md — generic scaffold; no version-
     specific content to mirror.
  ✅ .specify/templates/spec-template.md — no documentation-policy references;
     no changes needed.
  ✅ .specify/templates/tasks-template.md — no documentation-policy references;
     no changes needed.
  ⚠ .specify/templates/commands/*.md — directory does not exist; nothing to
     audit.
  ✅ CLAUDE.md / AGENTS.md — docs-currency is governance, not a new
     architectural invariant or operational command; no update required.
  ✅ README.md — the stale v0.1.0 status line was corrected separately; this
     amendment needs no further README change.

Follow-up TODOs: none.
-->

# gh-aw-fleet Constitution

## Core Principles

### I. Thin-Orchestrator Code Quality

gh-aw-fleet MUST orchestrate upstream tools (`gh aw`, `gh`, `git`) rather than re-implement their logic. Every non-trivial operation wraps an upstream primitive via `exec.Command` or an equivalent subprocess call. The tool never rewrites workflow markdown; it never duplicates compilation, 3-way merge, or repo cloning logic that `gh aw`/`git` already provide.

`go build ./...` and `go vet ./...` MUST exit clean on every commit. Go idioms apply: errors wrapped with `%w`, no unused imports, no silenced returns. Files stay focused — refactor before a single file drifts materially past 300 lines without a clear structural reason. Comments explain WHY (non-obvious constraints, hidden invariants, workarounds for specific bugs) — never restate WHAT well-named code already says.

**Rationale**: Upstream tools evolve independently. Re-implementation creates drift surface: every `gh aw` release would otherwise require us to mirror its behavior. A thin orchestrator inherits upstream improvements automatically and breaks only when the CLI contract changes — which is a stable interface.

### II. Testing Standards (Build-Green + Real-World Dry-Run)

Before any change is considered done:

1. `go build ./...` and `go vet ./...` MUST pass.
2. Mutating commands (`deploy`, `sync`, `upgrade`) MUST exercise a dry-run that runs the actual `gh aw add` / `gh aw upgrade` against a real scratch clone before `--apply`. Pre-flight failures surface via `fleet.CollectHints` hints in the dry-run output.
3. Skills in `skills/*/SKILL.md` MUST be tested in subagents before shipping. At minimum, one realistic user prompt per skill with hard stops before destructive actions.

Unit tests are not currently required by this constitution; the project is early and the integration surface (real `gh aw`, real GitHub) is the test substrate. This MAY change (via amendment) once the tool stabilizes and internal logic grows non-trivial.

**Rationale**: The value of the tool emerges from correct orchestration of external tools — mocking upstream would prove very little. Dry-runs against real repos catch the real failure modes (compile errors, network, auth, path conventions) that unit tests would miss.

### III. User Experience Consistency (Three-Turn Mutation Pattern)

Every command that mutates external state MUST follow the three-turn pattern:

1. **Turn 1 — dry-run**: Run read-only; report the plan and any pre-flight failures with hints.
2. **Turn 2 — user approval**: Wait for explicit go-ahead ("go", "apply", "yes", etc.). No implicit progression.
3. **Turn 3 — apply**: Execute the mutation only when `--apply` is passed and the user approved.

All generated commit messages and PR titles MUST conform to Conventional Commits with scope `ci(workflows)`, subject under 72 characters, no trailing period. Every recoverable failure MUST route through `fleet.CollectHints` so the user sees an actionable remediation alongside the raw error.

Scratch clones at `/tmp/gh-aw-fleet-*` MUST be preserved after `--apply` failure so the user can inspect or resume via `--work-dir`.

**Rationale**: `--apply` pushes branches and opens PRs on external repositories — the blast radius extends beyond the caller. The three-turn pattern makes approval explicit and auditable. Conventional Commits keeps downstream commitlint-enabled repos green. Hints turn dead-ends into next steps.

### IV. Performance Requirements (Parallelism, Cache, 5-Minute Ceiling)

I/O-bound operations that touch independent targets (e.g., per-workflow catalog fetches, per-repo audits) SHOULD be parallelized. Network-derived state (`templates.json`) MUST be cached locally and re-used; commands MUST NOT re-fetch catalog data they already have unless the user explicitly requests a refresh.

No single command invocation SHOULD exceed 5 minutes on a healthy `gh` API session against a typical fleet (≤10 repos, ≤20 workflows each). `fleet template fetch` is a known exception today — it is serial and slated for parallelization; the exception MUST be documented in its command surface until resolved.

**Rationale**: Interactive ergonomics matter. A dry-run that takes 3 minutes because we re-fetched data we have locally is a footgun for iteration. Parallelism is nearly free for `exec.Command` fanout; absence of it is an oversight, not a design choice.

## Declarative Reconcile Invariants

These apply to every command that reads or mutates fleet state:

- `fleet.json` (or its local override `fleet.local.json`) is the source of truth. The tool mutates reality toward fleet state, never the reverse. Any command that would mutate `fleet.json` to reflect reality (e.g., a future `fleet import`) MUST make that direction explicit in its help text.
- `fleet.local.json` MUST NOT be committed. `.gitignore` enforces this; `fleet.json` MUST NOT contain private repo names under any circumstance.
- Commit signing MUST NEVER be bypassed in tool code or in any operator-facing flow. `--no-gpg-sign`, `-c commit.gpgsign=false`, and `git config commit.gpgsign false` are forbidden. When signing fails, the tool surfaces a manual-finish template and exits.
- `git add`, `git commit`, and `git push` from Claude Code's Bash tool are denied at the allowlist level (`.claude/settings.json`). The Go tool invoking git via `exec.Command` inside `Deploy`/`Sync`/`Upgrade` is the only legitimate path.
- `github/gh-aw` sources MUST pin to a tagged release (not `main` or a SHA) in any profile used for production deploys. `githubnext/agentics` MAY pin to `main` until the library tags releases.

## Third-Party Dependencies

Go module direct dependencies stay minimal. New entries in `go.mod`'s top-level `require()` block MUST be evaluated against three alternatives — the Go standard library, vendoring the minimal surface from the upstream package, or delegating the work to one of the orchestrated CLIs (`gh aw`, `gh`, `git`) — before adoption. Adding any new direct dependency requires a constitution amendment to this section (MINOR version bump); removing an approved entry is MAJOR.

Indirect (transitive) dependencies in `go.mod`'s second `require()` block are not governed by this section — they flow from the approved direct deps and from the Go toolchain.

**Approved direct dependencies**:

- `github.com/spf13/cobra` — CLI scaffolding; the standard library `flag` package lacks subcommand and persistent-flag support (grandfathered at v1.0.0 ratification).
- `github.com/rs/zerolog` — Structured stderr logging with level/format flags wired via `internal/log.Configure`; `log/slog` was evaluated but the codebase had already adopted zerolog in #24 before slog reached parity (grandfathered at v1.0.0 ratification).
- `gopkg.in/yaml.v3` — Workflow frontmatter parsing and `templates.json` evaluation handling; required to interop with `gh aw`'s YAML output (grandfathered at v1.0.0 ratification).
- `github.com/zricethezav/gitleaks/v8` — Secrets-scanning rule engine for the Layer 1 security scanner (`internal/security`). Vendoring the rule set was rejected because the upstream catalog updates faster than this repo's release cadence; re-implementing detection logic would re-host complexity that has a maintained home upstream. Adopted in #37; retroactively recorded as approved here.
- `github.com/tailscale/hujson` — Comment-preserving reads and writes of fleet config files (`fleet.json`, `fleet.local.json`, `templates.json`, `profiles/default.json`). The standard `encoding/json` package cannot preserve `//` comments or trailing commas across round-trip edits, which blocks the inline-documentation use case in #73 (operators annotating pin choices, profile rationale, and per-workflow `Evaluation` notes next to the data they describe). Scope: `hujson.Standardize()` runs on the read path before `json.Unmarshal`; `hujson.Patch` runs on the write path to surgically apply edits while preserving operator-authored comments. Vendoring was rejected because the package must stay in sync with `encoding/json` semantics, a maintenance burden best owned upstream.
- `github.com/rshade/ax-go` — Shared Agentic Experience (AX) contracts for import-isolated config parsing/patching and CLI schema discoverability. The standard library was rejected because AX contracts such as error envelopes, discoverability, idempotency, and mode resolution are bespoke rather than built into Go; vendoring was rejected because it would fork the shared, golden-pinned contract DNA; CLI delegation is not applicable because this is gh-aw-fleet's own output and AX layer, not work that `gh aw`, `gh`, or `git` can perform. Scope: gh-aw-fleet consumes only the `config`, `schema`, and transitive stdlib-only `contract` packages so ax-go's OpenTelemetry, gRPC, and protobuf transitive dependencies stay out of this tool's build.

## Development Workflow

- Proposed changes enter via PRs that include at minimum: `go build`/`go vet` clean, and for behavior changes, evidence from a dry-run or a subagent test of an affected skill.
- The four skills in `skills/` MUST be updated when a command they reference gains or loses a flag, when a new failure class surfaces a new `CollectHints` pattern, or when the three-turn flow materially changes.
- `CLAUDE.md` MUST be updated when a new architectural invariant is established (e.g., a new source repo added to `SourceLayout`, a new loader precedence rule).
- User-facing documentation MUST be updated in the same change that alters a surface it documents: `README.md` and the `docs/` Starlight site when a command gains or loses a flag, a subcommand ships or changes behavior, install / configuration / reconcile / consumption guidance drifts, or the published release status changes. Internal-only changes (scanners, loaders, manifest logic) with no user-facing surface carry no documentation obligation. Deliberately hidden surfaces (e.g. the `__schema` command) MUST NOT be documented. Each feature's `plan.md` records this assessment in its **Documentation Impact** gate.
- Release cadence is continuous on the tool's own `main` branch. Fleet deployments use tagged source refs (`v0.68.3` etc.) via `fleet.json`; this is independent of the tool's own versioning.
- Complexity MUST be justified: added abstractions and new deny/allow entries in `.claude/settings.json` require a one-line rationale in the PR description. New direct dependencies require a constitution amendment per the **Third-Party Dependencies** section above — a one-line PR rationale is not sufficient.

## Governance

This constitution supersedes ad-hoc practices. Conflicts between a principle and a prevailing habit resolve by amending the constitution (or by following the principle) — not by exceptions.

Amendments require a PR modifying this document with an explicit version bump, following semantic versioning:

- **MAJOR**: Principle removal, principle redefinition that contradicts a prior version, or governance procedure change.
- **MINOR**: New principle, new section, or material expansion of existing guidance.
- **PATCH**: Clarifications, wording, typo fixes, non-semantic refinements.

Every PR that touches a principle-implicated file SHOULD note which principles are implicated in the PR description. PR reviewers SHOULD check for constitutional violations and either request fixes or (in unusual cases) propose an amendment alongside the change.

Layering:

- `README.md` and the `docs/` Starlight site — audience: humans. Describe what the project does and how to run it.
- `CLAUDE.md` — audience: AI agents working in this repo. Encodes the operational subset (commands, invariants, non-obvious patterns).
- This constitution — audience: anyone proposing change. Cross-cutting policy, versioned, amendable.

When these layers disagree, the constitution wins, followed by CLAUDE.md, followed by the human-facing docs (`README.md` and the `docs/` site). Disagreements surface as amendment proposals.

**Version**: 1.3.0 | **Ratified**: 2026-04-18 | **Last Amended**: 2026-06-22
