package security

import (
	"context"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// dependabotScanner inspects a managed repo's clone for a Dependabot
// configuration and warns when a github-actions ecosystem update entry does
// not ignore the gh-aw-actions action family. An out-of-band Dependabot bump
// of that family rewrites the generated *.lock.yml files and desyncs them from
// the gh aw compiler version baked into lock-file metadata. It is read-only
// and advisory — each unprotected entry emits one LOW finding, an unparseable
// config emits one INFO finding; it never mutates a file, returns an error, or
// panics (Scanner contract).
//
// Unlike the Renovate sibling (two conflict rules), Dependabot ignores by
// dependency name only — there is no file-glob analog to the *.lock.yml
// exclusion — so this scanner has exactly one conflict rule, and its remedy
// must educate the operator about the name-only protection.
type dependabotScanner struct{}

// newDependabotScanner constructs the Dependabot config scanner. It holds no
// state — probe/parse/detect all run per Scan call against the clone dir.
func newDependabotScanner() *dependabotScanner {
	return &dependabotScanner{}
}

// dependabotGhAwActionNames is the gh-aw action family that must be fully
// ignored for a github-actions update entry to be protected.
//
//nolint:gochecknoglobals // immutable list used for coverage checks
var dependabotGhAwActionNames = [...]string{
	"github/gh-aw-actions",
	"github/gh-aw-actions/setup",
	"github/gh-aw/actions/setup-cli",
}

// dependabotGitHubActionsEcosystem is the canonical package-ecosystem
// identifier for the entries this scanner evaluates.
const dependabotGitHubActionsEcosystem = "github-actions"

// dependabotGhAwActionsMessageFmt is the conflict finding's Message template;
// the single verb is the entry's directory label.
const dependabotGhAwActionsMessageFmt = "Dependabot github-actions entry (directory %q) does not ignore the gh-aw-actions action family; an out-of-band bump rewrites the generated *.lock.yml files and desyncs them from the gh aw compiler version"

// dependabotConfigNames is the probe order for a repo's Dependabot config.
// Dependabot reads only .github/dependabot.yml (or .yaml); the first existing
// path wins and configs are never merged (research.md Decision 1). Slash-form
// here doubles as the clone-relative Finding.File.
//
//nolint:gochecknoglobals // immutable probe-order list
var dependabotConfigNames = []string{
	".github/dependabot.yml",
	".github/dependabot.yaml",
}

// dependabotGhAwActionsRemedy is the copy-pasteable guidance emitted in the
// conflict finding's Remedy. It quotes the canonical ignore: block (FR-010)
// AND carries the name-only caveat (FR-004): Dependabot has no file-glob
// equivalent to a *.lock.yml exclusion, so the lock files stay reachable by
// name. Reproduced in contracts/scanner-contract.md C5.
const dependabotGhAwActionsRemedy = `Add an ignore block to the github-actions update entry:
    ignore:
      - dependency-name: "github/gh-aw-actions"
      - dependency-name: "github/gh-aw-actions/setup"
      - dependency-name: "github/gh-aw/actions/setup-cli"
Note: Dependabot can only ignore by dependency NAME — it has no file-glob equivalent to a *.lock.yml exclusion, so the generated lock files remain reachable if any action they reference is independently named. gh-aw-actions is coupled to the gh aw compiler version and is managed atomically by the fleet via 'gh aw upgrade'.`

// dependabotConfig is the minimal document-root subset the scanner reads.
// Unknown fields are ignored (KnownFields is intentionally left off).
type dependabotConfig struct {
	Updates []dependabotUpdate `yaml:"updates"`
}

// dependabotUpdate is one element of the updates list — a single ecosystem
// update entry. Only github-actions entries are evaluated.
type dependabotUpdate struct {
	PackageEcosystem      string             `yaml:"package-ecosystem"`        // gate: only "github-actions" entries are evaluated
	Directory             string             `yaml:"directory"`                // finding label; classic single-dir form
	Directories           []string           `yaml:"directories"`              // finding label fallback; newer multi-dir/glob form
	OpenPullRequestsLimit *int               `yaml:"open-pull-requests-limit"` // 0 ⇒ entry cannot open bump PRs ⇒ protected
	Ignore                []dependabotIgnore `yaml:"ignore"`                   // full unscoped name coverage ⇒ protected
}

// dependabotIgnore is one element of an entry's ignore list.
type dependabotIgnore struct {
	DependencyName string   `yaml:"dependency-name"` // exact name or * wildcard pattern
	Versions       []string `yaml:"versions"`        // scopes the ignore to matching versions only
	UpdateTypes    []string `yaml:"update-types"`    // scopes the ignore to selected SemVer update types only
}

// Scan probes the clone for a Dependabot config (first match per probe order
// wins), parses it as YAML, and emits one LOW finding per unprotected
// github-actions entry. No config present → nil. Unparseable config → one INFO
// finding. A config with no github-actions entry → nil.
func (s *dependabotScanner) Scan(_ context.Context, cloneDir string) []Finding {
	rel, full, found := probeConfigFile(cloneDir, dependabotConfigNames)
	if !found {
		return nil
	}
	cfg, parseErr := parseDependabotConfig(full)
	if parseErr != nil {
		return []Finding{dependabotParseFinding(rel, parseErr)}
	}
	var out []Finding
	for _, entry := range cfg.Updates {
		if entry.PackageEcosystem != dependabotGitHubActionsEcosystem {
			continue
		}
		if dependabotEntryProtected(entry) {
			continue
		}
		out = append(out, dependabotGhAwActionsFinding(rel, dependabotEntryLabel(entry)))
	}
	return out
}

// parseDependabotConfig reads the config bytes and unmarshals the YAML into the
// minimal struct. Any read/decode failure is returned as an error so Scan can
// map it to the INFO parse finding. KnownFields is intentionally not enabled —
// unknown keys are ignored, not rejected.
func parseDependabotConfig(full string) (*dependabotConfig, error) {
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	var cfg dependabotConfig
	if unmarshalErr := yaml.Unmarshal(data, &cfg); unmarshalErr != nil {
		return nil, unmarshalErr
	}
	return &cfg, nil
}

// dependabotEntryProtected reports whether a github-actions entry fully keeps
// Dependabot off the gh-aw action family — either by zeroing
// open-pull-requests-limit so the entry cannot open bump PRs at all, or by
// covering every known gh-aw action name with unscoped ignore rules.
func dependabotEntryProtected(entry dependabotUpdate) bool {
	if entry.OpenPullRequestsLimit != nil && *entry.OpenPullRequestsLimit == 0 {
		return true
	}
	covered := make(map[string]bool, len(dependabotGhAwActionNames))
	for _, ig := range entry.Ignore {
		if dependabotIgnoreScoped(ig) {
			continue
		}
		for _, dep := range dependabotGhAwActionNames {
			if dependabotDependencyNameMatches(ig.DependencyName, dep) {
				covered[dep] = true
			}
		}
	}
	return len(covered) == len(dependabotGhAwActionNames)
}

// dependabotIgnoreScoped reports whether an ignore entry leaves some matching
// dependency updates enabled through version or update-type filters.
func dependabotIgnoreScoped(ig dependabotIgnore) bool {
	return len(ig.Versions) > 0 || len(ig.UpdateTypes) > 0
}

// dependabotDependencyNameMatches applies Dependabot's dependency-name wildcard
// semantics for the subset this scanner needs: * matches zero or more
// characters, including slashes in GitHub Actions dependency names.
func dependabotDependencyNameMatches(pattern, dependencyName string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	dependencyName = strings.ToLower(strings.TrimSpace(dependencyName))
	if pattern == "" {
		return false
	}
	if !strings.Contains(pattern, "*") {
		return pattern == dependencyName
	}
	return wildcardMatch(pattern, dependencyName)
}

// wildcardMatch reports whether pattern matches value when * means zero or
// more characters.
func wildcardMatch(pattern, value string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == value
	}
	if prefix := parts[0]; prefix != "" {
		if !strings.HasPrefix(value, prefix) {
			return false
		}
		value = value[len(prefix):]
	}
	for _, part := range parts[1 : len(parts)-1] {
		if part == "" {
			continue
		}
		idx := strings.Index(value, part)
		if idx == -1 {
			return false
		}
		value = value[idx+len(part):]
	}
	suffix := parts[len(parts)-1]
	return suffix == "" || strings.HasSuffix(value, suffix)
}

