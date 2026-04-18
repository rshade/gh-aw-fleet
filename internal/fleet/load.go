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

// LoadConfig reads fleet.local.json if present, otherwise fleet.json from the given directory (or cwd if empty).
// Sets LoadedFrom on the returned config to indicate which file was loaded.
func LoadConfig(dir string) (*Config, error) {
	// Try fleet.local.json first
	localPath := resolve(dir, LocalConfigFile)
	data, err := os.ReadFile(localPath)
	if err == nil {
		var c Config
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("parse %s: %w", localPath, err)
		}
		if c.Version != SchemaVersion {
			return nil, fmt.Errorf("%s schema version %d unsupported (expected %d)", localPath, c.Version, SchemaVersion)
		}
		c.LoadedFrom = localPath
		return &c, nil
	}
	if !os.IsNotExist(err) {
		// Some error other than "file not found"
		return nil, fmt.Errorf("read %s: %w", localPath, err)
	}

	// Fall back to fleet.json
	path := resolve(dir, ConfigFile)
	data, err = os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s or %s: %w", localPath, path, err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if c.Version != SchemaVersion {
		return nil, fmt.Errorf("%s schema version %d unsupported (expected %d)", path, c.Version, SchemaVersion)
	}
	c.LoadedFrom = path
	return &c, nil
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
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
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
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
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
		p, ok := c.Profiles[profileName]
		if !ok {
			return nil, fmt.Errorf("profile %q referenced by %q not defined", profileName, repo)
		}
		for _, w := range p.Workflows {
			if excluded[w.Name] || seen[w.Name] {
				continue
			}
			src, ok := p.Sources[w.Source]
			if !ok {
				return nil, fmt.Errorf("workflow %q references source %q with no pin in profile %q", w.Name, w.Source, profileName)
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
