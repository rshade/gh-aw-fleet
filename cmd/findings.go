package cmd

import (
	zlog "github.com/rs/zerolog/log"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
	"github.com/rshade/gh-aw-fleet/internal/fleet/security"
)

// emitSecurityFindingWarnings emits one stderr Warn line per security
// finding using the shared zerolog field shape (rule_id, severity, file,
// line, remedy + Message). Used by deploy/sync/upgrade so all three
// surfaces describe findings identically.
func emitSecurityFindingWarnings(findings []security.Finding) {
	for _, f := range findings {
		zlog.Warn().
			Str("rule_id", f.RuleID).
			Str("severity", f.Severity.String()).
			Str("file", f.File).
			Int("line", f.Line).
			Str("remedy", f.Remedy).
			Msg(f.Message)
	}
}

// appendFindingDiagnostics projects each finding into a Diagnostic and
// appends to dst. Shared by the deploy/sync/upgrade JSON-envelope builders.
func appendFindingDiagnostics(dst []fleet.Diagnostic, findings []security.Finding) []fleet.Diagnostic {
	for _, f := range findings {
		dst = append(dst, f.ToDiagnostic())
	}
	return dst
}
