# JSON Envelope Contract Delta: Fleet Manifest

**Feature**: `011-fleet-manifest`
**Scope**: Additive changes only. No `cmd.SchemaVersion` bump.

## `gh-aw-fleet status --output json`

### Change: `result.repos[].version_drift` (new field)

Added to every `RepoStatus` object in the `result.repos` array.

**Present when**: `drift_state != "errored"` (i.e., the repo was successfully queried).
**Omitted when**: `drift_state == "errored"` (early failure before manifest fetch ran).

#### Schema

```json
"version_drift": {
  "state": "behind | current | unmanaged",
  "recorded_version": "<string>",
  "expected_version": "<string>"
}
```

#### Field semantics

| Field | Type | Values | Meaning |
|-------|------|--------|---------|
| `state` | string | `"behind"` | Manifest version older than fleet pin. Init refresh will run on next deploy. |
| | | `"current"` | Manifest version matches fleet pin. No refresh needed. |
| | | `"unmanaged"` | No manifest found, or manifest is malformed. Repo has never been fleet-deployed, or was deployed before this feature shipped. |
| `recorded_version` | string | semver tag or `""` | `gh_aw_version` from the repo's fleet-manifest.json. Empty when `state == "unmanaged"`. |
| `expected_version` | string | semver tag or `""` | Fleet's current `github/gh-aw` source pin for this repo. Empty when the repo has no `github/gh-aw`-sourced workflows. |

#### Example responses

**Current** (manifest matches fleet pin):
```json
{
  "repo": "owner/myrepo",
  "drift_state": "aligned",
  "missing": [],
  "extra": [],
  "drifted": [],
  "unpinned": [],
  "error_message": "",
  "version_drift": {
    "state": "current",
    "recorded_version": "v0.79.2",
    "expected_version": "v0.79.2"
  }
}
```

**Behind** (manifest records old version):
```json
{
  "repo": "owner/finfocus",
  "drift_state": "aligned",
  "missing": [],
  "extra": [],
  "drifted": [],
  "unpinned": [],
  "error_message": "",
  "version_drift": {
    "state": "behind",
    "recorded_version": "v0.68.3",
    "expected_version": "v0.79.2"
  }
}
```

**Unmanaged** (no manifest):
```json
{
  "repo": "owner/newrepo",
  "drift_state": "drifted",
  "missing": ["daily-malicious-code-scan"],
  "extra": [],
  "drifted": [],
  "unpinned": [],
  "error_message": "",
  "version_drift": {
    "state": "unmanaged",
    "recorded_version": "",
    "expected_version": "v0.79.2"
  }
}
```

**Errored** (version_drift omitted):
```json
{
  "repo": "owner/inaccessible",
  "drift_state": "errored",
  "missing": [],
  "extra": [],
  "drifted": [],
  "unpinned": [],
  "error_message": "list owner/inaccessible workflows: HTTP 404"
}
```

---

## `gh-aw-fleet deploy --output json`

**No changes.** The manifest write is a side effect inside the clone; it does not add a field
to `DeployResult`. `init_was_run` continues to reflect whether `gh aw init` ran during this
deploy.

---

## `gh-aw-fleet sync --output json`

**No changes** to `SyncResult`. The manifest write propagates through the nested `DeployResult`
(accessed via `result.deploy`), which is unchanged.

---

## Manifest file (not part of fleet tool output envelope)

The manifest file written into managed repos at `.github/aw/fleet-manifest.json` is not part of
the fleet tool's stdout JSON envelope. It is a JSON file in the managed repo's history. Its schema
is versioned implicitly by the `FleetManifest` Go type. Future breaking changes to the manifest
schema would require a schema version field to be added (not currently present — treated as v1
implicitly).

```json
{
  "managed": true,
  "fleet": "rshade/gh-aw-fleet",
  "gh_aw_version": "v0.79.2",
  "cli_version": "v0.79.2",
  "profiles": ["default", "observability-plus"],
  "deployed_at": "2026-06-11T01:39:27Z"
}
```
