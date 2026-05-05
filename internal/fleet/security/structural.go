package security

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/rshade/gh-aw-fleet/internal/fleet/frontmatter"
)

// structuralScanner evaluates each workflow's YAML frontmatter against the
// six fleet-specific structural rules. Engine for the engine.env rule is
// resolved per-workflow from frontmatter (FR-018), not from a constructor
// parameter, because a profile may legitimately mix engines across workflows.
type structuralScanner struct {
	rules []rule
}

// rule is one structural rule. Eval receives the raw frontmatter map
// (`engine:` key included so the engine.env rule can read it per-workflow)
// and returns zero or more rule hits.
type rule struct {
	ID       string
	Severity Severity
	Message  string
	Remedy   string
	Eval     func(fm map[string]any) []ruleHit
}

// ruleHit is one location-bearing match. Line is 1-indexed; 0 means
// "no specific line" (rare; the rule could not localize within the file).
// Detail optionally augments the rule's base Message (e.g. the offending
// host for fleet.mcp.non-standard-server). SeverityOverride is non-nil
// when this hit demotes / promotes off the rule's default Severity —
// used by the engine.env.non-allowlist rule to emit INFO on the FR-018
// missing-engine path while keeping HIGH for the normal allowlist-miss
// path.
type ruleHit struct {
	Line             int
	Detail           string
	SeverityOverride *Severity
}

// infoSev is a small helper for rule eval functions that need to demote
// a hit to INFO without exposing pointer-fiddling at every call site.
func infoSev() *Severity {
	s := SeverityInfo
	return &s
}

func newStructuralScanner() *structuralScanner {
	return &structuralScanner{rules: structuralRules()}
}

// Scan walks <cloneDir>/.github/workflows/*.md, parses each frontmatter,
// and runs every rule against it. Malformed frontmatter emits one INFO
// finding per affected file ("fleet.frontmatter.parse-error") and the
// remaining rules are skipped for that file (other scanners — gitleaks,
// actionlint — still run on the file because they read the raw bytes).
func (s *structuralScanner) Scan(_ context.Context, cloneDir string) []Finding {
	var out []Finding
	for _, w := range walkWorkflows(cloneDir, ".md") {
		out = append(out, s.scanFile(w)...)
	}
	return out
}

func (s *structuralScanner) scanFile(w walkEntry) []Finding {
	content, err := os.ReadFile(w.Full)
	if err != nil {
		return nil
	}
	fmText, _ := frontmatter.Split(string(content))
	fm, parseErr := frontmatter.Parse(fmText)
	if parseErr != nil {
		return []Finding{frontmatterParseFinding(w.Rel, parseErr)}
	}
	var out []Finding
	for _, r := range s.rules {
		for _, h := range r.Eval(fm) {
			out = append(out, hitToFinding(r, h, w.Rel))
		}
	}
	return out
}

// frontmatterParseFinding constructs the INFO finding emitted when a
// workflow's YAML frontmatter is unparseable.
func frontmatterParseFinding(file string, err error) Finding {
	return Finding{
		RuleID:   ruleIDFrontmatterParseError,
		Severity: SeverityInfo,
		File:     file,
		Line:     0,
		Message: fmt.Sprintf(
			"frontmatter could not be parsed: %v; "+
				"structural rules skipped for this workflow",
			err,
		),
		Remedy: "Review the workflow's YAML frontmatter for syntax errors.",
	}
}

// hitToFinding converts one rule + ruleHit into a Finding, applying the
// rule's default severity unless the hit overrides it (FR-018 INFO path).
func hitToFinding(r rule, h ruleHit, file string) Finding {
	msg := r.Message
	if h.Detail != "" {
		msg = h.Detail
	}
	sev := r.Severity
	if h.SeverityOverride != nil {
		sev = *h.SeverityOverride
	}
	return Finding{
		RuleID:   r.ID,
		Severity: sev,
		File:     file,
		Line:     h.Line,
		Message:  msg,
		Remedy:   r.Remedy,
	}
}

