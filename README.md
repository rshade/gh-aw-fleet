# gh-aw-fleet

Declarative fleet manager for GitHub Agentic Workflows.

## Status

Early work-in-progress. The core reconcile loop (`deploy`, `sync`, `upgrade`) is functional. Commands `add` and `status` are stubs; `template fetch` works but is incomplete. Architecture and APIs are actively evolving.

## Why

When you manage 3+ repositories that deploy agentic workflows—markdown-authored workflows that compile to GitHub Actions and run AI agents—keeping them consistent becomes tedious. Each repo independently pins a version of `gh-aw`, drifts on its own, and requires manual sync cycles. `gh-aw-fleet` centralizes that: a single `fleet.json` declares the desired state (which repos get which workflows), and `deploy`, `sync`, and `upgrade` commands bring each repo in line via `gh aw add/update/upgrade` under the hood.

## How It Works

**1. gh-aw ships the compiler; `githubnext/agentics` ships the library.** `gh-aw` is the markdown→GitHub Actions compiler and an internal dogfooding testbed. The curated, reusable workflows live in the `githubnext/agentics` repository. The `default` profile sources workflows from agentics whenever an equivalent exists, falling back to gh-aw only for workflows that don't exist elsewhere (currently: `audit-workflows`, `docs-noob-tester`, `mergefest`).

**2. The tool is a thin orchestrator.** It never rewrites workflow markdown. It only invokes `gh aw add` (which writes the `source:` frontmatter pin), `gh aw upgrade` (bumps actions + recompiles), `gh aw update` (pulls upstream + 3-way merge), and git/gh for branching, commits, and PRs. All value comes from answering *who gets what workflow, when, and from which profile*—not from file manipulation.

