package security

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tailscale/hujson"
)

// renovateScanner inspects a managed repo's clone for a Renovate
// configuration and warns when that config lacks the two rules the fleet
// needs so Renovate does not fight gh-aw-managed pins: (A) disabling
// updates to the gh-aw-actions package family, and (B) excluding the
// generated *.lock.yml files. It is read-only and advisory — conflict
// findings emit at LOW severity, an unparseable config emits one INFO
// finding; it never mutates a file, returns an error, or panics (Scanner
// contract).
type renovateScanner struct{}

// newRenovateScanner constructs the Renovate config scanner. It holds no
// state — probe/parse/detect all run per Scan call against the clone dir.
func newRenovateScanner() *renovateScanner {
	return &renovateScanner{}
}

// renovateGhAwActionsMarker is the substring that signals intent to disable
// the gh-aw-actions package family (Rule A); renovateLockfileMarker is the
// substring that signals intent to exclude the generated lock files (Rule B).
// Substring matching (not exact match) recognizes the canonical block and the
// common equivalents — alternate match keys, broader globs — without
// re-implementing Renovate's glob/regex engine (research.md Decision 4).
const (
	renovateGhAwActionsMarker = "gh-aw-actions"
	renovateLockfileMarker    = ".lock.yml"
)

// renovateDefaultConfigName is the highest-priority probe location and the
// most common Renovate config filename.
const renovateDefaultConfigName = "renovate.json"

// renovateConfigNames is the probe order for a repo's Renovate config. The
// first existing path wins; configs are never merged across files (research.md
// Decision 1). Slash-form here doubles as the clone-relative Finding.File.
//
//nolint:gochecknoglobals // immutable probe-order list
var renovateConfigNames = []string{
	renovateDefaultConfigName,
	"renovate.json5",
	".renovaterc",
	".renovaterc.json",
	".renovaterc.json5",
	".github/renovate.json",
	".github/renovate.json5",
}

// renovateRuleARemedy is the canonical packageRules block that disables
// gh-aw-actions bumps. Quoted verbatim in the Rule A finding so an operator
// can paste it directly (research.md Decision 6 / contract C5 — byte-for-byte).
const renovateRuleARemedy = `{
  "matchPackageNames": [
    "github/gh-aw-actions",
    "github/gh-aw-actions/setup",
    "github/gh-aw/actions/setup-cli"
  ],
  "enabled": false,
  "description": "gh-aw-actions version is coupled to the gh aw compiler version baked into lock file metadata. Renovate bumping it directly breaks hash validation. Managed atomically by the fleet tool via: gh aw upgrade"
}`

// renovateRuleBRemedy is the canonical packageRules block that keeps Renovate
// off the generated lock files. Quoted verbatim in the Rule B finding
// (research.md Decision 6 / contract C5 — byte-for-byte).
const renovateRuleBRemedy = `{
  "description": "gh-aw generates .github/workflows/*.lock.yml via 'gh aw compile'. Their pinned action SHAs and container tags are managed by gh-aw (recompiling bumps them), so Renovate must not touch them: edits would be reverted on the next compile and can break the gh-aw integrity manifest.",
  "matchFileNames": [
    ".github/workflows/*.lock.yml"
  ],
  "enabled": false
}`

// stringOrSlice tolerates a JSON value that is either a single string or an
// array of strings. Renovate matchers historically accept both forms, so a
// scalar matcher must not become a parse error that masquerades as a
// malformed config (data-model.md parsing notes).
type stringOrSlice []string

// UnmarshalJSON accepts both `"x"` and `["x", "y"]`, plus null (→ nil).
func (s *stringOrSlice) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*s = nil
		return nil
	}
	if trimmed[0] == '[' {
		var arr []string
		if err := json.Unmarshal(data, &arr); err != nil {
			return err
		}
		*s = arr
		return nil
	}
	var single string
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	*s = []string{single}
	return nil
}

// renovateConfig is the minimal subset of a Renovate config the scanner
// reads. Unknown fields are ignored (standard encoding/json behavior).
type renovateConfig struct {
	Enabled      *bool         `json:"enabled"`     // root-level false ⇒ Renovate off ⇒ both rules satisfied
	IgnoreDeps   []string      `json:"ignoreDeps"`  // Rule A: substring gh-aw-actions ⇒ present
	IgnorePaths  []string      `json:"ignorePaths"` // Rule B: substring .lock.yml ⇒ present
	PackageRules []packageRule `json:"packageRules"`
}

// packageRule is one entry of the top-level packageRules array. Only entries
// with enabled:false count as a disable signal for the matcher checks.
type packageRule struct {
	Enabled              *bool         `json:"enabled"`
	MatchPackageNames    stringOrSlice `json:"matchPackageNames"`    // Rule A (current)
	MatchPackagePatterns stringOrSlice `json:"matchPackagePatterns"` // Rule A (deprecated)
	MatchPackagePrefixes stringOrSlice `json:"matchPackagePrefixes"` // Rule A (deprecated)
	MatchDepNames        stringOrSlice `json:"matchDepNames"`        // Rule A (equivalent)
	MatchDepPatterns     stringOrSlice `json:"matchDepPatterns"`     // Rule A (equivalent)
	MatchFileNames       stringOrSlice `json:"matchFileNames"`       // Rule B (current)
	MatchPaths           stringOrSlice `json:"matchPaths"`           // Rule B (deprecated)
}

