package fleet

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	// slugHalves is the number of `/`-separated halves in a valid owner/repo slug.
	slugHalves = 2
	// extraSpecShortParts is the 3-part extra-workflow spec: owner/repo/name@ref (agentics layout).
	extraSpecShortParts = 3
)

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
	// must not carry unpersisted state.
	cfg.Repos[opts.Repo] = candidate
	resolved, err := cfg.ResolveRepoWorkflows(opts.Repo)
	delete(cfg.Repos, opts.Repo)
	if err != nil {
		return nil, fmt.Errorf("resolve workflows for %q: %w", opts.Repo, err)
	}

	localPath := resolve(opts.Dir, LocalConfigFile)
	res := &AddResult{
		Repo:             opts.Repo,
		Profiles:         opts.Profiles,
		Engine:           opts.Engine,
		Resolved:         resolved,
		Warnings:         collectAddWarnings(cfg, opts, parsedExtras, resolved),
		SynthesizedLocal: !strings.Contains(cfg.LoadedFrom, LocalConfigFile),
		LocalPath:        localPath,
	}

	if !opts.Apply {
		return res, nil
	}

	minimal := BuildMinimalLocalConfig(opts.Repo, candidate)
	if saveErr := SaveLocalConfig(opts.Dir, minimal); saveErr != nil {
		return res, fmt.Errorf("write %s: %w", localPath, saveErr)
	}
	res.WroteLocal = true
	return res, nil
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
// I/O. In the merged-load case we re-read both files to disambiguate.
func duplicateRepoError(cfg *Config, dir, repo string) error {
	basePath := resolve(dir, ConfigFile)
	localPath := resolve(dir, LocalConfigFile)
	switch cfg.LoadedFrom {
	case basePath:
		return fmt.Errorf("repo %q already exists in %s", repo, ConfigFile)
	case localPath:
		return fmt.Errorf("repo %q already exists in %s", repo, LocalConfigFile)
	}

	var sources []string
	if base, err := loadConfigFile(basePath); err == nil && base != nil {
		if _, ok := base.Repos[repo]; ok {
			sources = append(sources, ConfigFile)
		}
	}
	if local, err := loadConfigFile(localPath); err == nil && local != nil {
		if _, ok := local.Repos[repo]; ok {
			sources = append(sources, LocalConfigFile)
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
