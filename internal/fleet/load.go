package fleet

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	ConfigFile      = "fleet.json"
	LocalConfigFile = "fleet.local.json"
	TemplatesFile   = "templates.json"
)

// LoadConfig reads fleet.json as the base config, then overlays fleet.local.json if present.
// Repos and profiles from fleet.local.json are merged on top of fleet.json — local entries
// add to or replace base entries, so you never need to duplicate shared profiles.
// Sets LoadedFrom on the returned config to indicate which file(s) were loaded.
func LoadConfig(dir string) (*Config, error) {
	basePath := resolve(dir, ConfigFile)
	base, baseErr := loadConfigFile(basePath)
	if baseErr != nil && !os.IsNotExist(baseErr) {
		return nil, fmt.Errorf("read %s: %w", basePath, baseErr)
	}

	localPath := resolve(dir, LocalConfigFile)
	local, localErr := loadConfigFile(localPath)
	if localErr != nil && !os.IsNotExist(localErr) {
		return nil, fmt.Errorf("read %s: %w", localPath, localErr)
	}

	if base == nil && local == nil {
		return nil, fmt.Errorf("no config found: tried %s and %s", basePath, localPath)
	}
	if base == nil {
		local.LoadedFrom = localPath
		return local, nil
	}
	if local == nil {
		base.LoadedFrom = basePath
		return base, nil
	}

	merged := mergeConfigs(base, local)
	merged.LoadedFrom = fmt.Sprintf("%s + %s", basePath, localPath)
	return merged, nil
}

// loadConfigFile reads and parses a single config file. Returns (nil, os.ErrNotExist) if missing.
func loadConfigFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if jsonErr := json.Unmarshal(data, &c); jsonErr != nil {
		return nil, fmt.Errorf("parse %s: %w", path, jsonErr)
	}
	if c.Version != SchemaVersion {
		return nil, fmt.Errorf("%s schema version %d unsupported (expected %d)", path, c.Version, SchemaVersion)
	}
	return &c, nil
}

// mergeConfigs overlays the local config on top of the base config.
// Profiles and repos from local add to or replace those from base.
// Local defaults win over base defaults when non-empty.
func mergeConfigs(base, local *Config) *Config {
	merged := *base

	if local.Defaults.Engine != "" {
		merged.Defaults.Engine = local.Defaults.Engine
	}

	merged.Profiles = make(map[string]Profile, len(base.Profiles)+len(local.Profiles))
	for k, v := range base.Profiles {
		merged.Profiles[k] = v
	}
	for k, v := range local.Profiles {
		merged.Profiles[k] = v
	}

	merged.Repos = make(map[string]RepoSpec, len(base.Repos)+len(local.Repos))
	for k, v := range base.Repos {
		merged.Repos[k] = v
	}
	for k, v := range local.Repos {
		merged.Repos[k] = v
	}

	return &merged
}

// SaveConfig writes fleet.json atomically to the given directory.
func SaveConfig(dir string, c *Config) error {
	path := resolve(dir, ConfigFile)
	return writeJSON(path, c)
}

// LoadTemplates reads templates.json; returns an empty catalog if the file
// doesn't exist (first-run case, before `fleet template fetch`).
func LoadTemplates(dir string) (*Templates, error) {
	path := resolve(dir, TemplatesFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Templates{Version: SchemaVersion, Sources: map[string]TemplateSource{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var t Templates
	if jsonErr := json.Unmarshal(data, &t); jsonErr != nil {
		return nil, fmt.Errorf("parse %s: %w", path, jsonErr)
	}
	return &t, nil
}

func SaveTemplates(dir string, t *Templates) error {
	return writeJSON(resolve(dir, TemplatesFile), t)
}

func resolve(dir, name string) string {
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, name)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if writeErr := os.WriteFile(tmp, append(data, '\n'), 0o600); writeErr != nil {
		return writeErr
	}
	return os.Rename(tmp, path)
}

// ResolveRepoWorkflows flattens a RepoSpec into the concrete list of
// (workflow, source, ref) tuples that should be deployed to that repo.
// Applies profile membership, exclusions, and extras in that order.
func (c *Config) ResolveRepoWorkflows(repo string) ([]ResolvedWorkflow, error) {
	spec, ok := c.Repos[repo]
	if !ok {
		return nil, fmt.Errorf("repo %q not in fleet.json", repo)
	}
	excluded := map[string]bool{}
	for _, name := range spec.ExcludeFromProfiles {
		excluded[name] = true
	}
	var out []ResolvedWorkflow
	seen := map[string]bool{}
	for _, profileName := range spec.Profiles {
		p, profileOK := c.Profiles[profileName]
		if !profileOK {
			return nil, fmt.Errorf("profile %q referenced by %q not defined", profileName, repo)
		}
		for _, w := range p.Workflows {
			if excluded[w.Name] || seen[w.Name] {
				continue
			}
			src, pinOK := p.Sources[w.Source]
			if !pinOK {
				return nil, fmt.Errorf(
					"workflow %q references source %q with no pin in profile %q",
					w.Name, w.Source, profileName,
				)
			}
			out = append(out, ResolvedWorkflow{
				Name:    w.Name,
				Source:  w.Source,
				Ref:     src.Ref,
				Path:    w.Path,
				Profile: profileName,
			})
			seen[w.Name] = true
		}
	}
	for _, e := range spec.ExtraWorkflows {
		if seen[e.Name] {
			continue
		}
		out = append(out, ResolvedWorkflow{
			Name:   e.Name,
			Source: e.Source,
			Ref:    e.Ref,
			Path:   e.Path,
			Extra:  true,
		})
	}
	return out, nil
}

// ResolvedWorkflow is a workflow reduced to its concrete deploy coordinates.
type ResolvedWorkflow struct {
	Name    string
	Source  string
	Ref     string
	Path    string
	Profile string
	Extra   bool
}

// Spec returns the gh-aw spec string for `gh aw add`.
// For sources with a .github/workflows layout (like gh-aw dogfooding itself),
// produces the 4-part form: "owner/repo/.github/workflows/name.md@ref".
// For sources with a simple workflows/ layout (agentics), produces the
// 3-part form: "owner/repo/name@ref".
// Local workflows pass through unchanged.
func (r ResolvedWorkflow) Spec() string {
	if r.Source == "local" {
		if r.Path != "" {
			return r.Path
		}
		return fmt.Sprintf("./.github/workflows/%s.md", r.Name)
	}
	layout := SourceLayout[r.Source]
	var s string
	if layout == ".github/workflows" {
		s = fmt.Sprintf("%s/%s/%s.md", r.Source, layout, r.Name)
	} else {
		s = fmt.Sprintf("%s/%s", r.Source, r.Name)
	}
	if r.Ref != "" {
		s += "@" + r.Ref
	}
	return s
}
