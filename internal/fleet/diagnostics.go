// Package fleet diagnostic layer scans gh-aw CLI output for known error
// patterns and emits remediation hints.
//
// CollectHintDiagnostics is the base scanner; CollectHints projects its
// output to []string for text-mode consumers. Adding a hint pattern means
// touching one table.
package fleet

import "strings"

// Diagnostic is the shared shape for warnings and hints embedded in the
// JSON envelope (cmd/output.go) and emitted on stderr via zerolog.
type Diagnostic struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

// Stable diagnostic codes. Snake_case identifiers consumed by downstream
// agents to gate on classes of warning/hint without parsing free-form text.
const (
	DiagMissingSecret   = "missing_secret"
	DiagDriftDetected   = "drift_detected"
	DiagHint            = "hint"
	DiagUnknownProperty = "unknown_property"
	DiagHTTP404         = "http_404"
	DiagGPGFailure      = "gpg_failure"
)

// Hint is a remediation suggestion keyed by a substring match against
// gh-aw CLI output.
type Hint struct {
	Pattern string
	Message string
	Code    string
}

// Ordered most-specific first; only the first match per input text is emitted.
//
//nolint:gochecknoglobals // immutable hint table; Go has no const slice of structs
var hints = []Hint{
	{
		Pattern: "Unknown property: mount-as-clis",
		Message: "Workflow uses `mount-as-clis`, an unreleased gh-aw feature. " +
			"`gh extension upgrade gh-aw` if your CLI is out of date; if already latest, " +
			"the upstream is ahead of the release — pin the source to a tagged release (e.g. `@v0.68.3`) " +
			"via `fleet sync --apply --force`.",
		Code: DiagUnknownProperty,
	},
	{
		Pattern: "Unknown property:",
		Message: "Workflow uses a property your installed `gh aw` CLI doesn't recognize. " +
			"Try `gh extension upgrade gh-aw`, or pin the workflow source to a tagged release.",
		Code: DiagUnknownProperty,
	},
	{
		Pattern: "HTTP 404",
		Message: "Source path not found. Check the spec — `github/gh-aw` workflows live under `.github/workflows/`; " +
			"`githubnext/agentics` workflows live under `workflows/`.",
		Code: DiagHTTP404,
	},
	{
		Pattern: "gpg failed to sign",
		Message: "gpg-agent couldn't prompt for a passphrase in this non-interactive context. " +
			"Unlock gpg-agent in your shell (`echo test | gpg -as > /dev/null`) and re-run.",
		Code: DiagGPGFailure,
	},
}

// CollectHintDiagnostics scans output text for known error patterns and
// returns one Diagnostic per matched hint (deduped by message, ordered by
// first appearance; most-specific hint wins per input string). Returns a
// non-nil empty slice when no patterns match (JSON arrays never null).
func CollectHintDiagnostics(texts ...string) []Diagnostic {
	seen := map[string]bool{}
	out := []Diagnostic{}
	for _, t := range texts {
		for _, h := range hints {
			if strings.Contains(t, h.Pattern) {
				if !seen[h.Message] {
					out = append(out, Diagnostic{
						Code:    h.Code,
						Message: h.Message,
						Fields:  map[string]any{"hint": h.Message},
					})
					seen[h.Message] = true
				}
				break
			}
		}
	}
	return out
}

// CollectHints returns just the message strings from CollectHintDiagnostics
// for text-mode consumers.
func CollectHints(texts ...string) []string {
	diags := CollectHintDiagnostics(texts...)
	if len(diags) == 0 {
		return nil
	}
	out := make([]string, 0, len(diags))
	for _, d := range diags {
		out = append(out, d.Message)
	}
	return out
}

// HintFromError wraps err as a free-form Diagnostic using the DiagHint code.
// Used by subcommands when an error blocks result construction and there's
// no structured context to attach — the error message becomes both Message
// and Fields.hint (the latter for jq filter convenience).
func HintFromError(err error) Diagnostic {
	msg := err.Error()
	return Diagnostic{
		Code:    DiagHint,
		Message: msg,
		Fields:  map[string]any{"hint": msg},
	}
}
