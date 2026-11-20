package fleet

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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
		name        string
		vars        map[string]string
		wantModel   string
		wantControl string
	}{
		{
			name: "agent override wins",
			vars: map[string]string{
				varAgentModelCopilot:   "gpt-5.3-codex",
				varDefaultModelCopilot: "gpt-5-mini",
			},
			wantModel:   "gpt-5.3-codex",
			wantControl: varAgentModelCopilot,
		},
		{
			name:        "default var when agent unset",
			vars:        map[string]string{varDefaultModelCopilot: "gpt-5-mini"},
			wantModel:   "gpt-5-mini",
			wantControl: varDefaultModelCopilot,
		},
		{
			name:        "baked default when both unset is fixed via the default var",
			vars:        map[string]string{},
			wantModel:   ghAwBakedDefaultCopilotModel,
			wantControl: varDefaultModelCopilot,
		},
		{
			name: "empty-string values are skipped",
			vars: map[string]string{
				varAgentModelCopilot:   "",
				varDefaultModelCopilot: "",
			},
			wantModel:   ghAwBakedDefaultCopilotModel,
			wantControl: varDefaultModelCopilot,
		},
		{
			name: "empty agent falls through to default var",
			vars: map[string]string{
				varAgentModelCopilot:   "",
				varDefaultModelCopilot: "claude-opus-4",
			},
			wantModel:   "claude-opus-4",
			wantControl: varDefaultModelCopilot,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model, control := resolveEffectiveModel(tc.vars)
			if model != tc.wantModel || control != tc.wantControl {
				t.Fatalf("resolveEffectiveModel(%v) = (%q, %q); want (%q, %q)",
					tc.vars, model, control, tc.wantModel, tc.wantControl)
			}
		})
	}
}

func TestMergeActionsVariables(t *testing.T) {
	org := map[string]string{
		varAgentModelCopilot:   "org-agent",
		varDefaultModelCopilot: "org-default",
	}
	repo := map[string]string{varDefaultModelCopilot: "repo-default"}

	got := mergeActionsVariables(org, repo)

	if got[varDefaultModelCopilot] != "repo-default" {
		t.Fatalf("repo variable must override org: got %q", got[varDefaultModelCopilot])
	}
	if got[varAgentModelCopilot] != "org-agent" {
		t.Fatalf("org-only variable must be inherited: got %q", got[varAgentModelCopilot])
	}
	if org[varDefaultModelCopilot] != "org-default" {
		t.Fatal("merge mutated the org input map")
	}
}

func TestModelSupportRemediationHint(t *testing.T) {
	cases := []struct {
		name           string
		controllingVar string
		wantSetVar     string
		wantPrecedence bool
	}{
		{name: "default var controls", controllingVar: varDefaultModelCopilot, wantSetVar: varDefaultModelCopilot},
		{
			name:           "agent var controls",
			controllingVar: varAgentModelCopilot,
			wantSetVar:     varAgentModelCopilot,
			wantPrecedence: true,
		},
		{name: "variables unreadable", controllingVar: "", wantSetVar: varDefaultModelCopilot, wantPrecedence: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hint := modelSupportRemediationHint(tc.controllingVar)
			if !strings.Contains(hint, "Set the "+tc.wantSetVar+" Actions variable") {
				t.Fatalf("hint does not tell the operator to set %s: %q", tc.wantSetVar, hint)
			}
			if !strings.Contains(hint, varDefaultModelCopilot) {
				t.Fatalf("hint must reference %s (FR-004): %q", varDefaultModelCopilot, hint)
			}
			if got := strings.Contains(hint, "takes precedence"); got != tc.wantPrecedence {
				t.Fatalf("hint precedence note present = %v; want %v: %q", got, tc.wantPrecedence, hint)
			}
		})
	}
}

func TestIsGitHubValidationFailed(t *testing.T) {
	userOwned := errors.New("gh api: gh: Validation Failed (HTTP 422)")
	if !isGitHubValidationFailed(userOwned) {
		t.Fatalf("HTTP 422 must classify as validation-failed")
	}
	if isGitHubValidationFailed(errors.New("gh api: HTTP 403")) || isGitHubValidationFailed(nil) {
		t.Fatalf("non-422 errors must not classify as validation-failed")
	}
}
