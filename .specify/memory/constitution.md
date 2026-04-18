<!--
SYNC IMPACT REPORT
Version change: TEMPLATE (placeholders) → 1.0.0
Rationale: Initial ratification; establishes governance for the project for the first time.

Principles established:
  I.   Thin-Orchestrator Code Quality
  II.  Testing Standards (Build-Green + Real-World Dry-Run)
  III. User Experience Consistency (Three-Turn Mutation Pattern)
  IV.  Performance Requirements (Parallelism, Cache, 5-Minute Ceiling)

Sections added:
  - Declarative Reconcile Invariants
  - Development Workflow
  - Governance

Sections removed: none (first ratification).

Templates requiring updates:
  ✅ .specify/templates/constitution-template.md — source template; left intact (it is a scaffold, not a mirror).
  ✅ .specify/templates/plan-template.md — has a generic "Constitution Check" slot that resolves to these principles at plan time; no hardcoded references to prior principles.
  ✅ .specify/templates/spec-template.md — no constitution references detected.
  ✅ .specify/templates/tasks-template.md — no constitution references detected.
  ⚠ .specify/templates/commands/*.md — directory does not exist; nothing to audit.
  ✅ CLAUDE.md — already encodes the operational subset (gpg signing, git-from-Bash, conventional commits, dry-run default). Principles here subsume and generalize it.
  ✅ README.md — describes architecture; no conflicts with new principles.

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

## Development Workflow

- Proposed changes enter via PRs that include at minimum: `go build`/`go vet` clean, and for behavior changes, evidence from a dry-run or a subagent test of an affected skill.
- The four skills in `skills/` MUST be updated when a command they reference gains or loses a flag, when a new failure class surfaces a new `CollectHints` pattern, or when the three-turn flow materially changes.
- `CLAUDE.md` MUST be updated when a new architectural invariant is established (e.g., a new source repo added to `SourceLayout`, a new loader precedence rule).
- Release cadence is continuous on the tool's own `main` branch. Fleet deployments use tagged source refs (`v0.68.3` etc.) via `fleet.json`; this is independent of the tool's own versioning.
- Complexity MUST be justified: added abstractions, new dependencies, and new deny/allow entries in `.claude/settings.json` require a one-line rationale in the PR description.

## Governance

This constitution supersedes ad-hoc practices. Conflicts between a principle and a prevailing habit resolve by amending the constitution (or by following the principle) — not by exceptions.

Amendments require a PR modifying this document with an explicit version bump, following semantic versioning:

- **MAJOR**: Principle removal, principle redefinition that contradicts a prior version, or governance procedure change.
- **MINOR**: New principle, new section, or material expansion of existing guidance.
- **PATCH**: Clarifications, wording, typo fixes, non-semantic refinements.

Every PR that touches a principle-implicated file SHOULD note which principles are implicated in the PR description. PR reviewers SHOULD check for constitutional violations and either request fixes or (in unusual cases) propose an amendment alongside the change.

Layering:

- `README.md` — audience: humans. Describes what the project does and how to run it.
- `CLAUDE.md` — audience: AI agents working in this repo. Encodes the operational subset (commands, invariants, non-obvious patterns).
- This constitution — audience: anyone proposing change. Cross-cutting policy, versioned, amendable.

When the three disagree, the constitution wins, followed by CLAUDE.md, followed by README.md. Disagreements surface as amendment proposals.

**Version**: 1.0.0 | **Ratified**: 2026-04-18 | **Last Amended**: 2026-04-18
