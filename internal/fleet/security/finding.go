package security

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/rshade/gh-aw-fleet/internal/fleet/fleetdiag"
)

// Severity is a typed int describing the urgency of a Finding. Higher values
// are more severe and sort first; INFO is reserved for purely informational
// notes (skipped scanners, unknown engine, malformed frontmatter).
type Severity int

// Severity constants. INFO, LOW, MEDIUM, and HIGH are all emitted by
// current detectors — the Renovate config scanner emits LOW for its
// advisory conflict findings (FR-015).
const (
	SeverityInfo   Severity = 0
	SeverityLow    Severity = 1
	SeverityMedium Severity = 2
	SeverityHigh   Severity = 3
)

// String returns the severity's display name in upper case
// ("INFO" / "LOW" / "MEDIUM" / "HIGH").
func (s Severity) String() string {
	switch s {
	case SeverityHigh:
		return severityHighLabel
	case SeverityMedium:
		return severityMediumLabel
	case SeverityLow:
		return severityLowLabel
	case SeverityInfo:
		return severityInfoLabel
	default:
		return fmt.Sprintf("UNKNOWN(%d)", int(s))
	}
}

// Finding is the rich internal type every scanner emits. It flows across
// all three output surfaces (stderr, JSON envelope, PR body) via
// ToDiagnostic and the Render* helpers.
type Finding struct {
	// RuleID is a namespaced identifier; e.g. "gitleaks:aws-access-key"
	// or "fleet.permissions.write-on-schedule". Required, non-empty.
	RuleID string `json:"rule_id"`
	// Severity is one of SeverityInfo / SeverityLow / SeverityMedium /
	// SeverityHigh.
	Severity Severity `json:"severity"`
	// File is a path relative to the work-dir clone root, e.g.
	// ".github/workflows/foo.md". Empty only for INFO findings that
	// have no file context (e.g. actionlint:not-installed).
	File string `json:"file"`
	// Line is the 1-indexed source line; 0 means "no specific line"
	// (rare; INFO scanner-skip findings).
	Line int `json:"line"`
	// Message is a human-readable description. For credential findings
	// it MUST contain "<redacted>" not the matched literal (FR-008a).
	Message string `json:"message"`
	// Remedy is single-sentence operator guidance.
	Remedy string `json:"remedy"`
}

// Scanner is the v1 detector interface. Implementations MUST tolerate
// missing or malformed input by emitting INFO findings rather than
// panicking or returning errors.
type Scanner interface {
	// Scan walks the workflow content available on disk in the clone-dir,
	// returning zero or more findings. Implementations MUST NOT modify
	// any file.
	Scan(ctx context.Context, cloneDir string) []Finding
}

// Run scans every workflow in the work-dir clone and returns sorted
// findings. A scanner run that finds nothing returns a non-nil empty slice
// so callers can distinguish "scanner ran clean" from "scanner did not run."
//
// Run is non-fatal: scanner panics are converted to a single INFO
// finding ("fleet.scanner.panic") and the run continues with the
// remaining scanners. Run never returns an error.
//
// Engine resolution for the engine.env.non-allowlist rule (FR-018) is
// per-workflow: the structural scanner reads each workflow's `engine:`
// key from its own frontmatter. There is no fleet-level engine parameter.
func Run(ctx context.Context, cloneDir string) []Finding {
	scanners := defaultScanners()
	return runWithScanners(ctx, cloneDir, scanners)
}

// runWithScanners is the test seam used by Run. It accepts an explicit
// scanner list so tests can inject panicking or stub scanners without
// touching the v1 default scanner registration.
func runWithScanners(ctx context.Context, cloneDir string, scanners []Scanner) []Finding {
	combined := make([]Finding, 0)
	for _, s := range scanners {
		combined = append(combined, safeScan(ctx, cloneDir, s)...)
	}
	sort.SliceStable(combined, func(i, j int) bool {
		if combined[i].Severity != combined[j].Severity {
			return combined[i].Severity > combined[j].Severity
		}
		if combined[i].File != combined[j].File {
			return combined[i].File < combined[j].File
		}
		if combined[i].Line != combined[j].Line {
			return combined[i].Line < combined[j].Line
		}
		if combined[i].RuleID != combined[j].RuleID {
			return combined[i].RuleID < combined[j].RuleID
		}
		if combined[i].Message != combined[j].Message {
			return combined[i].Message < combined[j].Message
		}
		return combined[i].Remedy < combined[j].Remedy
	})
	return combined
}

