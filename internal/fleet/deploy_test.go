package fleet

import (
	"context"
	"slices"
	"testing"
)

func TestCheckEngineSecret(t *testing.T) {
	origGhAPIExists := ghAPIExists
	t.Cleanup(func() { ghAPIExists = origGhAPIExists })

	tests := []struct {
		name        string
		repo        string
		engine      string
		existsPaths map[string]bool
		wantSecret  string
		wantKeyURL  string
		wantCalls   []string
	}{
		{
			name:       "unknown engine returns empty and makes no API calls",
			repo:       "acme/widgets",
			engine:     "fictional-engine",
			wantSecret: "",
			wantKeyURL: "",
			wantCalls:  nil,
		},
		{
			name:   "secret exists at repo level - org check skipped",
			repo:   "acme/widgets",
			engine: "claude",
			existsPaths: map[string]bool{
				"/repos/acme/widgets/actions/secrets/ANTHROPIC_API_KEY": true,
			},
			wantSecret: "",
			wantKeyURL: "",
			wantCalls:  []string{"/repos/acme/widgets/actions/secrets/ANTHROPIC_API_KEY"},
		},
		{
			name:   "secret exists at org level only - falls back successfully",
			repo:   "acme/widgets",
			engine: "claude",
			existsPaths: map[string]bool{
				"/orgs/acme/actions/secrets/ANTHROPIC_API_KEY": true,
			},
			wantSecret: "",
			wantKeyURL: "",
			wantCalls: []string{
				"/repos/acme/widgets/actions/secrets/ANTHROPIC_API_KEY",
				"/orgs/acme/actions/secrets/ANTHROPIC_API_KEY",
			},
		},
		{
			name:        "secret missing at both levels returns secret name and URL",
			repo:        "alice/widgets",
			engine:      "claude",
			existsPaths: nil,
			wantSecret:  "ANTHROPIC_API_KEY",
			wantKeyURL:  "https://console.anthropic.com/settings/keys",
			wantCalls: []string{
				"/repos/alice/widgets/actions/secrets/ANTHROPIC_API_KEY",
				"/orgs/alice/actions/secrets/ANTHROPIC_API_KEY",
			},
		},
		{
			name:   "copilot engine uses COPILOT_GITHUB_TOKEN",
			repo:   "acme/widgets",
			engine: "copilot",
			existsPaths: map[string]bool{
				"/repos/acme/widgets/actions/secrets/COPILOT_GITHUB_TOKEN": true,
			},
			wantSecret: "",
			wantKeyURL: "",
			wantCalls:  []string{"/repos/acme/widgets/actions/secrets/COPILOT_GITHUB_TOKEN"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			ghAPIExists = func(_ context.Context, path string) bool {
				calls = append(calls, path)
				return tt.existsPaths[path]
			}

			gotSecret, gotKeyURL := checkEngineSecret(context.Background(), tt.repo, tt.engine)

			if gotSecret != tt.wantSecret {
				t.Errorf("secret = %q, want %q", gotSecret, tt.wantSecret)
			}
			if gotKeyURL != tt.wantKeyURL {
				t.Errorf("keyURL = %q, want %q", gotKeyURL, tt.wantKeyURL)
			}
			if !slices.Equal(calls, tt.wantCalls) {
				t.Errorf("API calls = %v, want %v", calls, tt.wantCalls)
			}
		})
	}
}
