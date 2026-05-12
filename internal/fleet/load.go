package fleet

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	zlog "github.com/rs/zerolog/log"
	"github.com/tailscale/hujson"
)

// Filenames the fleet reads/writes relative to the working directory.
const (
	// ConfigFile is the committed, public declarative config (example fleet).
	ConfigFile = "fleet.json"
	// LocalConfigFile is the private, gitignored overlay merged on top of ConfigFile at load time.
	LocalConfigFile = "fleet.local.json"
	// TemplatesFile is the upstream-catalog cache written by `fleet template fetch`.
	TemplatesFile = "templates.json"

	// SourceLocal marks an ExtraWorkflow / ResolvedWorkflow as living in the
	// target repo itself (no upstream fetch; `gh aw add` takes a local path).
	SourceLocal = "local"
)

// Base names (no extension) used by probeConfigPath when deciding between
// the standard-JSON and HuJson variants of each config file.
const (
	configBase    = "fleet"
	localBase     = "fleet.local"
	templatesBase = "templates"

	hujsonExt = ".hujson"
	jsonExt   = ".json"
)

// jsonPatchOpAdd is the RFC 6902 "add" operation name. Per the RFC, "add"
// replaces the value when the target path already exists, which is the
// behavior buildTemplatesPatch relies on.
const (
	jsonPatchOpAdd       = "add"
	jsonPatchMemberOp    = "op"
	jsonPatchMemberPath  = "path"
	jsonPatchMemberValue = "value"
)

// probeConfigPath returns the on-disk path for the given base name. Prefers
// <base>.hujson over <base>.json so operators who opt into HuJson syntax
// can name files explicitly. Errors when both extensions are present —
// that is a misconfiguration: the loader cannot guess which one is
// authoritative, and a silent prefer would let the unread file drift.
//
// When neither file exists, the .json path is returned with exists=false
// so callers can synthesize a new file at the standard-JSON name.
func probeConfigPath(dir, base string) (string, bool, error) {
	hujsonPath := resolve(dir, base+hujsonExt)
	jsonPath := resolve(dir, base+jsonExt)
	hujsonExists, hErr := pathExists(hujsonPath)
	if hErr != nil {
		return "", false, hErr
	}
	jsonExists, jErr := pathExists(jsonPath)
	if jErr != nil {
		return "", false, jErr
	}
	if hujsonExists && jsonExists {
		return "", false, fmt.Errorf(
			"ambiguous config: both %s and %s exist; remove one",
			hujsonPath, jsonPath,
		)
	}
	if hujsonExists {
		return hujsonPath, true, nil
	}
	if jsonExists {
		return jsonPath, true, nil
	}
	return jsonPath, false, nil
}

