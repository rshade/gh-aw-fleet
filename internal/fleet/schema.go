package fleet

import (
	"context"
	"time"
	"unicode/utf8"

	pkgfleet "github.com/rshade/gh-aw-fleet/pkg/fleet"
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
const SchemaVersion = pkgfleet.SchemaVersion

// Config is the declarative desired state for the fleet.
type Config = pkgfleet.Config

// Defaults are applied to every repo unless overridden in RepoSpec.
type Defaults = pkgfleet.Defaults

// Profile is a named bundle of workflows pulled from one or more upstream
// source repositories.
type Profile = pkgfleet.Profile

// SourcePin records the ref for a source repo within a profile.
type SourcePin = pkgfleet.SourcePin

// ProfileWorkflow names a workflow and which source repo provides it.
type ProfileWorkflow = pkgfleet.ProfileWorkflow

// RepoSpec is the desired state for a single target repo.
type RepoSpec = pkgfleet.RepoSpec

// ExtraWorkflow is a per-repo workflow not sourced from any profile.
type ExtraWorkflow = pkgfleet.ExtraWorkflow

// EffectiveCompileStrict resolves whether `gh aw compile --strict` should run
// for repo. Returns (effective, source, reason) where source is one of
// CompileStrictSourceExplicit (operator set RepoSpec.CompileStrict),
// CompileStrictSourceAutoPublic (visibility lookup returned "public"),
// CompileStrictSourceAutoPrivate (visibility lookup returned any other value),
// or CompileStrictSourceAutoFallback (visibility lookup errored — fail-secure
// to strict ON, reason carries the truncated raw error). The function never
// returns an error: lookup failures fold into the auto-fallback source so the
// caller can proceed deterministically. Explicit override skips the
// visibility lookup entirely (FR-008).
func EffectiveCompileStrict(ctx context.Context, c *Config, repo string) (bool, string, string) {
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
