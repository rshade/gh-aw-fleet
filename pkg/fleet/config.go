// Package fleet defines the public, importable fleet.json wire contract:
// the config-data shapes, the on-disk schema version, and their JSON
// serialization. Load/merge/validate/analysis logic lives in the engine's
// internal package, not here.
package fleet

// SchemaVersion is the on-disk fleet-config (fleet.json / fleet.local.json)
// format version written into Config.Version. Distinct from the CLI output
// envelope version.
const SchemaVersion = 1

// Config is the declarative desired state for the fleet.
type Config struct {
	// Version is the on-disk fleet-config schema version.
	Version int `json:"version"`
	// Defaults contains fleet-wide settings applied unless a repo overrides them.
	Defaults Defaults `json:"defaults,omitzero"`
	// Profiles maps profile names to workflow bundles.
	Profiles map[string]Profile `json:"profiles,omitempty"`
	// Repos maps owner/repo names to desired repo state.
	Repos map[string]RepoSpec `json:"repos"`
	// LoadedFrom names the on-disk source that produced this Config.
	LoadedFrom string `json:"-"`
}

// EffectiveEngine returns the engine for repo, preferring the per-repo override
// over the fleet-level default.
func (c *Config) EffectiveEngine(repo string) string {
	if spec, ok := c.Repos[repo]; ok && spec.Engine != "" {
		return spec.Engine
	}
	return c.Defaults.Engine
}

// Defaults are applied to every repo unless overridden in RepoSpec.
type Defaults struct {
	// Engine is the fleet-wide default agentic workflow engine.
	Engine string `json:"engine,omitempty"`
}

// Profile is a named bundle of workflows pulled from one or more upstream
// source repositories.
type Profile struct {
	// Description summarizes the profile's intended workflow bundle.
	Description string `json:"description,omitempty"`
	// Tier is an advisory cost-tier label for fleet reporting.
	Tier string `json:"tier,omitempty"`
	// Sources maps source repositories to the refs used by this profile.
	Sources map[string]SourcePin `json:"sources"`
	// Workflows lists the workflows included in this profile.
	Workflows []ProfileWorkflow `json:"workflows"`
}

// SourcePin records the ref for a source repo within a profile.
type SourcePin struct {
	// Ref is the tag, branch, or commit SHA pinned for a source repo.
	Ref string `json:"ref"`
}

// ProfileWorkflow names a workflow and which source repo provides it.
type ProfileWorkflow struct {
	// Name is the workflow slug.
	Name string `json:"name"`
	// Source is the owner/repo key into the profile's Sources map.
	Source string `json:"source"`
	// Path is an optional source-relative workflow path override.
	Path string `json:"path,omitempty"`
}

// RepoSpec is the desired state for a single target repo.
type RepoSpec struct {
	// Profiles lists the profile names applied to the repo.
	Profiles []string `json:"profiles"`
	// CostCenter is a free-form per-repo budget-attribution label.
	CostCenter string `json:"cost_center,omitempty"`
	// Engine is the repo-specific engine override.
	Engine string `json:"engine,omitempty"`
	// CompileStrict toggles gh-aw strict compilation for this repo.
	CompileStrict *bool `json:"compile_strict,omitempty"`
	// ExtraWorkflows lists per-repo workflows outside applied profiles.
	ExtraWorkflows []ExtraWorkflow `json:"extra,omitempty"`
	// ExcludeFromProfiles lists profile workflow names excluded for this repo.
	ExcludeFromProfiles []string `json:"exclude,omitempty"`
	// Overrides maps profile workflow names to local override paths.
	Overrides map[string]string `json:"overrides,omitempty"`
}

// ExtraWorkflow is a per-repo workflow not sourced from any profile.
type ExtraWorkflow struct {
	// Name is the workflow slug.
	Name string `json:"name"`
	// Source identifies where the workflow comes from.
	Source string `json:"source"`
	// Ref is an optional source ref override.
	Ref string `json:"ref,omitempty"`
	// Path is an optional workflow path override.
	Path string `json:"path,omitempty"`
}
