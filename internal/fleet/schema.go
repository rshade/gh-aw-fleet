package fleet

import (
	"context"
	"time"
	"unicode/utf8"
)

// effectiveCompileStrictReasonMax bounds the length of the truncated raw
// visibility-lookup error surfaced in the auto-fallback `reason` field so
// large network errors do not bloat structured logs.
const effectiveCompileStrictReasonMax = 200

// CompileStrictSource* are the four valid values for
// DeployResult.CompileStrictSource / UpgradeResult.CompileStrictSource.
// The empty string is intentionally separate — it signals "resolver did
// not run" (early-error path) and is not part of this constant set.
const (
	CompileStrictSourceExplicit     = "explicit"
	CompileStrictSourceAutoPublic   = "auto-public"
	CompileStrictSourceAutoPrivate  = "auto-private"
	CompileStrictSourceAutoFallback = "auto-fallback"
)

// VisibilityPublic is the `visibility` value returned by
// `gh api /repos/<owner>/<repo> --jq .visibility` for a public repo.
// The auto-detect compile-strict path treats this value as the signal to
// turn `--strict` ON; any other value (e.g. "private", "internal") turns
// it OFF. Kept as a named constant so the compile-strict resolver and the
// `gh-aw-fleet add` info-line printer compare against the same literal.
const VisibilityPublic = "public"

// SchemaVersion is the fleet config-file (fleet.json / fleet.local.json) format
// version written into Config.Version. Bumped only on breaking changes to the
// on-disk structure; additive changes (new optional fields) do not bump.
// Distinct from cmd.SchemaVersion, which versions the JSON output envelope.
const SchemaVersion = 1

// Config is the declarative desired state for the fleet.
// Loaded from fleet.local.json (if present) or fleet.json. Edits to this file are the source of truth;
// `fleet sync` and `fleet upgrade` reconcile repos toward it.
type Config struct {
	Version  int                 `json:"version"`
	Defaults Defaults            `json:"defaults,omitzero"`
	Profiles map[string]Profile  `json:"profiles,omitempty"`
	Repos    map[string]RepoSpec `json:"repos"`
	// LoadedFrom names the on-disk source of this Config in human form,
	// e.g. "fleet.json", "fleet.local.hujson", or "fleet.json + fleet.local.json"
	// when both base and local files were merged. Set by LoadConfig only;
	// callers must not modify.
	LoadedFrom string `json:"-"`
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

// EffectiveCompileStrict resolves whether `gh aw compile --strict` should run
// for repo. Returns (effective, source, reason) where source is one of
// CompileStrictSourceExplicit (operator set RepoSpec.CompileStrict),
// CompileStrictSourceAutoPublic (visibility lookup returned "public"),
// CompileStrictSourceAutoPrivate (visibility lookup returned any other value),
// or CompileStrictSourceAutoFallback (visibility lookup errored — fail-secure
// to strict ON, reason carries the truncated raw error). The method never
// returns an error: lookup failures fold into the auto-fallback source so the
// caller can proceed deterministically. Explicit override skips the
// visibility lookup entirely (FR-008).
func (c *Config) EffectiveCompileStrict(
	ctx context.Context, repo string,
) (bool, string, string) {
	if spec, ok := c.Repos[repo]; ok && spec.CompileStrict != nil {
		return *spec.CompileStrict, CompileStrictSourceExplicit, ""
	}
	visibility, err := ghRepoVisibility(ctx, repo)
	if err != nil {
		return true, CompileStrictSourceAutoFallback,
			truncateReason(err.Error(), effectiveCompileStrictReasonMax)
	}
	if visibility == VisibilityPublic {
		return true, CompileStrictSourceAutoPublic, ""
	}
	return false, CompileStrictSourceAutoPrivate, ""
}

// truncateReason bounds s to at most limit bytes, backing up to the
// nearest rune boundary so a multi-byte UTF-8 sequence is never split mid
// character. Typical input is ASCII gh-api error text, but a localized OS
// message or a repo name with non-ASCII could embed multibyte runes; the
// rune-safe truncation preserves valid UTF-8 in structured log fields.
func truncateReason(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	out := s[:limit]
	for len(out) > 0 {
		r, _ := utf8.DecodeLastRuneInString(out)
		if r != utf8.RuneError {
			return out
		}
		out = out[:len(out)-1]
	}
	return out
}

// Profile is a named bundle of workflows pulled from one or more upstream
// source repositories. A profile advances atomically: bumping Sources[x].Ref
// re-pins every workflow in the profile sourced from x. The optional Tier
// field carries an advisory cost-tier label consumed by `gh-aw-fleet list`
// and the planned consumption subcommand.
type Profile struct {
	Description string `json:"description,omitempty"`
	// Tier is an advisory cost-tier label (recommended vocabulary:
	// "minimal" | "standard" | "premium") used by the planned consumption
	// subcommand as a group-by key. Free-form — the tool does not enforce
	// the recommended vocabulary, and an empty string is equivalent to the
	// field being absent on disk.
	Tier      string               `json:"tier,omitempty"`
	Sources   map[string]SourcePin `json:"sources"`
	Workflows []ProfileWorkflow    `json:"workflows"`
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

// RepoSpec is the desired state for a single target repo. The optional
// CostCenter field carries a free-form budget-attribution label surfaced by
// `gh-aw-fleet list` and consumed as a group-by key by the planned
// consumption subcommand.
type RepoSpec struct {
	Profiles []string `json:"profiles"`
	// CostCenter is a free-form per-repo budget-attribution label. The tool
	// does not validate that the named cost center actually exists in the
	// org's billing UI, and applies no special handling based on which
	// fleet file (fleet.json vs fleet.local.json) the value appears in.
	CostCenter string `json:"cost_center,omitempty"`
	Engine     string `json:"engine,omitempty"`
	// CompileStrict toggles `gh aw compile --strict` for this repo's deploy
	// and upgrade pipelines. Tri-state: nil (the default) auto-detects from
	// repo visibility (public → ON, non-public → OFF, lookup failure →
	// fail-secure ON); a non-nil value short-circuits the auto-detect and
	// the visibility lookup is skipped. The field is additive — absence
	// round-trips byte-identically via HuJson AST mutation and incurs no
	// `fleet.SchemaVersion` bump.
	CompileStrict       *bool             `json:"compile_strict,omitempty"`
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
	// SkipIfMatch holds the pre-activation guard conditions from the
	// on.skip-if-match frontmatter key. When non-empty, the gh-aw runtime
	// cancels the workflow cheaply (before any AI call) if a condition matches
	// — the primary lever for reducing high-frequency trigger cost.
	SkipIfMatch []string `json:"skip_if_match,omitempty"`
	// SkipIfNoMatch holds the pre-activation guard conditions from the
	// on.skip-if-no-match frontmatter key, the inverse of skip-if-match.
	SkipIfNoMatch []string       `json:"skip_if_no_match,omitempty"`
	Frontmatter   map[string]any `json:"frontmatter,omitempty"`
	Body          string         `json:"body,omitempty"`
	Lines         int            `json:"lines,omitempty"`
}

// Evaluation is Claude's judgment on a workflow — used by `fleet template
// fetch` to flag newly-seen workflows and suggest profile membership.
type Evaluation struct {
	EvaluatedAt time.Time `json:"evaluated_at"`
	Summary     string    `json:"summary"`
	FitProfiles []string  `json:"fit_profiles,omitempty"`
	Recommend   string    `json:"recommend,omitempty"`
}
