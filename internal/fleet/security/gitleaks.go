// gitleaks adapter — wraps github.com/zricethezav/gitleaks/v8
// (pinned at v8.30.1 in go.mod). The adapter constructs the
// *detect.Detector once via NewDetectorDefaultConfig (~300–500ms regex
// compile) and reuses it across all workflows in a single Run
// invocation. Per FR-008a the matched secret literal is NEVER
// propagated into Finding.Message — gleak.Secret is not read; Message
// is constructed from the rule's Description plus the literal
// "<redacted>".

package security

import (
	"context"
	"fmt"
	"os"

	zlog "github.com/rs/zerolog/log"
	"github.com/zricethezav/gitleaks/v8/detect"
)

// gitleaksScanner wraps a *detect.Detector. The detector is built once per
// Run; nil if NewDetectorDefaultConfig failed at construction (rare, but
// graceful: Scan returns one INFO finding instead of panicking).
type gitleaksScanner struct {
	detector *detect.Detector
	initErr  error
}

// newGitleaksScanner builds the detector with the gitleaks default ruleset
// (~200 regex patterns). Constructor never panics; any error is captured on
// the struct and surfaced as one INFO finding from the first Scan call.
func newGitleaksScanner() *gitleaksScanner {
	d, err := detect.NewDetectorDefaultConfig()
	return &gitleaksScanner{detector: d, initErr: err}
}

// Scan walks <cloneDir>/.github/workflows/*.md, runs each file through the
// detector, and returns one HIGH-severity Finding per match. The matched
// literal (gleak.Secret) is intentionally not read; the message uses
// rule Description + " (<redacted>)" per FR-008a.
func (s *gitleaksScanner) Scan(_ context.Context, cloneDir string) []Finding {
	if s.initErr != nil {
		return []Finding{{
			RuleID:   "gitleaks:init-error",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("gitleaks detector failed to initialize: %v; credential scanner skipped", s.initErr),
			Remedy:   "Open an issue with the build environment so the gitleaks default config can be debugged.",
		}}
	}
	var out []Finding
	for _, w := range walkWorkflows(cloneDir, ".md") {
		content, readErr := os.ReadFile(w.Full)
		if readErr != nil {
			zlog.Debug().Str("file", w.Full).Err(readErr).Msg("gitleaks scanner: read failed; skipping file")
			continue
		}
		for _, gf := range s.detector.DetectBytes(content) {
			out = append(out, Finding{
				RuleID:   "gitleaks:" + gf.RuleID,
				Severity: SeverityHigh,
				File:     w.Rel,
				Line:     gf.StartLine,
				Message:  fmt.Sprintf("%s (<redacted>)", gf.Description),
				Remedy:   "Rotate the credential. Remove from source. Use the engine.env / GitHub Actions secrets mechanism to inject at runtime.",
			})
		}
	}
	return out
}