// structuralRules returns the v1 rule table. Each rule's Eval receives the
// parsed frontmatter map; the engine.env rule reads `engine:` itself
// per-workflow (FR-018).
func structuralRules() []rule {
	return []rule{
		writeOnScheduleRule(),
		draftFalseRule(),
		missingProtectedFilesRule(),
		engineEnvNonAllowlistRule(),
		repoMemoryMainBranchRule(),
		mcpNonStandardServerRule(),
	}
}

// writeOnScheduleRule fires HIGH when the workflow has any write/admin
// permission scope AND its `on:` triggers include `schedule` or
// `workflow_run`. Schedule-triggered workflows with write permissions are
// the operational shape of a supply-chain compromise (a malicious upstream
// commit lands without an interactive trigger).
func writeOnScheduleRule() rule {
	return rule{
		ID:       ruleIDPermissionsWriteOnSchedule,
		Severity: SeverityHigh,
		Message:  "workflow has write/admin permissions and a schedule or workflow_run trigger",
		Remedy:   "Schedule-triggered workflows with write permissions are the operational shape of a supply-chain compromise. Restrict permissions to read-only or remove the schedule trigger.",
		Eval: func(fm map[string]any) []ruleHit {
			if !hasWriteOrAdminPermission(fm["permissions"]) {
				return nil
			}
			if !hasScheduleOrWorkflowRunTrigger(fm["on"]) {
				return nil
			}
			return []ruleHit{{Line: 0}}
		},
	}
}

// hasWriteOrAdminPermission walks the permissions tree (string scope or
// map) and returns true on any "write" or "admin" value. The shorthand
// `permissions: write-all` or `permissions: read-all` is also recognized.
func hasWriteOrAdminPermission(v any) bool {
	switch p := v.(type) {
	case string:
		return p == "write-all"
	case map[string]any:
		for _, val := range p {
			if s, ok := val.(string); ok && (s == "write" || s == "admin") {
				return true
			}
		}
	}
	return false
}

// hasScheduleOrWorkflowRunTrigger inspects the `on:` block for the two
// triggers that turn this rule on. Accepts the string, list, and map forms
// GitHub Actions allows.
func hasScheduleOrWorkflowRunTrigger(v any) bool {
	switch t := v.(type) {
	case string:
		return t == "schedule" || t == "workflow_run"
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok && (s == "schedule" || s == "workflow_run") {
				return true
			}
		}
	case map[string]any:
		_, sched := t["schedule"]
		_, wfRun := t["workflow_run"]
		return sched || wfRun
	}
	return false
}

// draftFalseRule fires MEDIUM when safe-outputs.create-pull-request.draft
// is explicitly false. Draft PRs require human action to leave draft;
// non-draft means the agent's output enters the merge funnel directly.
func draftFalseRule() rule {
	return rule{
		ID:       ruleIDSafeOutputsDraftFalse,
		Severity: SeverityMedium,
		Message:  "safe-outputs.create-pull-request.draft is set to false",
		Remedy:   "Use draft: true so PRs require human approval before transitioning to non-draft.",
		Eval: func(fm map[string]any) []ruleHit {
			cpr := nestedMap(fm, "safe-outputs", "create-pull-request")
			if cpr == nil {
				return nil
			}
			v, ok := cpr["draft"]
			if !ok {
				return nil
			}
			b, isBool := v.(bool)
			if !isBool || b {
				return nil
			}
			return []ruleHit{{Line: 0}}
		},
	}
}

// missingProtectedFilesRule fires MEDIUM when safe-outputs.create-pull-request
// exists but has no protected-files key. Without the list, the agent can
// edit any path under the repo via the PR.
func missingProtectedFilesRule() rule {
	return rule{
		ID:       ruleIDSafeOutputsMissingProtected,
		Severity: SeverityMedium,
		Message:  "safe-outputs.create-pull-request block has no protected-files key",
		Remedy:   "Add a protected-files list to safe-outputs.create-pull-request to prevent the agent from modifying sensitive paths.",
		Eval: func(fm map[string]any) []ruleHit {
			cpr := nestedMap(fm, "safe-outputs", "create-pull-request")
			if cpr == nil {
				return nil
			}
			if _, ok := cpr["protected-files"]; ok {
				return nil
			}
			return []ruleHit{{Line: 0}}
		},
	}
}

