package fleet

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	zlog "github.com/rs/zerolog/log"
	"github.com/tailscale/hujson"
)

const (
	// slugHalves is the number of `/`-separated halves in a valid owner/repo slug.
	slugHalves = 2
	// extraSpecShortParts is the 3-part extra-workflow spec: owner/repo/name@ref (agentics layout).
	extraSpecShortParts = 3
)

// AddOptions controls the `fleet add` onboarding call. Apply=false is dry-run;
// Apply=true writes fleet.local.json and requires Confirmed=true to guard
// against non-interactive misuse.
type AddOptions struct {
	Repo           string
	Profiles       []string
	Engine         string
	Excludes       []string
	ExtraWorkflows []string
	Apply          bool
	Confirmed      bool
	Dir            string
}

// AddResult aggregates what an Add call resolved and persisted. SynthesizedLocal
// is true when Add had to create fleet.local.json from scratch (no prior
// private overlay existed); WroteLocal is true only after --apply succeeded.
type AddResult struct {
	Repo             string
	Profiles         []string
	Engine           string
	Resolved         []ResolvedWorkflow
	Warnings         []string
	WroteLocal       bool
	SynthesizedLocal bool
	LocalPath        string
}

var slugHalfRE = regexp.MustCompile(`^[a-z0-9._-]+$`)

// ValidateSlug normalizes and validates an owner/repo slug. Every error
// includes a valid-form example so operators can fix their input without
// consulting docs.
func ValidateSlug(s string) (string, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", fmt.Errorf("invalid repo slug %q: empty string; expected form: owner/repo", s)
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) != slugHalves {
		return "", fmt.Errorf(
			"invalid repo slug %q: must contain exactly one %q; expected form: owner/repo",
			s, "/",
		)
	}
	owner := strings.TrimSpace(parts[0])
	repo := strings.TrimSpace(parts[1])
	if owner == "" || repo == "" {
		return "", fmt.Errorf(
			"invalid repo slug %q: owner and repo halves must both be non-empty; expected form: owner/repo",
			s,
		)
	}
	owner = strings.ToLower(owner)
	repo = strings.ToLower(repo)
	if !slugHalfRE.MatchString(owner) || !slugHalfRE.MatchString(repo) {
		return "", fmt.Errorf(
			"invalid repo slug %q: halves must match [a-z0-9._-]+; expected form: owner/repo",
			s,
		)
	}
	return owner + "/" + repo, nil
}

// Add onboards a new repo into the fleet. Validates profiles/engine/extras,
// resolves the candidate's workflows for preview, and — when Apply=true —
// persists a minimal fleet.local.json overlay for the new repo.
func Add(cfg *Config, opts AddOptions) (*AddResult, error) {
	if opts.Apply && !opts.Confirmed {
		return nil, errors.New("--apply requires --yes or interactive confirmation")
	}
	if len(opts.Profiles) == 0 {
		return nil, errors.New("at least one --profile must be specified")
	}
	if _, exists := cfg.Repos[opts.Repo]; exists {
		return nil, duplicateRepoError(cfg, opts.Dir, opts.Repo)
	}
	for _, p := range opts.Profiles {
		if _, ok := cfg.Profiles[p]; !ok {
			return nil, unknownKeyError("profile", p, cfg.Profiles)
		}
	}
	if opts.Engine != "" {
		if _, ok := EngineSecrets[opts.Engine]; !ok {
			return nil, unknownKeyError("engine", opts.Engine, EngineSecrets)
		}
	}

	parsedExtras := make([]ExtraWorkflow, 0, len(opts.ExtraWorkflows))
	for _, raw := range opts.ExtraWorkflows {
		extra, parseErr := parseExtraWorkflowSpec(raw)
		if parseErr != nil {
			return nil, parseErr
		}
		parsedExtras = append(parsedExtras, extra)
	}

	candidate := RepoSpec{
		Profiles:            opts.Profiles,
		Engine:              opts.Engine,
		ExtraWorkflows:      parsedExtras,
		ExcludeFromProfiles: opts.Excludes,
	}

	// Insert the candidate transiently so ResolveRepoWorkflows sees it in
	// the merged view. Always remove before returning — the caller's Config
	// must not carry unpersisted state. cfg.Repos may be nil when a config
	// file omits the "repos" key; initialize defensively before writing.
	if cfg.Repos == nil {
		cfg.Repos = make(map[string]RepoSpec)
	}
	cfg.Repos[opts.Repo] = candidate
	resolved, err := ResolveRepoWorkflows(cfg, opts.Repo)
	delete(cfg.Repos, opts.Repo)
	if err != nil {
		return nil, fmt.Errorf("resolve workflows for %q: %w", opts.Repo, err)
	}

	localPath, localExists, probeErr := probeConfigPath(opts.Dir, localBase)
	if probeErr != nil {
		return nil, fmt.Errorf("probe local config: %w", probeErr)
	}
	res := &AddResult{
		Repo:             opts.Repo,
		Profiles:         opts.Profiles,
		Engine:           opts.Engine,
		Resolved:         resolved,
		Warnings:         collectAddWarnings(cfg, opts, parsedExtras, resolved),
		SynthesizedLocal: !localExists,
		LocalPath:        localPath,
	}

	if !opts.Apply {
		return res, nil
	}

	if writeErr := writeLocalAddition(opts.Dir, opts.Repo, candidate); writeErr != nil {
		return res, fmt.Errorf("write %s: %w", localPath, writeErr)
	}
	res.WroteLocal = true
	return res, nil
}

