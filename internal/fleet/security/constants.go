package security

const (
	engineClaude   = "claude"
	engineCodex    = "codex"
	engineCopilot  = "copilot"
	engineCrush    = "crush"
	engineGemini   = "gemini"
	engineOpencode = "opencode"
)

const (
	secretAnthropicAPIKey    = "ANTHROPIC_API_KEY"    // #nosec G101 -- secret name, not credential value.
	secretCodexAPIKey        = "CODEX_API_KEY"        // #nosec G101 -- secret name, not credential value.
	secretCopilotGitHubToken = "COPILOT_GITHUB_TOKEN" // #nosec G101 -- secret name, not credential value.
	secretGeminiAPIKey       = "GEMINI_API_KEY"       // #nosec G101 -- secret name, not credential value.
	secretOpenAIAPIKey       = "OPENAI_API_KEY"       // #nosec G101 -- secret name, not credential value.
)

const (
	severityHighLabel   = "HIGH"
	severityInfoLabel   = "INFO"
	severityLowLabel    = "LOW"
	severityMediumLabel = "MEDIUM"
)

const (
	ruleIDActionlintNotInstalled      = "actionlint:not-installed"
	ruleIDEngineEnvNonAllowlist       = "fleet.engine.env.non-allowlist"
	ruleIDFrontmatterParseError       = "fleet.frontmatter.parse-error"
	ruleIDMCPNonStandardServer        = "fleet.mcp.non-standard-server"
	ruleIDPermissionsWriteOnSchedule  = "fleet.permissions.write-on-schedule"
	ruleIDRepoMemoryMainBranch        = "fleet.repo-memory.main-branch"
	ruleIDSafeOutputsDraftFalse       = "fleet.safe-outputs.draft-false"
	ruleIDSafeOutputsMissingProtected = "fleet.safe-outputs.missing-protected-files"
)

const (
	rulePrefixActionlint = "actionlint:"
	rulePrefixGitleaks   = "gitleaks:"
)
