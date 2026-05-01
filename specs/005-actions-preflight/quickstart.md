# Quickstart: Deploy Preflight for Actions Enabled and Workflow Write Permissions

**Feature**: `005-actions-preflight` | **Date**: 2026-04-29

This is the operator-facing quickstart. It assumes you already have `gh-aw-fleet` installed, `gh auth status` clean, and a `fleet.json` (and optionally `fleet.local.json`) in the working directory.

---

## What this feature adds

Before this feature, `gh-aw-fleet deploy <repo>` checked one precondition: was the engine secret (e.g., `ANTHROPIC_API_KEY`) present on the target repo or its org? If it was missing, you got a warning.

This feature adds two more checks during the same preflight pass:

1. **GitHub Actions is enabled on the target repo.** A repo where the operator (or an admin) clicked "Disable actions" will silently swallow every workflow you deploy — they sit there inert. With this feature, deploy warns you with a direct link to the settings page so you can fix it before merging.
2. **The `GITHUB_TOKEN` workflow permission is `write`.** Many agentic workflows (`pr-fix`, `code-simplifier`, `weekly-research`) push commits, create reviews, or comment on PRs. If the repo's "Workflow permissions" setting is "Read repository contents and packages permissions" (the default for new repos in many orgs), those workflows execute and then 403 at the first write call. With this feature, deploy warns you up-front and tells you exactly which radio button to flip.

Both warnings fire in **dry-run** and `--apply` modes. Both also appear in the PR body's "Setup required" section under `--apply`, so a reviewer who didn't run the dry-run still sees them.

---

## Three things you'll see

### 1. Healthy repo — no change

```bash
$ gh-aw-fleet deploy alice/widgets
```

If Actions is enabled, the workflow token is `write`, and the engine secret is set, output looks identical to before. No new noise.

### 2. Misconfigured repo — new stderr warnings

```bash
$ gh-aw-fleet deploy alice/widgets

[DRY RUN] alice/widgets (clone: /tmp/gh-aw-fleet-abc123)
  added: 5
    + audit  (github/gh-aw/.github/workflows/audit.md@v0.68.3)
    + ci-doctor  (githubnext/agentics/ci-doctor@main)
    ... (etc)

⚠ WARNING: GitHub Actions is disabled on alice/widgets
  Enable at: https://github.com/alice/widgets/settings/actions

⚠ WARNING: Workflow token is read-only on alice/widgets
  Agentic workflows that push commits or create reviews will fail.
  Fix at: https://github.com/alice/widgets/settings/actions
  Set "Workflow permissions" → "Read and write permissions"

Re-run with --apply to commit, push, and open the PR.
```

The two warnings are independent: one or both can fire alongside (or instead of) the existing missing-secret warning. The order is fixed: Actions → token → secret.

### 3. Restricted-token CI — silent fall-through

When you're running deploy from CI with a token that lacks `admin:repo` (or a similar scope that lets you read repo settings), the API returns 403. Per the fail-open contract (clarification Q3), the preflight skips silently:

```bash
$ GH_TOKEN=$LIMITED_TOKEN gh-aw-fleet deploy alice/widgets
# (output is identical to a healthy repo deploy — no new warnings, no errors)
```

To see what the preflight is doing under the hood, run with `--log-level debug`:

```bash
$ GH_TOKEN=$LIMITED_TOKEN gh-aw-fleet deploy alice/widgets --log-level debug
... (existing debug output) ...
DBG actions-settings preflight skipped repo=alice/widgets endpoint=/repos/alice/widgets/actions/permissions reason=http_403
DBG actions-settings preflight skipped repo=alice/widgets endpoint=/repos/alice/widgets/actions/permissions/workflow reason=http_403
... (rest of deploy) ...
```

This is the only place the skip surfaces. At the default `info` log level, restricted-token deploys are byte-identical to healthy deploys.

---

## How to verify the feature manually

These steps assume you have a test repo you can break and unbreak.

### Verify "Actions disabled" warning

```bash
# 1. Disable Actions on a test repo
gh api -X PUT /repos/<owner>/<test-repo>/actions/permissions -f enabled=false

# 2. Run deploy dry-run
gh-aw-fleet deploy <owner>/<test-repo>

# 3. Confirm the warning fires; copy the URL into your browser to verify it lands on the right settings page

# 4. Re-enable
gh api -X PUT /repos/<owner>/<test-repo>/actions/permissions -f enabled=true
```

### Verify "Workflow token read-only" warning

```bash
# 1. Set the workflow token to read-only
gh api -X PUT /repos/<owner>/<test-repo>/actions/permissions/workflow \
  -f default_workflow_permissions=read

# 2. Run deploy dry-run
gh-aw-fleet deploy <owner>/<test-repo>

# 3. Confirm the warning fires with the consequence sentence and the explicit fix instruction

# 4. Restore
gh api -X PUT /repos/<owner>/<test-repo>/actions/permissions/workflow \
  -f default_workflow_permissions=write
```

### Verify both warnings + PR body integration

```bash
# 1. Misconfigure both
gh api -X PUT /repos/<owner>/<test-repo>/actions/permissions -f enabled=false
gh api -X PUT /repos/<owner>/<test-repo>/actions/permissions/workflow -f default_workflow_permissions=read

# 2. Run with --apply (you'll get a deploy PR)
gh-aw-fleet deploy <owner>/<test-repo> --apply

# 3. Open the resulting PR. Confirm the body contains a single "## ⚠ Setup required"
#    heading with three sub-blocks in order: Actions disabled, token read-only, secret missing
#    (assuming the secret is also missing on the test repo)

# 4. Restore
gh api -X PUT /repos/<owner>/<test-repo>/actions/permissions -f enabled=true
gh api -X PUT /repos/<owner>/<test-repo>/actions/permissions/workflow -f default_workflow_permissions=write
```

---

## How to gate CI on the new findings

If you want a CI pipeline to abort the deploy when settings are misconfigured (regardless of whether you let `gh-aw-fleet --apply` proceed), use the JSON envelope:

```bash
# Bash example: fail the CI step if either new warning fires
result=$(gh-aw-fleet deploy alice/widgets --output json)
if echo "$result" | jq -e '.warnings[] | select(.code == "actions_disabled" or .code == "workflow_token_read_only")' > /dev/null; then
  echo "Repo settings are misconfigured. Fix before merging."
  exit 1
fi
```

The two stable codes (`actions_disabled` and `workflow_token_read_only`) are documented in [contracts/deploy-result.md](./contracts/deploy-result.md). They will not be renamed without a `schema_version` bump.

---

## What's NOT in scope

The feature deliberately stops at *detect and warn*. It does not:

- Automatically toggle the settings on your behalf via the API.
- Check branch-protection rules.
- Check individual workflow `permissions:` blocks (the `permissions:` field inside a `workflow.yml`).
- Check org-level allow-lists (`Settings > Actions > Restrict actions to selected repositories`).

Each of these is a separate feature if demand emerges. Out-of-scope items are listed in the spec.
