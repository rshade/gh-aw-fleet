package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// SourceLayout is where a source repo keeps its workflow markdown files.
// Hardcoded for the two known sources; promote to config when a third arrives.
//
//nolint:gochecknoglobals // immutable source-layout lookup; Go has no const map
var SourceLayout = map[string]string{
	"github/gh-aw":        ".github/workflows",
	"githubnext/agentics": "workflows",
}

// FetchResult is what a single source fetch returns — plus a diff summary
// against the prior catalog.
type FetchResult struct {
	Source    string
	Ref       string
	Added     []string
	Changed   []string
	Removed   []string
	Unchanged []string
}

// FetchAll refreshes the catalog for every unique source referenced by any
// profile in cfg. Uses `main` for the catalog regardless of the profile's
// pinned ref — the catalog is about discovery, pins are about deployment.
func FetchAll(ctx context.Context, cfg *Config, prev *Templates) (*Templates, []FetchResult, error) {
	sources := collectSources(cfg)
	fresh := &Templates{
		Version:     SchemaVersion,
		FetchedAt:   time.Now().UTC(),
		Sources:     map[string]TemplateSource{},
		Evaluations: map[string]Evaluation{},
	}
	if prev != nil {
		fresh.Evaluations = prev.Evaluations
		if fresh.Evaluations == nil {
			fresh.Evaluations = map[string]Evaluation{}
		}
	}

	var results []FetchResult
	for _, src := range sources {
		ts, res, err := fetchSource(ctx, src)
		if err != nil {
			return nil, nil, fmt.Errorf("fetch %s: %w", src, err)
		}
		if prev != nil {
			if old, ok := prev.Sources[src]; ok {
				diffSource(old, ts, &res)
			} else {
				for _, w := range ts.Workflows {
					res.Added = append(res.Added, w.Name)
				}
			}
		} else {
			for _, w := range ts.Workflows {
				res.Added = append(res.Added, w.Name)
			}
		}
		fresh.Sources[src] = ts
		results = append(results, res)
	}
	return fresh, results, nil
}

func collectSources(cfg *Config) []string {
	seen := map[string]bool{}
	for _, p := range cfg.Profiles {
		for src := range p.Sources {
			seen[src] = true
		}
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// fetchSource lists the workflows dir of a source at `main`, then fetches
// each .md and parses it.
func fetchSource(ctx context.Context, source string) (TemplateSource, FetchResult, error) {
	dir, ok := SourceLayout[source]
	if !ok {
		return TemplateSource{}, FetchResult{Source: source}, fmt.Errorf("no layout configured for %s", source)
	}
	listing, err := ghAPIJSON(ctx, fmt.Sprintf("/repos/%s/contents/%s?ref=main", source, dir))
	if err != nil {
		return TemplateSource{}, FetchResult{Source: source}, err
	}
	entries, ok := listing.([]any)
	if !ok {
		return TemplateSource{}, FetchResult{Source: source}, fmt.Errorf("unexpected contents response for %s", source)
	}

	ts := TemplateSource{RefFetched: "main"}
	res := FetchResult{Source: source, Ref: "main"}

	for _, e := range entries {
		m, entryOK := e.(map[string]any)
		if !entryOK {
			continue
		}
		typ, _ := m["type"].(string)
		name, _ := m["name"].(string)
		path, _ := m["path"].(string)
		sha, _ := m["sha"].(string)
		if typ != "file" || !strings.HasSuffix(name, ".md") {
			continue
		}
		raw, rawErr := ghAPIRaw(ctx, fmt.Sprintf("/repos/%s/contents/%s?ref=main", source, path))
		if rawErr != nil {
			return TemplateSource{}, res, fmt.Errorf("fetch %s/%s: %w", source, path, rawErr)
		}
		tw := TemplateWorkflow{
			Name:  strings.TrimSuffix(name, ".md"),
			Path:  path,
			SHA:   sha,
			Body:  "",
			Lines: strings.Count(raw, "\n"),
		}
		fmText, body := SplitFrontmatter(raw)
		fm, parseErr := ParseFrontmatter(fmText)
		if parseErr != nil && !errors.Is(parseErr, ErrEmptyFrontmatter) {
			return TemplateSource{}, res, fmt.Errorf("parse %s/%s: %w", source, path, parseErr)
		}
		tw.Frontmatter = fm
		tw.Body = body
		ExtractWorkflowMeta(fm, &tw)
		ts.Workflows = append(ts.Workflows, tw)
	}
	sort.Slice(ts.Workflows, func(i, j int) bool { return ts.Workflows[i].Name < ts.Workflows[j].Name })
	return ts, res, nil
}

// diffSource mutates res with the Added/Changed/Removed/Unchanged names
// computed from prev vs next.
func diffSource(prev, next TemplateSource, res *FetchResult) {
	prevBy := map[string]TemplateWorkflow{}
	for _, w := range prev.Workflows {
		prevBy[w.Name] = w
	}
	nextBy := map[string]TemplateWorkflow{}
	for _, w := range next.Workflows {
		nextBy[w.Name] = w
	}
	for name, nw := range nextBy {
		pw, ok := prevBy[name]
		if !ok {
			res.Added = append(res.Added, name)
			continue
		}
		if pw.SHA != nw.SHA {
			res.Changed = append(res.Changed, name)
		} else {
			res.Unchanged = append(res.Unchanged, name)
		}
	}
	for name := range prevBy {
		if _, ok := nextBy[name]; !ok {
			res.Removed = append(res.Removed, name)
		}
	}
	sort.Strings(res.Added)
	sort.Strings(res.Changed)
	sort.Strings(res.Removed)
	sort.Strings(res.Unchanged)
}

//nolint:gochecknoglobals // test-injection seam for gh API JSON binding
var ghAPIJSON = func(ctx context.Context, path string) (any, error) {
	out, err := exec.CommandContext(ctx, "gh", "api", path).Output()
	if err != nil {
		return nil, ghErr(err)
	}
	var v any
	if decodeErr := json.Unmarshal(out, &v); decodeErr != nil {
		return nil, fmt.Errorf("decode gh api response: %w", decodeErr)
	}
	return v, nil
}

func ghAPIRaw(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", "api", "-H", "Accept: application/vnd.github.raw", path)
	out, err := cmd.Output()
	if err != nil {
		return "", ghErr(err)
	}
	return string(out), nil
}

func ghErr(err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) && len(ee.Stderr) > 0 {
		return fmt.Errorf("gh api: %s", strings.TrimSpace(string(ee.Stderr)))
	}
	return fmt.Errorf("gh api: %w", err)
}
