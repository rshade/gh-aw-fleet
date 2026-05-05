# Contract: `## Security Findings` PR-body section

**Phase**: 1 (Design & Contracts) | **Spec**: [../spec.md](../spec.md) | **Date**: 2026-04-30

## Purpose

Defines the stable user-facing contract for the PR-body section that surfaces findings to reviewers (User Story 3, FR-005, FR-010). Stability matters: downstream tooling (humans, future automation) may grep for the heading or parse the body.

## Stable invariants

| Invariant | Source | Notes |
|---|---|---|
| Heading text is exactly `## Security Findings` | FR-005 | No emoji, no trailing punctuation. Capitalization fixed. |
| Section is OMITTED ENTIRELY when zero findings exist | FR-005 | No "no findings" placeholder, no empty heading. |
| Summary line begins with `**Summary**: ` (bold-asterisk + colon-space) | FR-010 | Format change is breaking. |
| Severity tally in summary line is in descending order: `HIGH, MEDIUM, LOW, INFO` | FR-011 + R6 | Severities with zero count are omitted (e.g. all-INFO output → `**Summary**: 1 INFO`). |
| Per-finding bullets are markdown unordered list items beginning with `- **<SEVERITY>**` | This contract | Reviewers / tools rely on bullet structure for scanning. |
| Per-finding fields are em-dash separated: `severity — rule_id — file:line — message — remedy` | This contract | Em-dash (`—`, U+2014) chosen because it doesn't conflict with rule IDs containing dots, hyphens, or colons. |
| Findings are listed in the same sort order as the slice | R6 | Severity desc → file asc → line asc. |
| Section is positioned AFTER `## ⚠ Setup required` and BEFORE the workflow list / footer | R5 | Setup-required is action-required-before-merge; security is action-recommended-during-review. |

## Rendering examples

### Single HIGH finding (typical)

```markdown
## Security Findings

**Summary**: 1 HIGH

- **HIGH** `gitleaks:aws-access-key` — `.github/workflows/foo.md:23` — AWS Access Key (<redacted>) — Rotate the credential. Remove from source. Use the engine.env / GitHub Actions secrets mechanism to inject at runtime.
```

### Mixed severities

```markdown
## Security Findings

**Summary**: 2 HIGH, 1 MEDIUM, 1 INFO

- **HIGH** `gitleaks:aws-access-key` — `.github/workflows/foo.md:23` — AWS Access Key (<redacted>) — Rotate the credential. Remove from source. Use the engine.env / GitHub Actions secrets mechanism to inject at runtime.
- **HIGH** `fleet.permissions.write-on-schedule` — `.github/workflows/bar.md:5` — Workflow has permissions: contents: write and on: schedule trigger — Schedule-triggered workflows with write permissions are the operational shape of a supply-chain compromise. Restrict permissions to read-only or remove the schedule trigger.
- **MEDIUM** `fleet.safe-outputs.draft-false` — `.github/workflows/baz.md:12` — safe-outputs.create-pull-request.draft is set to false — Use draft: true so PRs require human approval before transitioning to non-draft.
- **INFO** `actionlint:not-installed` — — actionlint binary not found in PATH; compiled-YAML lint scanner skipped — Install actionlint (https://github.com/rhysd/actionlint) for compiled-workflow validation. The fleet runs without it — this is graceful degradation.
```

Note in the INFO line: when `File == ""`, the `file:line` slot renders as empty between the em-dashes (visible as `— —`). When `File != "" && Line == 0`, render as just `file` (no `:0` suffix). When both are populated, render as `file:line`.

### Zero findings (clean run)

The PR body has NO `## Security Findings` heading and no related content. The composer returns the empty string and the caller (`prBody`) does not concatenate anything for this section.

## Position in PR body

```markdown
<existing PR description / summary>

## ⚠ Setup required
<setup blocks if any — see internal/fleet/deploy.go:setupRequiredSection>

## Security Findings
<this contract — see above>

<workflow list, fleet.json refs, etc.>
```

## Test obligations

In `internal/fleet/deploy_test.go`:

1. **TestSecurityFindingsSection_NoFindings**: pass `DeployResult{SecurityFindings: nil}` → assert `securityFindingsSection(res) == ""`.
2. **TestSecurityFindingsSection_SingleHigh**: pass one HIGH finding → assert exact golden render (use a `testdata/security/golden-pr-body-single-high.md` fixture if helpful; inline string compare for v1 simplicity).
3. **TestSecurityFindingsSection_MixedSeverities**: pass HIGH+MEDIUM+INFO → assert summary line tally, sort order, em-dash separators.
4. **TestSecurityFindingsSection_StableSort**: pass findings in scrambled input order → assert output order matches sort contract.
5. **TestPRBodyAppendsSecurityFindings**: pass `DeployResult` with both `MissingSecret` set AND `SecurityFindings` populated → assert PR body contains `## ⚠ Setup required` BEFORE `## Security Findings` (positional invariant).

## Surface boundary

`RenderForPRBody` (in `security/render.go`) generates the body content (everything below the heading). `securityFindingsSection` (in `internal/fleet/deploy.go`) wraps the heading. This split:

- Keeps the `security/` package free of "I know about deploy's PR body" knowledge.
- Lets the heading text live next to other PR-body composers in `deploy.go` for easy auditing.
- Makes `RenderForPRBody` reusable by future surfaces (e.g. an `--output markdown` CLI mode) without dragging the heading along.
