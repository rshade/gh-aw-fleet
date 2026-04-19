package fleet

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrEmptyFrontmatter is returned by ParseFrontmatter when the input has no
// YAML content. Callers typically treat this as a non-fatal skip.
var ErrEmptyFrontmatter = errors.New("empty frontmatter")

// SplitFrontmatter separates a markdown file's YAML frontmatter from its body.
// Returns (frontmatterYAML, body) — frontmatterYAML is empty if no `---` fence.
func SplitFrontmatter(src string) (string, string) {
	src = strings.TrimPrefix(src, "\ufeff")
	if !strings.HasPrefix(src, "---") {
		return "", src
	}
	after := strings.TrimPrefix(src, "---")
	after = strings.TrimPrefix(after, "\r")
	after = strings.TrimPrefix(after, "\n")
	end := strings.Index(after, "\n---")
	if end < 0 {
		return "", src
	}
	fm := after[:end]
	body := after[end+len("\n---"):]
	body = strings.TrimPrefix(body, "\r")
	body = strings.TrimPrefix(body, "\n")
	return fm, body
}

// ParseFrontmatter yaml-decodes a frontmatter chunk into a generic map.
// Returns ErrEmptyFrontmatter when the input has no YAML content.
func ParseFrontmatter(fm string) (map[string]any, error) {
	if strings.TrimSpace(fm) == "" {
		return nil, ErrEmptyFrontmatter
	}
	out := map[string]any{}
	if err := yaml.Unmarshal([]byte(fm), &out); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}
	return out, nil
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
