package fleet

const addToken = "add"

const logsToken = "logs"

const (
	engineCopilot = "copilot"
	engineClaude  = "claude"
	engineCodex   = "codex"
	engineGemini  = "gemini"
)

const (
	secretAnthropicAPIKey    = "ANTHROPIC_API_KEY"    // #nosec G101 -- secret name, not credential value.
	secretCopilotGitHubToken = "COPILOT_GITHUB_TOKEN" // #nosec G101 -- secret name, not credential value.
	secretGeminiAPIKey       = "GEMINI_API_KEY"       // #nosec G101 -- secret name, not credential value.
	secretOpenAIAPIKey       = "OPENAI_API_KEY"       // #nosec G101 -- secret name, not credential value.
)

const (
	keyURLAnthropic = "https://console.anthropic.com/settings/keys"
	keyURLCopilot   = "https://github.com/settings/personal-access-tokens/new"
	keyURLOpenAI    = "https://platform.openai.com/api-keys"
	keyURLGemini    = "https://aistudio.google.com/app/apikey"
)

const (
	sourceGitHubAW = "github/gh-aw"
	sourceAgentics = "githubnext/agentics"
)

const (
	fieldCloneDir = "clone_dir"
	fieldGroup    = "group"
	fieldHint     = "hint"
	fieldPath     = "path"
	fieldPeriod   = "period"
	fieldRepo     = "repo"
	fieldWorkflow = "workflow_id"
)

const (
	branchMain   = "main"
	branchMaster = "master"
)

const (
	workflowPermissionRead  = "read"
	workflowPermissionWrite = "write"
)