// writeLocalAddition persists a new repo entry into the local-overlay file,
// preferring an in-place HuJson AST mutation that preserves operator-authored
// comments and trailing commas elsewhere in the file. Falls back to a full
// re-marshal (losing comments but emitting a structured warning) when the
// AST path cannot apply the mutation.
//
// When no local file exists yet, BuildMinimalLocalConfig produces the
// scaffold and writeJSON writes it out — there are no comments to preserve.
func writeLocalAddition(dir, repo string, spec RepoSpec) error {
	path, exists, err := probeConfigPath(dir, localBase)
	if err != nil {
		return err
	}
	if !exists {
		return writeJSON(path, BuildMinimalLocalConfig(repo, spec))
	}
	astErr := writeHujson(path, func(root *hujson.Value) error {
		return appendRepoMember(root, repo, spec)
	})
	if astErr == nil {
		return nil
	}
	zlog.Warn().
		Str("event", "hujson_fallback_to_rewrite").
		Str("path", path).
		Err(astErr).
		Msg("comment-preserving append failed; rewriting from current contents")
	existing, loadErr := loadConfigFile(path)
	if loadErr != nil {
		return fmt.Errorf("fallback re-read %s: %w", path, loadErr)
	}
	if existing.Repos == nil {
		existing.Repos = map[string]RepoSpec{}
	}
	existing.Repos[repo] = spec
	return writeJSON(path, existing)
}

// appendRepoMember inserts a new (slug, spec) pair under the root config's
// /repos object via direct AST mutation. Comments and trailing commas
// elsewhere in the file are unaffected. Synthesizes an empty /repos
// member when the on-disk file omits the key.
func appendRepoMember(root *hujson.Value, slug string, spec RepoSpec) error {
	rootObj, ok := root.Value.(*hujson.Object)
	if !ok {
		return errors.New("config root is not a JSON object")
	}
	reposVal := root.Find("/repos")
	if reposVal == nil {
		reposParsed, parseErr := hujson.Parse([]byte("{}"))
		if parseErr != nil {
			return fmt.Errorf("synthesize /repos: %w", parseErr)
		}
		nameParsed, parseErr := hujson.Parse([]byte(`"repos"`))
		if parseErr != nil {
			return fmt.Errorf("synthesize /repos name: %w", parseErr)
		}
		rootObj.Members = append(rootObj.Members, hujson.ObjectMember{
			Name:  nameParsed,
			Value: reposParsed,
		})
		reposVal = root.Find("/repos")
		if reposVal == nil {
			return errors.New("/repos missing after synthesis")
		}
	}
	reposObj, ok := reposVal.Value.(*hujson.Object)
	if !ok {
		return errors.New("/repos is not a JSON object")
	}

	specJSON, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("marshal repo spec: %w", err)
	}
	specParsed, err := hujson.Parse(specJSON)
	if err != nil {
		return fmt.Errorf("parse repo spec: %w", err)
	}
	nameJSON, err := json.Marshal(slug)
	if err != nil {
		return fmt.Errorf("marshal slug: %w", err)
	}
	nameParsed, err := hujson.Parse(nameJSON)
	if err != nil {
		return fmt.Errorf("parse slug: %w", err)
	}
	reposObj.Members = append(reposObj.Members, hujson.ObjectMember{
		Name:  nameParsed,
		Value: specParsed,
	})
	return nil
}

