package fleet

import (
	"os"
	"path/filepath"
	"testing"
)

func mustReadFailureFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "failures", name))
	if err != nil {
		t.Fatalf("read failure fixture %s: %v", name, err)
	}
	return string(b)
}

func TestParseModelFailure(t *testing.T) {
	modelBody := mustReadFailureFixture(t, "model_not_supported.txt")
	unrelatedBody := mustReadFailureFixture(t, "unrelated.txt")

	cases := []struct {
		name string
		body string
		want bool
	}{
		{name: "fixture model_not_supported_error", body: modelBody, want: true},
		{name: "fixture unrelated agent_failure only", body: unrelatedBody, want: false},
		{
			name: "runtime class model_not_supported without _error suffix",
			body: "<!-- gh-aw-failure-issue: true, failure_categories: model_not_supported -->",
			want: true,
		},
		{
			name: "token present but marker absent",
			body: "failure_categories: agent_failure|model_not_supported_error (no gh-aw marker here)",
			want: false,
		},
		{
			name: "no-op runs issue lacks the failure marker",
			body: "## [aw] No-Op Runs\n\nA daily digest of no-op agentic runs.\n",
			want: false,
		},
		{
			name: "marker present but no failure_categories key",
			body: "<!-- gh-aw-failure-issue: true, workflow_id: x, branch: main -->",
			want: false,
		},
		{
			name: "categories value stops at comma before the token",
			body: "<!-- gh-aw-failure-issue: true, failure_categories: agent_failure, note: model_not_supported -->",
			want: false,
		},
		{
			name: "model token as one of several categories",
			body: "<!-- gh-aw-failure-issue: true, failure_categories: timeout|model_not_supported_error|agent_failure -->",
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseModelFailure(tc.body); got != tc.want {
				t.Fatalf("parseModelFailure(%q) = %v; want %v", tc.body, got, tc.want)
			}
		})
	}
}

func TestResolveEffectiveModel(t *testing.T) {
	cases := []struct {
		name string
		vars map[string]string
		want string
	}{
		{
			name: "agent override wins",
			vars: map[string]string{
				varAgentModelCopilot:   "gpt-5.3-codex",
				varDefaultModelCopilot: "gpt-5-mini",
			},
			want: "gpt-5.3-codex",
		},
		{
			name: "default var when agent unset",
			vars: map[string]string{varDefaultModelCopilot: "gpt-5-mini"},
			want: "gpt-5-mini",
		},
		{
			name: "baked default when both unset",
			vars: map[string]string{},
			want: ghAwBakedDefaultCopilotModel,
		},
		{
			name: "empty-string values are skipped",
			vars: map[string]string{
				varAgentModelCopilot:   "",
				varDefaultModelCopilot: "",
			},
			want: ghAwBakedDefaultCopilotModel,
		},
		{
			name: "empty agent falls through to default var",
			vars: map[string]string{
				varAgentModelCopilot:   "",
				varDefaultModelCopilot: "claude-opus-4",
			},
			want: "claude-opus-4",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveEffectiveModel(tc.vars); got != tc.want {
				t.Fatalf("resolveEffectiveModel(%v) = %q; want %q", tc.vars, got, tc.want)
			}
		})
	}
}
