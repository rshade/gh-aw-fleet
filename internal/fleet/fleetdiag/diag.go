// Package fleetdiag holds the cross-surface Diagnostic shape and the
// stable diagnostic-code constants. It exists as a leaf package — no
// dependencies on internal/fleet — so that internal/fleet AND its
// sub-packages (e.g. internal/fleet/security) can depend on Diagnostic
// without import cycles.
//
// internal/fleet re-exports the constants and the type via aliases, so
// existing call sites (cmd/, tests) continue to use the fleet.Diagnostic
// / fleet.Diag* surface unchanged.
package fleetdiag

// Diagnostic is the shared shape for warnings and hints embedded in the
// JSON envelope (cmd/output.go) and emitted on stderr via zerolog.
type Diagnostic struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

// Stable diagnostic codes. Snake_case identifiers consumed by downstream
// agents to gate on classes of warning/hint without parsing free-form text.
// Security_* entries are one code per rule family; per-rule granularity
// rides on Diagnostic.Fields["rule_id"]. See internal/fleet/security/.
const (
	DiagMissingSecret                 = "missing_secret"
	DiagActionsDisabled               = "actions_disabled"
	DiagWorkflowTokenReadOnly         = "workflow_token_read_only"
	DiagDriftDetected                 = "drift_detected"
	DiagHint                          = "hint"
	DiagUnknownProperty               = "unknown_property"
	DiagHTTP404                       = "http_404"
	DiagGPGFailure                    = "gpg_failure"
	DiagBillingQuotaExceeded          = "billing_quota_exceeded"
	DiagRateLimited                   = "rate_limited"
	DiagRepoInaccessible              = "repo_inaccessible"
	DiagNetworkUnreachable            = "network_unreachable"
	DiagEmptyFleet                    = "empty_fleet"
	DiagSecurityCredential            = "security_credential"
	DiagSecurityWriteOnSchedule       = "security_write_on_schedule"
	DiagSecurityDraftFalse            = "security_draft_false"
	DiagSecurityMissingProtectedFiles = "security_missing_protected_files"
	DiagSecurityEngineEnvNonAllowlist = "security_engine_env_non_allowlist"
	DiagSecurityRepoMemoryMain        = "security_repo_memory_main"
	DiagSecurityMCPNonStandardHost    = "security_mcp_non_standard_host"
	DiagSecurityActionlint            = "security_actionlint"
	DiagSecurityFrontmatterParseError = "security_frontmatter_parse_error"

	// DiagCompileStrictFailed fires when `gh aw compile --strict` exits non-zero
	// because one or more workflows violate strict-mode validation. The hint
	// names the work-dir clone path for inspection and the `compile_strict: false`
	// opt-out path in fleet.local.json.
	DiagCompileStrictFailed = "compile_strict_failed"
	// DiagGhAwTooOld fires when `gh aw compile --help` does not advertise the
	// `--strict` flag — the installed `gh aw` extension predates the minimum
	// supported version (v0.79.2). The hint names `gh extension upgrade aw`.
	DiagGhAwTooOld = "gh_aw_too_old"
	// DiagGhAwMissing fires when the `gh aw compile --help` probe itself fails
	// (binary not found or exec error). The hint names
	// `gh extension install github/gh-aw`.
	DiagGhAwMissing = "gh_aw_missing"
)
