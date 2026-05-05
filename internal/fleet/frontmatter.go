package fleet

import (
	"sort"

	"github.com/rshade/gh-aw-fleet/internal/fleet/frontmatter"
)

// ErrEmptyFrontmatter is returned by ParseFrontmatter when the input has no
// YAML content. Callers typically treat this as a non-fatal skip.
//
// Forwarded from the leaf frontmatter package so internal/fleet/security
// can share the parser without an import cycle.
var ErrEmptyFrontmatter = frontmatter.ErrEmpty

// SplitFrontmatter separates a markdown file's YAML frontmatter from its body.
// Returns (frontmatterYAML, body) — frontmatterYAML is empty if no `---` fence.
//
// Forwards to frontmatter.Split.
func SplitFrontmatter(src string) (string, string) {
	return frontmatter.Split(src)
}

// ParseFrontmatter yaml-decodes a frontmatter chunk into a generic map.
// Returns ErrEmptyFrontmatter when the input has no YAML content.
//
// Forwards to frontmatter.Parse.
func ParseFrontmatter(fm string) (map[string]any, error) {
	return frontmatter.Parse(fm)
}

// ExtractWorkflowMeta pulls well-known, diff-friendly fields out of a parsed
// frontmatter map into a TemplateWorkflow. Caller still owns Frontmatter
// (full fidelity) and Body.
func ExtractWorkflowMeta(fm map[string]any, tw *TemplateWorkflow) {
	if v, ok := asString(fm, "description"); ok {
		tw.Description = v
	}
	if v, ok := asString(fm, "engine"); ok {
		tw.Engine = v
	} else if engMap, engOK := fm["engine"].(map[string]any); engOK {
		if idVal, idOK := asString(engMap, "id"); idOK {
			tw.Engine = idVal
		}
	}
	if v, ok := asString(fm, "stop-after"); ok {
		tw.StopAfter = v
	}
	tw.Triggers = extractTriggers(fm["on"])
	tw.Tools = extractToolKeys(fm["tools"])
	tw.SafeOutputs = extractToolKeys(fm["safe-outputs"])
	if perms, ok := fm["permissions"].(map[string]any); ok {
		tw.Permissions = perms
	}
}

func asString(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// extractTriggers flattens the `on:` section into a sorted list of trigger
// names, losing detail but good for table rendering / diffing.
func extractTriggers(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		sort.Strings(out)
		return out
	case map[string]any:
		out := make([]string, 0, len(t))
		for k := range t {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	default:
		return nil
	}
}

// extractToolKeys flattens a tools/safe-outputs map into a sorted list of
// tool names. Preserves the top-level keys only (no nested config).
func extractToolKeys(v any) []string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
