package fleet

import "sort"

// ListResult is the machine-readable form of `fleet list`'s tabwriter output.
// Embedded in the JSON envelope when --output json is active. Slice fields
// are normalized to non-nil empty slices by BuildListResult so JSON
// marshaling renders them as [] (FR-009).
type ListResult struct {
	LoadedFrom string    `json:"loaded_from"`
	Repos      []ListRow `json:"repos"`
}

// ListRow is one repo entry in ListResult.Repos. Engine renders as the empty
// string when no engine is configured, NOT the text-mode "-" placeholder.
// Profiles, Workflows, Excluded, Extra, and ProfileTiers are always non-nil.
// CostCenter is always emitted in the JSON envelope; the empty string
// signals "unset" rather than the field being omitted (FR-008).
type ListRow struct {
	Repo     string   `json:"repo"`
	Profiles []string `json:"profiles"`
	// ProfileTiers maps each of this row's profile names to the Tier value
	// declared on the underlying Profile definition. Keys are a subset of
	// Profiles — only profiles whose Tier is non-empty appear here.
	// Invariant: always an initialized map (never nil) so JSON marshals as
	// {} rather than null (FR-007).
	ProfileTiers map[string]string `json:"profile_tiers"`
	Engine       string            `json:"engine"`
	Workflows    []string          `json:"workflows"`
	Excluded     []string          `json:"excluded"`
	Extra        []string          `json:"extra"`
	// CostCenter mirrors RepoSpec.CostCenter. Always emitted in the JSON
	// envelope (no omitempty); the empty string is the unset signal.
	CostCenter string `json:"cost_center"`
}

// BuildListResult walks cfg.Repos in sorted order, resolves each repo's
// workflows, and builds the structured form for JSON emission.
//
// Returns an error if any repo's workflow resolution fails (matches the
// behavior of the existing tabwriter list path, which short-circuits on
// the first ResolveRepoWorkflows error).
func BuildListResult(cfg *Config) (*ListResult, error) {
	repoNames := make([]string, 0, len(cfg.Repos))
	for r := range cfg.Repos {
		repoNames = append(repoNames, r)
	}
	sort.Strings(repoNames)

	rows := make([]ListRow, 0, len(repoNames))
	for _, repo := range repoNames {
		spec := cfg.Repos[repo]
		resolved, err := ResolveRepoWorkflows(cfg, repo)
		if err != nil {
			return nil, err
		}
		rows = append(rows, ListRow{
			Repo:         repo,
			Profiles:     nonNilStrings(spec.Profiles),
			ProfileTiers: ProfileTiersMap(spec.Profiles, cfg.Profiles),
			Engine:       cfg.EffectiveEngine(repo),
			Workflows:    workflowNames(resolved),
			Excluded:     nonNilStrings(spec.ExcludeFromProfiles),
			Extra:        extraNames(spec.ExtraWorkflows),
			CostCenter:   spec.CostCenter,
		})
	}
	return &ListResult{LoadedFrom: cfg.LoadedFrom, Repos: rows}, nil
}

func workflowNames(resolved []ResolvedWorkflow) []string {
	out := make([]string, 0, len(resolved))
	for _, w := range resolved {
		out = append(out, w.Name)
	}
	return out
}

func extraNames(extras []ExtraWorkflow) []string {
	out := make([]string, 0, len(extras))
	for _, e := range extras {
		out = append(out, e.Name)
	}
	return out
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ProfileTiersMap returns the sparse map of profile-name → Tier for the
// profiles listed in `names`, dropping profiles whose Tier is unset.
// Returns a non-nil (possibly empty) map so JSON marshals as {} rather
// than null (FR-007 invariant). Single source of truth for the
// "what counts as a tier" rule, consumed by BuildListResult (JSON) and
// the text-mode list renderer.
func ProfileTiersMap(names []string, defs map[string]Profile) map[string]string {
	out := make(map[string]string, len(names))
	for _, n := range names {
		if t := defs[n].Tier; t != "" {
			out[n] = t
		}
	}
	return out
}
