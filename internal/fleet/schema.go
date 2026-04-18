package fleet

import "time"

const SchemaVersion = 1

// Config is the declarative desired state for the fleet.
// Loaded from fleet.local.json (if present) or fleet.json. Edits to this file are the source of truth;
// `fleet sync` and `fleet upgrade` reconcile repos toward it.
type Config struct {
	Version    int                 `json:"version"`
	Defaults   Defaults            `json:"defaults"`
	Profiles   map[string]Profile  `json:"profiles"`
	Repos      map[string]RepoSpec `json:"repos"`
	LoadedFrom string              `json:"-"`
}

// Defaults are applied to every repo unless overridden in RepoSpec.
type Defaults struct {
	Engine string `json:"engine,omitempty"`
}

// EffectiveEngine returns the engine for a repo, preferring per-repo override
// over fleet-level default.
func (c *Config) EffectiveEngine(repo string) string {
	if spec, ok := c.Repos[repo]; ok && spec.Engine != "" {
		return spec.Engine
	}
	return c.Defaults.Engine
}

// Profile is a named bundle of workflows pulled from one or more upstream
// source repositories. A profile advances atomically: bumping Sources[x].Ref
// re-pins every workflow in the profile sourced from x.
type Profile struct {
	Description string               `json:"description,omitempty"`
	Sources     map[string]SourcePin `json:"sources"`
	Workflows   []ProfileWorkflow    `json:"workflows"`
}

// SourcePin records the ref (tag/branch/sha) for a given source repo within
// a profile. Keyed by "owner/repo" in the enclosing map.
type SourcePin struct {
	Ref string `json:"ref"`
}

// ProfileWorkflow names a workflow and which source repo provides it.
// The actual ref comes from Profile.Sources[Source].Ref at deploy time.
type ProfileWorkflow struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Path   string `json:"path,omitempty"`
}

// RepoSpec is the desired state for a single target repo.
type RepoSpec struct {
	Profiles            []string          `json:"profiles"`
	Engine              string            `json:"engine,omitempty"`
	ExtraWorkflows      []ExtraWorkflow   `json:"extra,omitempty"`
	ExcludeFromProfiles []string          `json:"exclude,omitempty"`
	Overrides           map[string]string `json:"overrides,omitempty"`
}

// ExtraWorkflow is a per-repo workflow not from any profile. Source "local"
// means it lives in the target repo and is not re-deployed from elsewhere.
type ExtraWorkflow struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Ref    string `json:"ref,omitempty"`
	Path   string `json:"path,omitempty"`
}

// Templates is the upstream catalog cache (templates.json). Populated by
// `fleet template fetch`. Read-only from the deploy path's perspective —
// it answers "what exists upstream and what does each one do" without
// hitting the network on every command.
type Templates struct {
	Version     int                       `json:"version"`
	FetchedAt   time.Time                 `json:"fetched_at"`
	Sources     map[string]TemplateSource `json:"sources"`
	Evaluations map[string]Evaluation     `json:"evaluations,omitempty"`
}

// TemplateSource is one upstream source repo's contents at the time of fetch.
type TemplateSource struct {
	RefFetched string             `json:"ref_fetched"`
	Workflows  []TemplateWorkflow `json:"workflows"`
}

// TemplateWorkflow is a single upstream workflow with enough fidelity that
// a reviewer (human or LLM-in-chat) can evaluate it and diff versions
// without re-fetching. Includes parsed frontmatter + full body.
type TemplateWorkflow struct {
	Name        string         `json:"name"`
	Path        string         `json:"path"`
	SHA         string         `json:"sha"`
	Description string         `json:"description,omitempty"`
	Engine      string         `json:"engine,omitempty"`
	Triggers    []string       `json:"triggers,omitempty"`
	Tools       []string       `json:"tools,omitempty"`
	SafeOutputs []string       `json:"safe_outputs,omitempty"`
	StopAfter   string         `json:"stop_after,omitempty"`
	Permissions map[string]any `json:"permissions,omitempty"`
	Frontmatter map[string]any `json:"frontmatter,omitempty"`
	Body        string         `json:"body,omitempty"`
	Lines       int            `json:"lines,omitempty"`
}

// Evaluation is Claude's judgment on a workflow — used by `fleet template
// fetch` to flag newly-seen workflows and suggest profile membership.
type Evaluation struct {
	EvaluatedAt time.Time `json:"evaluated_at"`
	Summary     string    `json:"summary"`
	FitProfiles []string  `json:"fit_profiles,omitempty"`
	Recommend   string    `json:"recommend,omitempty"`
}
