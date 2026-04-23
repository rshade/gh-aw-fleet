# Quickstart: CLI JSON Output Mode

**Feature**: 003-cli-output-json
**Audience**: Operators and agent-pipeline authors who want to consume `gh-aw-fleet` output programmatically.

This walkthrough shows how to invoke `-o json` on each of the four supported commands, how to pipe the output through `jq`, and how to interpret the envelope. All commands are non-destructive (dry-runs) — no `--apply` is involved.

---

## Prerequisites

- `gh-aw-fleet` built from this branch: `go build ./...` (run from repo root).
- `jq` installed (most distros have it; on macOS: `brew install jq`).
- A `fleet.local.json` or `fleet.json` with at least one declared repo. (`go run . list` without JSON mode will confirm you have one.)

---

## 1. `list -o json` — the simplest case

```bash
go run . list -o json 2>/dev/null | jq .
```

Expected output (pretty-printed by `jq`):

```json
{
  "schema_version": 1,
  "command": "list",
  "repo": "",
  "apply": false,
  "result": {
    "loaded_from": "fleet.local.json",
    "repos": [
      {
        "repo": "rshade/gh-aw-fleet",
        "profiles": ["default"],
        "engine": "claude",
        "workflows": ["issue-triage", "ci-failure-doctor"],
        "excluded": [],
        "extra": []
      }
    ]
  },
  "warnings": [],
  "hints": []
}
```

**What to notice**:
- `schema_version: 1` — the only contract version byte. Check it first in your agent.
- `command: "list"` — canonical form.
- `repo: ""` — `list` is not scoped to a repo.
- `result.loaded_from` tells you which config file was active (matches the `(loaded fleet.local.json)` breadcrumb, now on stderr).
- `warnings: []` and `hints: []` — arrays, never null, even when empty.
- `2>/dev/null` suppresses the stderr breadcrumb so you see only the envelope. In production you'd keep stderr to capture warnings live.

### Extract all tracked repos

```bash
go run . list -o json 2>/dev/null | jq -r '.result.repos[].repo'
```

Outputs one repo name per line — much easier than regex-scraping the tabwriter table.

---

## 2. `deploy <repo> -o json` (dry-run)

Pick a repo from your fleet and run:

```bash
go run . deploy rshade/gh-aw-fleet -o json 2>/dev/null | jq .
```

Expected (shortened):

```json
{
  "schema_version": 1,
  "command": "deploy",
  "repo": "rshade/gh-aw-fleet",
  "apply": false,
  "result": {
    "repo": "rshade/gh-aw-fleet",
    "clone_dir": "/tmp/gh-aw-fleet-12345",
    "added": [
      { "name": "issue-triage", "spec": "githubnext/agentics/issue-triage@main", "reason": "added", "error": "" }
    ],
    "skipped": [],
    "failed": [],
    "init_was_run": false,
    "branch_pushed": "",
    "pr_url": "",
    "missing_secret": "",
    "secret_key_url": ""
  },
  "warnings": [],
  "hints": []
}
```

**Notice**:
- `apply: false` — this was a dry-run.
- `added[]` has one entry; `skipped[]` and `failed[]` are empty arrays, not null.
- `clone_dir` points to the scratch clone — still present on disk after the dry-run for inspection (preserved per constitution III).
- `missing_secret` is `""` — the engine secret is present. If it were missing, a `missing_secret` diagnostic would appear in `warnings[]` AND on stderr.

### Trigger a missing-secret warning (demo)

Run against a repo that you know does not have `ANTHROPIC_API_KEY` set as an Actions secret:

```bash
go run . deploy some-test-repo -o json | jq '.warnings'
```

Expected output includes:

```json
[
  {
    "code": "missing_secret",
    "message": "Actions secret ANTHROPIC_API_KEY is missing on some-test-repo. Workflows using the claude engine will fail at runtime.",
    "fields": { "secret": "ANTHROPIC_API_KEY", "url": "https://github.com/some-test-repo/settings/secrets/actions" }
  }
]
```

The same warning also appeared on stderr via zerolog — humans and machines both get it.

---

## 3. `sync <repo> -o json`

```bash
go run . sync rshade/gh-aw-fleet -o json 2>/dev/null | jq .
```

Expected shape:

```json
{
  "schema_version": 1,
  "command": "sync",
  "repo": "rshade/gh-aw-fleet",
  "apply": false,
  "result": {
    "repo": "rshade/gh-aw-fleet",
    "clone_dir": "/tmp/gh-aw-fleet-67890",
    "missing": [],
    "drift": ["orphan-workflow"],
    "expected": ["issue-triage", "ci-failure-doctor"],
    "deploy": null,
    "pruned": [],
    "deploy_preflight": { "...": "nested DeployResult" }
  },
  "warnings": [
    { "code": "drift_detected", "message": "1 drift workflow(s) found", "fields": { "drift": ["orphan-workflow"] } }
  ],
  "hints": []
}
```