// dependabotEntryLabel names an update entry for its finding Message, keeping
// multiple per-entry findings distinct. Prefers the classic directory, falls
// back to the first directories value, then to a placeholder.
func dependabotEntryLabel(entry dependabotUpdate) string {
	if entry.Directory != "" {
		return entry.Directory
	}
	if len(entry.Directories) > 0 {
		return entry.Directories[0]
	}
	return "(unspecified directory)"
}

// dependabotGhAwActionsFinding builds the LOW finding emitted for an
// unprotected github-actions entry, naming the entry's directory and quoting
// the canonical ignore: block plus the name-only caveat as remediation.
func dependabotGhAwActionsFinding(file, label string) Finding {
	return Finding{
		RuleID:   ruleIDDependabotGhAwActionsNotIgnored,
		Severity: SeverityLow,
		File:     file,
		Line:     0,
		Message:  fmt.Sprintf(dependabotGhAwActionsMessageFmt, label),
		Remedy:   dependabotGhAwActionsRemedy,
	}
}

// dependabotParseFinding builds the INFO finding emitted when the config cannot
// be parsed; conflict checks are skipped for that file (never blocks).
func dependabotParseFinding(file string, err error) Finding {
	return Finding{
		RuleID:   ruleIDDependabotParseError,
		Severity: SeverityInfo,
		File:     file,
		Line:     0,
		Message:  fmt.Sprintf("Dependabot config could not be parsed: %v; conflict checks skipped for this file", err),
		Remedy:   "Review the Dependabot config for YAML syntax errors.",
	}
}