// engineEnvNonAllowlistRule fires HIGH for each `engine.env.<KEY>: ${{ secrets.<NAME> }}`
// entry whose <NAME> is not in the engine's ADR-26919 allowlist. When the
// workflow's `engine:` is missing or unknown, the rule emits one INFO
// finding for the workflow and skips itself for that workflow (FR-018).
func engineEnvNonAllowlistRule() rule {
	return rule{
		ID:       ruleIDEngineEnvNonAllowlist,
		Severity: SeverityHigh,
		Message:  "engine.env references a secret outside the ADR-26919 allowlist",
		Remedy:   "Either remove the engine.env entry or add the secret to the engine's ADR-26919 allowlist upstream.",
		Eval: func(fm map[string]any) []ruleHit {
			env := engineEnvMap(fm)
			if len(env) == 0 {
				return nil
			}
			engineID := frontmatterEngine(fm)
			allowed, known := adr26919Allowlist[engineID]
			if !known {
				return []ruleHit{{
					Line:             0,
					SeverityOverride: infoSev(),
					Detail: fmt.Sprintf(
						"engine.env.non-allowlist rule skipped: engine %q is missing or not recognized",
						engineID,
					),
				}}
			}
			var hits []ruleHit
			for key, val := range env {
				secretName := extractSecretRef(val)
				if secretName == "" {
					continue
				}
				if allowed[secretName] {
					continue
				}
				hits = append(hits, ruleHit{
					Line: 0,
					Detail: fmt.Sprintf(
						"engine.env.%s references secret %q which is not in the %s engine allowlist (ADR-26919)",
						key, secretName, engineID,
					),
				})
			}
			return hits
		},
	}
}

// repoMemoryMainBranchRule fires HIGH when repo-memory.branch-name is the
// default branch. The agent must not write its working memory to the
// default branch — that mixes agent-authored commits with human-authored
// commits in the protected branch's history.
func repoMemoryMainBranchRule() rule {
	return rule{
		ID:       ruleIDRepoMemoryMainBranch,
		Severity: SeverityHigh,
		Message:  "repo-memory.branch-name targets the default branch",
		Remedy:   "Set repo-memory.branch-name to a dedicated branch (e.g. agent-memory). The agent must not write to the default branch.",
		Eval: func(fm map[string]any) []ruleHit {
			rm, ok := fm["repo-memory"].(map[string]any)
			if !ok {
				return nil
			}
			name, ok := rm["branch-name"].(string)
			if !ok {
				return nil
			}
			if name != "main" && name != "master" {
				return nil
			}
			return []ruleHit{{
				Line: 0,
				Detail: fmt.Sprintf(
					"repo-memory.branch-name is %q, which is the default branch",
					name,
				),
			}}
		},
	}
}

// mcpAllowlist hardcodes the v1 GitHub-hosted-only allowlist for MCP
// server hosts. npm, pypi, and other registries are deliberately excluded
// as known typosquat / supply-chain channels (FR-019).
//
//nolint:gochecknoglobals // immutable allowlist set
var mcpAllowlist = map[string]bool{
	"github.com":                true,
	"githubusercontent.com":     true,
	"raw.githubusercontent.com": true,
}

// mcpNonStandardServerRule fires HIGH for each MCP server entry whose
// host is outside mcpAllowlist. Each non-allowlisted host is one finding.
func mcpNonStandardServerRule() rule {
	return rule{
		ID:       ruleIDMCPNonStandardServer,
		Severity: SeverityHigh,
		Message:  "MCP server entry references a host outside the v1 allowlist",
		Remedy:   "Verify the MCP server's provenance. v1 allowlists only GitHub-served hosts to mitigate npm/registry typosquat risk. A future fleet.json allowlist extension will allow per-fleet opt-in.",
		Eval: func(fm map[string]any) []ruleHit {
			servers := mcpServers(fm)
			var hits []ruleHit
			for _, host := range servers {
				if mcpAllowlist[host] {
					continue
				}
				hits = append(hits, ruleHit{
					Line: 0,
					Detail: fmt.Sprintf(
						"MCP server entry references host %q, outside the v1 allowlist {github.com, githubusercontent.com, raw.githubusercontent.com}",
						host,
					),
				})
			}
			return hits
		},
	}
}