**3. Pin-per-profile.** Each profile declares a `sources` map keyed by upstream repository (e.g., `github/gh-aw`, `githubnext/agentics`), each with its own ref. `default` pins `github/gh-aw@v0.68.3` (a tagged release for stability) and `githubnext/agentics@main` (the library's default branch). A profile advances atomically: bumping one source ref re-pins every workflow in that profile sourced from that repo.

**4. Declarative reconcile.** `fleet.json` is the source of truth. Commands like `deploy`, `sync`, and `upgrade` compute diffs between desired and actual state, then apply them. Edits to `fleet.json` become commits; the tool brings reality in line.

## Quick Start

### Prerequisites

- Go 1.25+
- `gh` CLI authed to your GitHub instance
- `gh aw` extension installed: `gh extension install github/gh-aw`

### Build

```bash
go build -o gh-aw-fleet .
```

### Example Workflow

List tracked repos and their resolved workflow sets:

```bash
./gh-aw-fleet list
```

Fetch the upstream catalog (templates.json) so you can review new workflows:

```bash
./gh-aw-fleet template fetch
```

Dry-run a deploy to one repo (shows what would be added, no changes made):

```bash
./gh-aw-fleet deploy acme/widgets
```

Apply the deploy (commits, pushes, opens a PR in the target repo):

```bash
./gh-aw-fleet deploy acme/widgets --apply
```

Reconcile a repo (add missing workflows, flag unexpected ones):

```bash
./gh-aw-fleet sync HavenTrack/goa-service-shared --apply
```

Upgrade all repos to the latest gh-aw and agentics versions:

```bash
./gh-aw-fleet upgrade --all --apply
```

## Commands

| Command | Description |
|---------|-------------|
| `list` | List tracked repos and their resolved workflow sets |
| `deploy <repo>` | Apply the declared workflow set to a repo via `gh aw add` + PR |
| `sync <repo>` | Reconcile a repo to match its declared profile (add missing, flag drift) |
| `upgrade [repo\|--all]` | Bump profile pins + run `gh aw upgrade` + update across repos |
| `template fetch` | Refresh `templates.json` from gh-aw and agentics |
| `status [repo]` | Diff desired (fleet.json) vs actual state (not yet implemented) |
| `add <owner/repo>` | Register a repo in fleet.json with a profile (not yet implemented) |

## Configuration

### fleet.json

The declarative desired state. Top-level keys:

- **`version`**: Schema version (currently `1`).
- **`defaults`**: Fleet-wide defaults. Currently: `engine` (e.g., `"copilot"` for all repos unless overridden).
- **`profiles`**: Named bundles of workflows. See "Profiles Bundled" below.
- **`repos`**: Per-repo desired state (profiles, excludes, extras, overrides).

Example fragment:

```json
{
  "version": 1,
  "defaults": {
    "engine": "copilot"
  },
  "profiles": {
    "default": {
      "description": "Baseline every tracked repo gets.",
      "sources": {
        "github/gh-aw": { "ref": "v0.68.3" },
        "githubnext/agentics": { "ref": "main" }
      },
      "workflows": [
        { "name": "audit-workflows", "source": "github/gh-aw" },
        { "name": "ci-doctor", "source": "githubnext/agentics" }
      ]
    }
  },
  "repos": {
    "acme/widgets": {
      "profiles": ["default"],
      "extra": [
        { "name": "shadow-engineer", "source": "local", "path": ".github/workflows/shadow-engineer.md" }
      ]
    }
  }
}
```

### RepoSpec

Per-repo configuration. Keys:

- **`profiles`** (required): List of profile names to apply.
- **`engine`** (optional): AI engine override (e.g., `"gpt-4"`). Defaults to `defaults.engine`.
- **`extra`** (optional): Additional workflows not from any profile. Source can be `"local"` (lives in the target repo) or an upstream repo name.
- **`exclude`** (optional): List of workflow names to skip (useful when a profile mostly fits but one or two workflows don't).
- **`overrides`** (optional): Map of workflow name → custom path (if the upstream path doesn't match convention).

## Profiles Bundled

### default

Baseline every tracked repo gets. Low-noise, broadly useful. Pulls 12 workflows: 3 from gh-aw (`audit-workflows`, `docs-noob-tester`, `mergefest`) and 9 from agentics (`ci-doctor`, `code-simplifier`, `daily-doc-updater`, `daily-malicious-code-scan`, `pr-fix`, `weekly-issue-summary`, `issue-arborist`, `sub-issue-closer`, `dependabot-pr-bundler`).

### quality-plus

PR-generating quality agents. Noisier—opt in for actively-developed repos. Adds 3 agentics workflows: `daily-test-improver`, `daily-perf-improver`, `repository-quality-improver`.

### security-plus

SAST, secret scanning, and outbound-traffic audits. Layered on top of `default`'s `daily-malicious-code-scan`. Adds 3 gh-aw workflows: `daily-semgrep-scan`, `daily-secrets-analysis`, `daily-firewall-report`.

### docs-plus

Heavier docs maintenance. Assumes the repo has a real docs site. Adds 6 agentics workflows: `glossary-maintainer`, `unbloat-docs`, `update-docs`, `link-checker`, `markdown-linter`, `daily-multi-device-docs-tester`.

### community-plus

Contributor-facing helpers. All command-triggered or event-scoped, dormant when idle. Adds 4 agentics workflows: `grumpy-reviewer`, `pr-nitpick-reviewer`, `archie`, `repo-ask`.

## templates.json

Upstream catalog cache. Populated by `template fetch`. Lists every workflow available from each source (gh-aw, agentics), with parsed frontmatter, descriptions, triggers, tools, permissions, and full body text. This is reviewable inline—humans or LLMs can evaluate new workflows and diffs without re-fetching from GitHub. When `template fetch` detects new or changed workflows, it suggests pointing Claude at `templates.json` for evaluation and profile recommendations.

## Design Choices

**Thin orchestrator, not re-implementation.** The tool delegates workflow compilation to `gh aw`, not a homegrown parser. This means improvements to `gh aw` flow through automatically; there's no separate maintenance burden.

**Asymmetry in deploy vs upgrade.** `deploy` uses the pins from `fleet.json`. `upgrade` follows the workflow's own frontmatter pin (the `source:` field written by `gh aw add`), then bumps that. This is intentional—it allows independent per-workflow decisions while `deploy` enforces fleet-wide consistency. Document this.

**Dry-run by default.** `deploy`, `sync`, and `upgrade` default to dry-run mode. Pass `--apply` to commit, push, and open PRs. This prevents surprises.

**Conventional Commits.** All generated commits follow the format `ci(workflows): <description>`, making them easy to filter from changelogs.

## Diagnostic Hints

When a command fails (e.g., `gh aw add` returns an error), the tool scans output for known error patterns and surfaces remediation hints. Examples:

- `Unknown property: mount-as-clis` → "This workflow uses an unreleased gh-aw feature. Upgrade your CLI or pin to a tagged release."
- `HTTP 404` → "Check the spec—github/gh-aw workflows live under `.github/workflows/`; githubnext/agentics workflows live under `workflows/`."
- `gpg failed to sign` → "Unlock gpg-agent in your shell and re-run."

## Known Limitations & Roadmap

- **No `--work-dir` persistence.** The `--work-dir` flag exists but doesn't resume from a previous state. A full retry re-clones and re-starts.
- **Sync dry-run doesn't do deploy preflight.** `sync --dry-run` computes the diff but doesn't pre-validate that the workflows would deploy successfully.
- **No GitHub extension packaging yet.** `gh-aw-fleet` is not yet installable as a `gh` extension. Build and invoke directly for now.
- **`add` and `status` are stubs.** These commands are planned but not yet implemented.

## Contributing & Development

Development requires `go build`, `go vet`, `go test`, and `gh aw` commands to work without prompting. The repo's `.claude/settings.json` allows these automatically. Deny rules block destructive git operations (`git add`, `git commit`, `git rebase --continue`, `git reset --hard`, `git push --force`).

To build and verify:

```bash
go build ./...
go vet ./...
go test ./...
```

All tests must pass before submitting changes.
