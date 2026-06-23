package fleet

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rshade/gh-aw-fleet/internal/fleet/security"
)

const promptInjectionRulePrefix = "promptinj:"

// SecurityOpts controls invocation-scoped security policy.
type SecurityOpts struct {
	Strict bool // block on HIGH non-promptinj security findings
}

// StrictSecurityError is returned when the opt-in strict security gate blocks.
type StrictSecurityError struct {
	Repo               string             // repository owner/name being processed
	BlockingCount      int                // number of blocking HIGH Layer 1 findings
	BlockingFindings   []security.Finding // blocking HIGH non-promptinj findings
	BreadcrumbPath     string             // path where findings.json was written or attempted
	BreadcrumbWriteErr error              // write failure, when findings.json could not be saved
}

// Error returns the actionable strict security gate failure message.
func (e *StrictSecurityError) Error() string {
	if e == nil {
		return ""
	}
	msg := fmt.Sprintf(
		"strict security gate blocked %d HIGH Layer 1 finding(s) for %s; "+
			"fix the findings or re-run without --strict to proceed advisory-only",
		e.BlockingCount,
		e.Repo,
	)
	if e.BreadcrumbPath == "" {
		return msg
	}
	if e.BreadcrumbWriteErr != nil {
		return fmt.Sprintf(
			"%s (failed to save findings to %s: %v)",
			msg,
			e.BreadcrumbPath,
			e.BreadcrumbWriteErr,
		)
	}
	return fmt.Sprintf("%s (findings saved to %s)", msg, e.BreadcrumbPath)
}

// Unwrap returns the breadcrumb write error, when one occurred.
func (e *StrictSecurityError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.BreadcrumbWriteErr
}

// BlockingSecurityFindings returns HIGH findings that strict mode treats as blockers.
//
// HIGH findings whose rule ID starts with "promptinj:" are intentionally
// excluded: prompt-injection findings are Layer 3 advisory results, not Layer 1
// blockers. The returned slice preserves the input order and does not mutate
// the original findings.
func BlockingSecurityFindings(findings []security.Finding) []security.Finding {
	var blockers []security.Finding
	for _, finding := range findings {
		if finding.Severity != security.SeverityHigh {
			continue
		}
		if strings.HasPrefix(finding.RuleID, promptInjectionRulePrefix) {
			continue
		}
		blockers = append(blockers, finding)
	}
	return blockers
}

// EvaluateStrictSecurityGate writes a findings breadcrumb and returns an error when strict blocks.
func EvaluateStrictSecurityGate(repo, cloneDir string, opts SecurityOpts, findings []security.Finding) error {
	if !opts.Strict {
		return nil
	}
	blockers := BlockingSecurityFindings(findings)
	if len(blockers) == 0 {
		return nil
	}

	breadcrumbPath := filepath.Join(cloneDir, "findings.json")
	writeErr := writeJSON(breadcrumbPath, findings)
	return &StrictSecurityError{
		Repo:               repo,
		BlockingCount:      len(blockers),
		BlockingFindings:   blockers,
		BreadcrumbPath:     breadcrumbPath,
		BreadcrumbWriteErr: writeErr,
	}
}

func evaluateStrictGatePreservingClone(
	repo, cloneDir string,
	opts SecurityOpts,
	findings []security.Finding,
	cleanupClone *bool,
) error {
	err := EvaluateStrictSecurityGate(repo, cloneDir, opts, findings)
	preserveCloneForStrictError(err, cleanupClone)
	return err
}

// IsStrictSecurityError reports whether err is (or wraps) a *StrictSecurityError.
func IsStrictSecurityError(err error) bool {
	var strictErr *StrictSecurityError
	return errors.As(err, &strictErr)
}