// frontmatterEngine resolves the workflow's engine ID from the
// `engine:` key in either string form (`engine: claude`) or map form
// (`engine: { id: claude, ... }`). Returns "" when missing.
func frontmatterEngine(fm map[string]any) string {
	v, ok := fm["engine"]
	if !ok {
		return ""
	}
	if s, isStr := v.(string); isStr {
		return s
	}
	if m, isMap := v.(map[string]any); isMap {
		if s, isStr := m["id"].(string); isStr {
			return s
		}
	}
	return ""
}

// engineEnvMap returns the engine.env map (KEY → templated-secret string)
// from either the top-level `engine.env:` form or the nested
// `engine: { env: {...} }` form. Returns nil when neither exists.
func engineEnvMap(fm map[string]any) map[string]any {
	if engMap, ok := fm["engine"].(map[string]any); ok {
		if env, hasEnv := engMap["env"].(map[string]any); hasEnv {
			return env
		}
	}
	if env, ok := fm["engine.env"].(map[string]any); ok {
		return env
	}
	return nil
}

// extractSecretRef parses a `${{ secrets.NAME }}` template expression and
// returns NAME. Returns "" when the value is not such an expression
// (literals, env-var refs, etc.).
func extractSecretRef(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	s = strings.TrimSpace(s)
	const prefix = "${{"
	const suffix = "}}"
	if !strings.HasPrefix(s, prefix) || !strings.HasSuffix(s, suffix) {
		return ""
	}
	inner := strings.TrimSpace(s[len(prefix) : len(s)-len(suffix)])
	const secretsPrefix = "secrets."
	if !strings.HasPrefix(inner, secretsPrefix) {
		return ""
	}
	return strings.TrimSpace(inner[len(secretsPrefix):])
}

// mcpServers extracts the host portion of every MCP server reference in
// the workflow's `mcp-servers` block. Supports two upstream shapes:
//   - mcp-servers: { name: { url: "https://host/..." } }
//   - mcp-servers: { name: { command: "...", args: [...] } } where args
//     contains a registry URL (we only inspect url-bearing entries; opaque
//     commands are out of scope for v1).
func mcpServers(fm map[string]any) []string {
	srvs, ok := fm["mcp-servers"].(map[string]any)
	if !ok {
		return nil
	}
	var hosts []string
	for _, raw := range srvs {
		entry, isMap := raw.(map[string]any)
		if !isMap {
			continue
		}
		if u, urlOK := entry["url"].(string); urlOK {
			hosts = append(hosts, hostFromURL(u))
		}
		if args, argsOK := entry["args"].([]any); argsOK {
			for _, a := range args {
				s, isStr := a.(string)
				if !isStr {
					continue
				}
				if strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://") {
					hosts = append(hosts, hostFromURL(s))
				}
			}
		}
	}
	return hosts
}

// hostFromURL extracts the host portion of a URL string without using
// net/url (cheaper for the common GitHub-hosted case and tolerant of
// malformed input — the rule then flags malformed-host as non-standard).
func hostFromURL(u string) string {
	for _, scheme := range []string{"https://", "http://"} {
		if strings.HasPrefix(u, scheme) {
			rest := u[len(scheme):]
			if i := strings.IndexAny(rest, "/?#"); i >= 0 {
				return rest[:i]
			}
			return rest
		}
	}
	return u
}

// nestedMap is a small helper that walks a nested map[string]any path and
// returns the leaf map, or nil if any intermediate key is missing or not
// a map.
func nestedMap(fm map[string]any, keys ...string) map[string]any {
	cur := fm
	for _, k := range keys {
		v, ok := cur[k]
		if !ok {
			return nil
		}
		cur, ok = v.(map[string]any)
		if !ok {
			return nil
		}
	}
	return cur
}
