# Contract: Public Go Package `github.com/rshade/gh-aw-fleet/pkg/fleet`

For a CLI/library project the "interface contract" is the **exported Go API
surface** of the new package. This is the stable promise the control-plane module
(#142/#143) imports against. Every identifier below MUST exist, be exported, and
carry a godoc comment (FR-010, SC-005).

## Import path

```go
import "github.com/rshade/gh-aw-fleet/pkg/fleet"
```

Package clause: `package fleet`. Distinguished from `internal/fleet` (also
`package fleet`) by import path only.

## Exported surface (declarative signature contract)

```go
// Package fleet defines the public, importable fleet.json wire contract:
// the config-data shapes, the on-disk schema version, and their JSON
// serialization. Load/merge/validate/analysis logic lives in the engine's
// internal package, not here.
package fleet

// SchemaVersion is the on-disk fleet-config (fleet.json / fleet.local.json)
// format version written into Config.Version. Distinct from the CLI output
// envelope version. Value: 1.
const SchemaVersion = 1

type Config struct {
    Version    int                 `json:"version"`
    Defaults   Defaults            `json:"defaults,omitzero"`
    Profiles   map[string]Profile  `json:"profiles,omitempty"`
    Repos      map[string]RepoSpec `json:"repos"`
    LoadedFrom string              `json:"-"`
}

// EffectiveEngine returns the engine for repo, preferring the per-repo
// override over the fleet-level default.
func (c *Config) EffectiveEngine(repo string) string

type Defaults struct {
    Engine string `json:"engine,omitempty"`
}

type Profile struct {
    Description string               `json:"description,omitempty"`
    Tier       string               `json:"tier,omitempty"`
    Sources    map[string]SourcePin `json:"sources"`
    Workflows  []ProfileWorkflow    `json:"workflows"`
}

type SourcePin struct {
    Ref string `json:"ref"`
}

type ProfileWorkflow struct {
    Name   string `json:"name"`
    Source string `json:"source"`
    Path   string `json:"path,omitempty"`
}

type RepoSpec struct {
    Profiles            []string          `json:"profiles"`
    CostCenter          string            `json:"cost_center,omitempty"`
    Engine              string            `json:"engine,omitempty"`
    CompileStrict       *bool             `json:"compile_strict,omitempty"`
    ExtraWorkflows      []ExtraWorkflow   `json:"extra,omitempty"`
    ExcludeFromProfiles []string          `json:"exclude,omitempty"`
    Overrides           map[string]string `json:"overrides,omitempty"`
}

type ExtraWorkflow struct {
    Name   string `json:"name"`
    Source string `json:"source"`
    Ref    string `json:"ref,omitempty"`
    Path   string `json:"path,omitempty"`
}
```

## Contract guarantees (asserted by tests)

| ID | Guarantee | Verified by |
|----|-----------|-------------|
| C-1 | All seven types + `SchemaVersion` are importable from outside `internal/` and compile. | `package fleet_test` black-box test (SC-001) |
| C-2 | `SchemaVersion == 1`. | black-box assertion (FR-003) |
| C-3 | `(&Config{...}).EffectiveEngine(repo)` returns per-repo override else default. | black-box behavior test (FR-004) |
| C-4 | Marshal/unmarshal is byte-identical to the canonical baseline for `fleet.example.json` data. | golden round-trip vs. `testdata/config.canonical.json` (SC-002, FR-005/FR-006) |
| C-5 | `LoadedFrom` (`json:"-"`) never appears in serialized output. | golden round-trip + explicit absence assertion (FR-005, spec US2 #3) |
| C-6 | `omitzero` (Defaults) / `omitempty` (Profiles, RepoSpec slices) / no-omit (Repos→`null`) all preserved. | golden round-trip covering each case (spec Edge Cases) |
| C-7 | Every exported identifier has a godoc comment. | `make lint` (revive/staticcheck), SC-005 |

## NON-contract (explicitly NOT exported by this package)

These remain `internal/fleet` and MUST NOT be importable via `pkg/fleet`
(FR-008/FR-015/FR-016, one-way-dependency FR-013):

- `EffectiveCompileStrict` (now a standalone func in `internal/fleet`),
  `ghRepoVisibility`, `truncateReason`, `CompileStrictSource*`, `VisibilityPublic`.
- `Templates`, `TemplateSource`, `TemplateWorkflow`, `Evaluation`.
- `LoadConfig`, `mergeConfigs`, `ResolveRepoWorkflows`, `ResolvedWorkflow`.

## Compatibility / stability

- The on-disk format does not change; `SchemaVersion` stays `1` (FR-011).
- No new `go.mod` require entry; the package is stdlib-only (FR-014, SC-006).
- The engine module takes on no dependency on the control plane (FR-013).