// safeScan calls one scanner, converting any panic into a single INFO
// finding so one detector cannot abort Run for the others (FR-016).
func safeScan(ctx context.Context, cloneDir string, s Scanner) []Finding {
	var result []Finding
	func() {
		defer func() {
			if r := recover(); r != nil {
				result = []Finding{{
					RuleID:   "fleet.scanner.panic",
					Severity: SeverityInfo,
					Message:  fmt.Sprintf("scanner panicked: %v; scanner skipped", r),
					Remedy:   "Open an issue with the workflow content that triggered the panic.",
				}}
			}
		}()
		result = s.Scan(ctx, cloneDir)
	}()
	return result
}

// ToDiagnostic projects a Finding into the cross-surface Diagnostic shape
// consumed by the JSON envelope's warnings[] (cmd/output.go). The return
// type is the leaf-package fleetdiag.Diagnostic — re-exported via type
// alias from internal/fleet as fleet.Diagnostic — so callers can use
// either alias.
func (f Finding) ToDiagnostic() fleetdiag.Diagnostic {
	return fleetdiag.Diagnostic{
		Code:    diagCodeForRuleID(f.RuleID),
		Message: f.Message,
		Fields: map[string]any{
			"severity": f.Severity.String(),
			"rule_id":  f.RuleID,
			"file":     f.File,
			"line":     f.Line,
			"remedy":   f.Remedy,
		},
	}
}

// diagCodeForRuleID maps a Finding's RuleID to its family diagnostic constant
// in fleet/diagnostics.go. Defensive fallback to DiagHint for unknown prefixes
// (should never happen given the rule table is closed).
func diagCodeForRuleID(ruleID string) string {
	switch {
	case strings.HasPrefix(ruleID, rulePrefixGitleaks):
		return fleetdiag.DiagSecurityCredential
	case ruleID == ruleIDPermissionsWriteOnSchedule:
		return fleetdiag.DiagSecurityWriteOnSchedule
	case ruleID == ruleIDSafeOutputsDraftFalse:
		return fleetdiag.DiagSecurityDraftFalse
	case ruleID == ruleIDSafeOutputsMissingProtected:
		return fleetdiag.DiagSecurityMissingProtectedFiles
	case ruleID == ruleIDEngineEnvNonAllowlist:
		return fleetdiag.DiagSecurityEngineEnvNonAllowlist
	case ruleID == ruleIDRepoMemoryMainBranch:
		return fleetdiag.DiagSecurityRepoMemoryMain
	case ruleID == ruleIDMCPNonStandardServer:
		return fleetdiag.DiagSecurityMCPNonStandardHost
	case strings.HasPrefix(ruleID, rulePrefixActionlint):
		return fleetdiag.DiagSecurityActionlint
	case ruleID == ruleIDFrontmatterParseError:
		return fleetdiag.DiagSecurityFrontmatterParseError
	case strings.HasPrefix(ruleID, rulePrefixRenovate):
		return fleetdiag.DiagSecurityRenovate
	case strings.HasPrefix(ruleID, rulePrefixDependabot):
		return fleetdiag.DiagSecurityDependabot
	default:
		return fleetdiag.DiagHint
	}
}

// defaultScanners constructs the v1 scanner list in the canonical order:
// gitleaks → structural → actionlint → renovate → dependabot. Each scanner's
// constructor cost (gitleaks regex compilation, actionlint exec.LookPath)
// is paid once per Run invocation. Run sorts the combined findings, so the
// registration order does not affect output ordering.
func defaultScanners() []Scanner {
	return []Scanner{
		newGitleaksScanner(),
		newStructuralScanner(),
		newActionlintScanner(),
		newRenovateScanner(),
		newDependabotScanner(),
	}
}