// unknownKeyError formats a consistent "X not defined; available: [...]" error
// across profile-mismatch and engine-mismatch paths.
func unknownKeyError[V any](kind, value string, available map[string]V) error {
	keys := make([]string, 0, len(available))
	for k := range available {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return fmt.Errorf("%s %q not defined; available %ss: [%s]",
		kind, value, kind, strings.Join(keys, ", "))
}

func collectAddWarnings(
	cfg *Config, opts AddOptions, parsedExtras []ExtraWorkflow, resolved []ResolvedWorkflow,
) []string {
	profileNames := map[string]bool{}
	for _, p := range opts.Profiles {
		prof, ok := cfg.Profiles[p]
		if !ok {
			continue
		}
		for _, w := range prof.Workflows {
			profileNames[w.Name] = true
		}
	}

	var warnings []string
	for _, excl := range opts.Excludes {
		if !profileNames[excl] {
			warnings = append(warnings, fmt.Sprintf(
				"--exclude %q did not match any workflow in the selected profile(s); the exclusion is a no-op",
				excl,
			))
		}
	}
	for _, extra := range parsedExtras {
		if profileNames[extra.Name] {
			warnings = append(warnings, fmt.Sprintf(
				"--extra-workflow %q shadows a profile workflow; the extra will be dropped by deduplication",
				extra.Name,
			))
		}
	}
	if len(resolved) == 0 {
		warnings = append(warnings,
			"no workflows resolved for this repo; zero workflows will be deployed (check --profile and --exclude)",
		)
	}
	return warnings
}

// duplicateRepoError names the source file(s) declaring repo. When LoadConfig
// loaded only one file, the answer is carried in cfg.LoadedFrom with no disk
// I/O. In the merged-load case we re-read both files (probing for .hujson
// or .json each) to disambiguate. Reports filenames as base names rather
// than full paths to keep operator-facing errors stable across working
// directories. Probe failures (e.g. permission errors on stat) surface a
// degraded error that still names the duplicate but admits the missing
// disambiguation.
func duplicateRepoError(cfg *Config, dir, repo string) error {
	basePath, _, baseProbeErr := probeConfigPath(dir, configBase)
	localPath, _, localProbeErr := probeConfigPath(dir, localBase)
	if baseProbeErr != nil || localProbeErr != nil {
		return fmt.Errorf(
			"repo %q already exists in fleet config (could not disambiguate source: %w)",
			repo, errors.Join(baseProbeErr, localProbeErr),
		)
	}
	switch cfg.LoadedFrom {
	case basePath:
		return fmt.Errorf("repo %q already exists in %s", repo, filepath.Base(basePath))
	case localPath:
		return fmt.Errorf("repo %q already exists in %s", repo, filepath.Base(localPath))
	}

	var sources []string
	if base, err := loadConfigFile(basePath); err == nil && base != nil {
		if _, ok := base.Repos[repo]; ok {
			sources = append(sources, filepath.Base(basePath))
		}
	}
	if local, err := loadConfigFile(localPath); err == nil && local != nil {
		if _, ok := local.Repos[repo]; ok {
			sources = append(sources, filepath.Base(localPath))
		}
	}
	if len(sources) == 0 {
		return fmt.Errorf("repo %q already exists in fleet config", repo)
	}
	return fmt.Errorf("repo %q already exists in %s", repo, strings.Join(sources, " + "))
}

// parseExtraWorkflowSpec accepts three forms:
//   - `name`                                             → local
//   - `owner/repo/name@ref`                              → 3-part (agentics)
//   - `owner/repo/.github/workflows/name.md@ref`         → 4-part (gh-aw)
func parseExtraWorkflowSpec(s string) (ExtraWorkflow, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return ExtraWorkflow{}, fmt.Errorf(
			"invalid --extra-workflow %q: empty; expected name | owner/repo/name@ref | owner/repo/.github/workflows/name.md@ref",
			s,
		)
	}
	if !strings.Contains(trimmed, "/") {
		return ExtraWorkflow{Name: trimmed, Source: SourceLocal}, nil
	}

	atIdx := strings.Index(trimmed, "@")
	if atIdx < 0 {
		return ExtraWorkflow{}, fmt.Errorf(
			"invalid --extra-workflow %q: missing @ref; expected owner/repo/name@ref or owner/repo/.github/workflows/name.md@ref",
			s,
		)
	}
	lhs := trimmed[:atIdx]
	ref := trimmed[atIdx+1:]
	if ref == "" {
		return ExtraWorkflow{}, fmt.Errorf(
			"invalid --extra-workflow %q: empty ref after @; expected owner/repo/name@ref",
			s,
		)
	}
	parts := strings.Split(lhs, "/")
	switch {
	case len(parts) == extraSpecShortParts:
		return ExtraWorkflow{
			Name:   parts[2],
			Source: parts[0] + "/" + parts[1],
			Ref:    ref,
		}, nil
	case len(parts) >= 5 && parts[2] == ".github" && parts[3] == "workflows":
		path := strings.Join(parts[2:], "/")
		name := strings.TrimSuffix(parts[len(parts)-1], ".md")
		return ExtraWorkflow{
			Name:   name,
			Source: parts[0] + "/" + parts[1],
			Ref:    ref,
			Path:   path,
		}, nil
	default:
		return ExtraWorkflow{}, fmt.Errorf(
			"invalid --extra-workflow %q: expected owner/repo/name@ref or owner/repo/.github/workflows/name.md@ref",
			s,
		)
	}
}

// BuildMinimalLocalConfig builds a Config carrying only Version and a single
// Repos entry. Defaults / profiles / peer repos are NOT copied — mergeConfigs
// supplies those at load time from fleet.json.
func BuildMinimalLocalConfig(repo string, spec RepoSpec) *Config {
	return &Config{
		Version: SchemaVersion,
		Repos:   map[string]RepoSpec{repo: spec},
	}
}