// pathExists distinguishes "absent" (false, nil) from "stat failed for some
// other reason" (false, err) so callers can treat the two cases differently.
func pathExists(p string) (bool, error) {
	_, err := os.Stat(p)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// LoadConfig reads fleet.json (or fleet.hujson) as the base, then overlays
// fleet.local.json (or fleet.local.hujson) if present. Repos and profiles
// from the local file merge on top of the base — local entries add to or
// replace base entries, so you never need to duplicate shared profiles.
//
// HuJson syntax (//-line comments, /*-block comments, trailing commas) is
// supported in either file via hujson.Standardize on the read path.
//
// Sets LoadedFrom on the returned config to indicate which file(s) were
// loaded.
func LoadConfig(dir string) (*Config, error) {
	basePath, baseExists, err := probeConfigPath(dir, configBase)
	if err != nil {
		return nil, err
	}
	var base *Config
	if baseExists {
		base, err = loadConfigFile(basePath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", basePath, err)
		}
	}

	localPath, localExists, err := probeConfigPath(dir, localBase)
	if err != nil {
		return nil, err
	}
	var local *Config
	if localExists {
		local, err = loadConfigFile(localPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", localPath, err)
		}
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

// loadConfigFile reads and parses a single config file. Runs the input
// through hujson.Standardize before json.Unmarshal so HuJson syntax is
// transparent to the consumer; vanilla JSON passes through unchanged.
// Returns (nil, os.ErrNotExist) if missing.
func loadConfigFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	std, stdErr := hujson.Standardize(data)
	if stdErr != nil {
		return nil, fmt.Errorf("parse %s: %w", path, stdErr)
	}
	var c Config
	if jsonErr := json.Unmarshal(std, &c); jsonErr != nil {
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

// SaveLocalConfig writes fleet.local.json (or fleet.local.hujson when that
// is the existing file) atomically to the given directory. Targets the
// probed path so a write next to an existing .hujson source does not
// silently create a .json sibling that the next read would reject as
// ambiguous. No symmetric SaveConfig exists: fleet.json is read-only
// from this package.
func SaveLocalConfig(dir string, c *Config) error {
	path, _, err := probeConfigPath(dir, localBase)
	if err != nil {
		return err
	}
	return writeJSON(path, c)
}

// LoadTemplates reads templates.json (or templates.hujson); returns an
// empty catalog if neither file exists (first-run case, before
// `fleet template fetch`).
func LoadTemplates(dir string) (*Templates, error) {
	path, exists, err := probeConfigPath(dir, templatesBase)
	if err != nil {
		return nil, err
	}
	if !exists {
		return &Templates{Version: SchemaVersion, Sources: map[string]TemplateSource{}}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	std, stdErr := hujson.Standardize(data)
	if stdErr != nil {
		return nil, fmt.Errorf("parse %s: %w", path, stdErr)
	}
	var t Templates
	if jsonErr := json.Unmarshal(std, &t); jsonErr != nil {
		return nil, fmt.Errorf("parse %s: %w", path, jsonErr)
	}
	return &t, nil
}

// SaveTemplates writes the upstream-catalog cache to dir/<templatesBase>.<ext>,
// targeting the probed path so a write next to an existing .hujson source
// does not create a .json sibling.
//
// When the file already exists, applies an RFC 6902 patch that replaces only
// /version, /fetched_at, and /sources — leaving /evaluations and any
// surrounding comments intact. This preserves operator-authored notes on
// individual workflow evaluations across `fleet template fetch` runs.
//
// When no existing file is present, falls back to a full marshal — there
// are no comments to preserve.
func SaveTemplates(dir string, t *Templates) error {
	path, exists, err := probeConfigPath(dir, templatesBase)
	if err != nil {
		return err
	}
	if !exists {
		return writeJSON(path, t)
	}
	ops, opsErr := buildTemplatesPatch(t)
	if opsErr != nil {
		return opsErr
	}
	patchErr := writeHujson(path, func(v *hujson.Value) error {
		if applyErr := v.Patch(ops); applyErr != nil {
			return fmt.Errorf("apply patch to %s: %w", path, applyErr)
		}
		return nil
	})
	if patchErr != nil {
		zlog.Warn().
			Str("event", "hujson_fallback_to_rewrite").
			Str("path", path).
			Err(patchErr).
			Msg("comment-preserving patch failed; rewriting templates from scratch")
		return writeJSON(path, t)
	}
	return nil
}

// buildTemplatesPatch produces an RFC 6902 patch document with three "add"
// ops (add replaces the value when the key already exists, per the RFC).
// /evaluations is intentionally excluded so existing entries — and the
// comments around them — survive the write unchanged.
func buildTemplatesPatch(t *Templates) ([]byte, error) {
	ops := []map[string]any{
		{jsonPatchMemberOp: jsonPatchOpAdd, jsonPatchMemberPath: "/version", jsonPatchMemberValue: t.Version},
		{jsonPatchMemberOp: jsonPatchOpAdd, jsonPatchMemberPath: "/fetched_at", jsonPatchMemberValue: t.FetchedAt},
		{jsonPatchMemberOp: jsonPatchOpAdd, jsonPatchMemberPath: "/sources", jsonPatchMemberValue: t.Sources},
	}
	data, err := json.Marshal(ops)
	if err != nil {
		return nil, fmt.Errorf("marshal patch ops: %w", err)
	}
	return data, nil
}

func resolve(dir, name string) string {
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, name)
}

// writeJSON serializes v as indented JSON and writes it to path atomically
// via tmp+rename. Used for the first-write case (no existing file to
// preserve comments from) and as the fallback path when comment-preserving
// writers cannot apply a mutation. Trailing-newline policy lives in
// atomicWrite.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, data)
}

// atomicWrite stages bytes at path+".tmp" then renames into place, ensuring
// readers never observe a partially-written file. Ensures a trailing
// newline (POSIX text-file convention).
func atomicWrite(path string, data []byte) error {
	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	tmp := path + ".tmp"
	if writeErr := os.WriteFile(tmp, data, 0o600); writeErr != nil {
		return writeErr
	}
	return os.Rename(tmp, path)
}

// writeHujson reads the existing file at path (or starts from an empty
// object scaffold when missing), parses it as HuJson, runs the caller's
// apply step on the syntax tree, formats, packs, and atomically writes.
// Comments and whitespace outside the touched region survive.
//
// Returns an error if the file is unreadable for a reason other than
// not-exist, if hujson.Parse rejects the file, or if apply fails.
// Callers that want graceful degradation (fall back to a full rewrite)
// detect those errors and call writeJSON themselves with a warning log.
func writeHujson(path string, apply func(*hujson.Value) error) error {
	existing, err := readHujsonOrScaffold(path)
	if err != nil {
		return err
	}
	v, parseErr := hujson.Parse(existing)
	if parseErr != nil {
		return fmt.Errorf("parse %s as hujson: %w", path, parseErr)
	}
	if applyErr := apply(&v); applyErr != nil {
		return applyErr
	}
	v.Format()
	return atomicWrite(path, v.Pack())
}

// readHujsonOrScaffold returns the contents of path, or "{}" when path
// does not exist (giving Parse a valid empty-object starting point).
func readHujsonOrScaffold(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return data, nil
	}
	if os.IsNotExist(err) {
		return []byte("{}"), nil
	}
	return nil, fmt.Errorf("read %s: %w", path, err)
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
	if r.Source == SourceLocal {
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