// Scan probes the clone for a Renovate config (first match per probe order
// wins), parses it tolerantly (JWCC via hujson), and emits one LOW finding
// per missing conflict rule. No config present → nil. Unparseable config →
// one INFO finding. Root enabled:false → nil (Renovate disabled repo-wide).
func (s *renovateScanner) Scan(_ context.Context, cloneDir string) []Finding {
	rel, full, found := probeRenovateConfig(cloneDir)
	if !found {
		return nil
	}
	cfg, parseErr := parseRenovateConfig(full)
	if parseErr != nil {
		return []Finding{renovateParseFinding(rel, parseErr)}
	}
	if cfg.Enabled != nil && !*cfg.Enabled {
		return nil
	}
	var out []Finding
	if !ruleAPresent(cfg) {
		out = append(out, renovateGhAwActionsFinding(rel))
	}
	if !ruleBPresent(cfg) {
		out = append(out, renovateLockfileFinding(rel))
	}
	return out
}

// probeRenovateConfig returns the first existing config path in probe order.
// The first result is the clone-relative slash-form path (used as
// Finding.File); the second is the OS path to read; the third is false when
// no recognized config exists.
func probeRenovateConfig(cloneDir string) (string, string, bool) {
	for _, name := range renovateConfigNames {
		candidate := filepath.Join(cloneDir, filepath.FromSlash(name))
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return name, candidate, true
		}
	}
	return "", "", false
}

// parseRenovateConfig reads the config bytes, standardizes JWCC syntax via
// hujson, and unmarshals into the minimal struct. Any read/standardize/decode
// failure is returned as an error so Scan can map it to the INFO parse finding.
func parseRenovateConfig(full string) (*renovateConfig, error) {
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	std, stdErr := hujson.Standardize(data)
	if stdErr != nil {
		return nil, stdErr
	}
	var cfg renovateConfig
	if jsonErr := json.Unmarshal(std, &cfg); jsonErr != nil {
		return nil, jsonErr
	}
	return &cfg, nil
}

// ruleAPresent reports whether the config expresses intent to disable the
// gh-aw-actions package family — via a disabling packageRule with a package
// matcher containing the marker, or a top-level ignoreDeps entry.
func ruleAPresent(cfg *renovateConfig) bool {
	if containsSubstring(cfg.IgnoreDeps, renovateGhAwActionsMarker) {
		return true
	}
	for _, pr := range cfg.PackageRules {
		if !isDisabled(pr.Enabled) {
			continue
		}
		if anyContains(renovateGhAwActionsMarker,
			pr.MatchPackageNames, pr.MatchPackagePatterns, pr.MatchPackagePrefixes,
			pr.MatchDepNames, pr.MatchDepPatterns) {
			return true
		}
	}
	return false
}

// ruleBPresent reports whether the config expresses intent to exclude the
// generated lock files — via a disabling packageRule with a file matcher
// containing the marker, or a top-level ignorePaths entry.
func ruleBPresent(cfg *renovateConfig) bool {
	if containsSubstring(cfg.IgnorePaths, renovateLockfileMarker) {
		return true
	}
	for _, pr := range cfg.PackageRules {
		if !isDisabled(pr.Enabled) {
			continue
		}
		if anyContains(renovateLockfileMarker, pr.MatchFileNames, pr.MatchPaths) {
			return true
		}
	}
	return false
}

// isDisabled reports whether an enabled pointer is an explicit false.
func isDisabled(enabled *bool) bool {
	return enabled != nil && !*enabled
}

// containsSubstring reports whether any element of values contains marker.
func containsSubstring(values []string, marker string) bool {
	for _, v := range values {
		if strings.Contains(v, marker) {
			return true
		}
	}
	return false
}

// anyContains reports whether any element of any group contains marker.
func anyContains(marker string, groups ...stringOrSlice) bool {
	for _, g := range groups {
		if containsSubstring(g, marker) {
			return true
		}
	}
	return false
}

// renovateGhAwActionsFinding builds the LOW finding emitted when Rule A is
// absent, quoting the canonical Rule A block as remediation.
func renovateGhAwActionsFinding(file string) Finding {
	return Finding{
		RuleID:   ruleIDRenovateGhAwActionsNotDisabled,
		Severity: SeverityLow,
		File:     file,
		Line:     0,
		Message:  "Renovate config does not disable updates to the gh-aw-actions package family; bumping it directly breaks gh-aw lock-file hash validation",
		Remedy:   renovateRuleARemedy,
	}
}

// renovateLockfileFinding builds the LOW finding emitted when Rule B is
// absent, quoting the canonical Rule B block as remediation.
func renovateLockfileFinding(file string) Finding {
	return Finding{
		RuleID:   ruleIDRenovateLockfileNotDisabled,
		Severity: SeverityLow,
		File:     file,
		Line:     0,
		Message:  "Renovate config does not exclude the generated .github/workflows/*.lock.yml files; Renovate edits to them are reverted on the next gh aw compile and can break the integrity manifest",
		Remedy:   renovateRuleBRemedy,
	}
}

// renovateParseFinding builds the INFO finding emitted when the config cannot
// be parsed; conflict checks are skipped for that file (never blocks).
func renovateParseFinding(file string, err error) Finding {
	return Finding{
		RuleID:   ruleIDRenovateParseError,
		Severity: SeverityInfo,
		File:     file,
		Line:     0,
		Message:  fmt.Sprintf("Renovate config could not be parsed: %v; conflict checks skipped for this file", err),
		Remedy:   "Review the Renovate config for JSON syntax errors.",
	}
}