**Notice**:
- `deploy` is `null` because this was a dry-run with no `--apply` (so no actual deploy was triggered).
- `deploy_preflight` is a nested `DeployResult` object — useful for spotting compile failures during dry-run.
- `drift[]` lists workflow files present on the remote but not declared in `fleet.json`.
- `warnings[]` has a `drift_detected` diagnostic with the drift list in structured fields.

### Filter only drifted repos

```bash
go run . sync rshade/gh-aw-fleet -o json 2>/dev/null \
  | jq 'select(.result.drift | length > 0) | {repo, drift: .result.drift}'
```

---

## 4. `upgrade <repo> -o json` (single repo)

```bash
go run . upgrade rshade/gh-aw-fleet -o json 2>/dev/null | jq '.result | {upgrade_ok, changed_files, pr_url}'
```

Expected:

```json
{
  "upgrade_ok": true,
  "changed_files": [".github/workflows/issue-triage.lock.yml"],
  "pr_url": ""
}
```

On dry-run, `pr_url` is empty; with `--apply` (and user approval), it would be the GitHub PR URL.

### Extract raw audit JSON

```bash
go run . upgrade rshade/gh-aw-fleet -o json 2>/dev/null | jq '.result.audit_json'
```

The audit JSON is a **native nested object** (not a stringified blob). You can descend into it directly with `jq '.result.audit_json.findings[]'` without an intermediate `fromjson`.

---

## 5. `upgrade --all -o json` (NDJSON stream)

This is the streaming case. Each repo's envelope is one line:

```bash
go run . upgrade --all -o json 2>/dev/null
```

Example raw output (3-repo fleet):

```text
{"schema_version":1,"command":"upgrade","repo":"rshade/gh-aw-fleet","apply":false,"result":{...},"warnings":[],"hints":[]}
{"schema_version":1,"command":"upgrade","repo":"rshade/repo-b","apply":false,"result":null,"warnings":[],"hints":[{"code":"hint",...}]}
{"schema_version":1,"command":"upgrade","repo":"rshade/repo-c","apply":false,"result":{...},"warnings":[],"hints":[]}
```

### Process line-by-line with `jq -c`

```bash
go run . upgrade --all -o json 2>/dev/null | jq -c 'select(.result == null) | {repo, error: .hints[0].message}'
```

Streams — each repo's envelope is printed as it completes, so you see failures in real time.

### Aggregate to a single object (optional)

```bash
go run . upgrade --all -o json 2>/dev/null | jq -s '{schema_version:1,command:"upgrade_all",repos: .}'
```

`jq -s` (`--slurp`) reads the whole stream into an array and wraps it. Loses streaming; useful when you want a single JSON object for a downstream tool that can't handle NDJSON.

---

## 6. Error envelopes

Try a repo that's not in your fleet:

```bash
go run . deploy nonexistent/repo -o json; echo "exit=$?"
```

Expected stdout:

```json
{"schema_version":1,"command":"deploy","repo":"nonexistent/repo","apply":false,"result":null,"warnings":[],"hints":[{"code":"hint","message":"Repo \"nonexistent/repo\" is not declared in fleet.json; add it with `gh-aw-fleet add nonexistent/repo` first.","fields":{"hint":"..."}}]}
exit=1
```

**Notice**:
- `result: null` — command did not produce a result.
- `hints[]` carries the actionable error.
- Exit code is non-zero.
- `jq -e .` would still succeed on this output — the envelope is valid JSON.

---

## 7. Invalid output mode

```bash
go run . list -o yaml
```

Expected stderr:

```text
Error: unsupported output mode "yaml": expected one of: text, json
```

Exit code: 1. Stdout is empty — NOT a JSON envelope, because flag validation happens before serialization.

Agents consuming `-o json` SHOULD spell the flag exactly — typos surface immediately as flag errors, not as silent text-mode output.

---

## 8. Text mode is byte-identical

```bash
diff <(go run . list) <(go run . list -o text)
```

Expected: empty diff. `-o text` is the default; specifying it explicitly changes nothing.

And:

```bash
# Pre-feature output (captured before this branch landed):
go run . list > /tmp/before.txt

# After this branch:
go run . list > /tmp/after.txt
diff /tmp/before.txt /tmp/after.txt
```

Expected: empty diff. FR-014 / SC-003 are load-bearing for this feature — any diff here is a regression.

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `jq: parse error: Invalid numeric literal` on `gh-aw-fleet <cmd> -o json \| jq .` | Stderr leaked into stdout (e.g., `2>&1` was in the pipeline) | Redirect stderr explicitly: `cmd -o json 2>/dev/null \| jq .` |
| `result: null` unexpectedly | Command failed before building its result. Inspect `.hints[0].message` and `.warnings[0].message`. | Fix the underlying issue; the hint usually tells you how. |
| All slice fields show `null` instead of `[]` | You're running a broken build — `initSlices` helper isn't firing. | Rebuild from `main` and re-run. This would be a regression; report it. |
| Unexpected `schema_version` (not `1`) | You're running a future version that bumped the schema. | Read `CHANGELOG.md` under the `### JSON envelope schema` section for the breaking changes. |
