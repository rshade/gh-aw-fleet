// ADR-26919 allowlist port. ADR-26919 itself does not transcribe the
// engine→secret table; it specifies that conformant codemods MUST call
// getSecretRequirementsForEngine(engine, includeSystemSecrets=false,
// includeOptional=false). The actual data lives in upstream
// github.com/github/gh-aw at pkg/constants/engine_constants.go
// (`EngineOptions` table). Pinned to upstream commit SHA
// b469d2e5bb4340b9ab2e1d93f1bfcaefbbf92109 of that file (verified
// 2026-04-30). Drift is caught by TestADR26919AllowlistMatchesFixture
// against testdata/security/adr-26919-allowlist.json.

package security

// adr26919Allowlist maps engine ID → set of allowed secret names referenced
// via engine.env. Values are SecretName + AlternativeSecrets per upstream
// EngineOptions.
//
//nolint:gochecknoglobals // immutable allowlist table
var adr26919Allowlist = map[string]map[string]bool{
	engineClaude:   {secretAnthropicAPIKey: true},
	engineCodex:    {secretOpenAIAPIKey: true, secretCodexAPIKey: true},
	engineCopilot:  {secretCopilotGitHubToken: true},
	engineGemini:   {secretGeminiAPIKey: true},
	engineOpencode: {secretCopilotGitHubToken: true, secretAnthropicAPIKey: true, secretGeminiAPIKey: true},
	engineCrush:    {secretCopilotGitHubToken: true, secretAnthropicAPIKey: true, secretGeminiAPIKey: true},
}
