package fleet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"

	"github.com/rshade/gh-aw-fleet/internal/fleet/security"
)

// fakeJSONResponse is one canned ghAPIJSON outcome keyed by request path.
// A nil body + non-nil err means the override returns (nil, err); a non-nil
// body returns (body, nil); a nil body + nil err is a programming bug
// (no override matched the path).
type fakeJSONResponse struct {
	body any
	err  error
}

// withGhAPIJSON installs a closure that consults responses[path]. Calls to
// unregistered paths return an error so missing fixtures surface as test
// failures rather than silent fall-through. Cleanup restores the original.
func withGhAPIJSON(t *testing.T, responses map[string]fakeJSONResponse) {
	t.Helper()
	orig := ghAPIJSON
	t.Cleanup(func() { ghAPIJSON = orig })
	ghAPIJSON = func(_ context.Context, path string) (any, error) {
		r, ok := responses[path]
		if !ok {
			return nil, fmt.Errorf("unexpected ghAPIJSON path: %s", path)
		}
		return r.body, r.err
	}
}

func TestCheckActionsSettings(t *testing.T) {
	const repo = "acme/widgets"
	const actionsPath = "/repos/acme/widgets/actions/permissions"
	const tokenPath = "/repos/acme/widgets/actions/permissions/workflow"

	tests := []struct {
		name              string
		responses         map[string]fakeJSONResponse
		wantActions       bool
		wantTokenReadOnly bool
	}{
		{
			name: "healthy enabled+write returns false/false",
			responses: map[string]fakeJSONResponse{
				actionsPath: {body: map[string]any{"enabled": true}},
				tokenPath:   {body: map[string]any{"default_workflow_permissions": "write"}},
			},
		},
		{
			name: "Actions disabled returns true on first boolean",
			responses: map[string]fakeJSONResponse{
				actionsPath: {body: map[string]any{"enabled": false}},
				tokenPath:   {body: map[string]any{"default_workflow_permissions": "write"}},
			},
			wantActions: true,
		},
		{
			name: "token read returns true on second boolean",
			responses: map[string]fakeJSONResponse{
				actionsPath: {body: map[string]any{"enabled": true}},
				tokenPath:   {body: map[string]any{"default_workflow_permissions": "read"}},
			},
			wantTokenReadOnly: true,
		},
		{
			name: "both disabled+read returns true/true (independence)",
			responses: map[string]fakeJSONResponse{
				actionsPath: {body: map[string]any{"enabled": false}},
				tokenPath:   {body: map[string]any{"default_workflow_permissions": "read"}},
			},
			wantActions:       true,
			wantTokenReadOnly: true,
		},
		{
			name: "write with can_approve_pull_request_reviews=true ignores extra field",
			responses: map[string]fakeJSONResponse{
				actionsPath: {body: map[string]any{"enabled": true}},
				tokenPath: {body: map[string]any{
					"default_workflow_permissions":     "write",
					"can_approve_pull_request_reviews": true,
				}},
			},
		},
		{
			name: "read with can_approve_pull_request_reviews=false still flags read",
			responses: map[string]fakeJSONResponse{
				actionsPath: {body: map[string]any{"enabled": true}},
				tokenPath: {body: map[string]any{
					"default_workflow_permissions":     "read",
					"can_approve_pull_request_reviews": false,
				}},
			},
			wantTokenReadOnly: true,
		},
		// Indeterminate paths — actions/permissions endpoint.
		{
			name: "actions endpoint http_403 returns false/_",
			responses: map[string]fakeJSONResponse{
				actionsPath: {err: errors.New("HTTP 403: forbidden")},
				tokenPath:   {body: map[string]any{"default_workflow_permissions": "write"}},
			},
		},
		{
			name: "actions endpoint http_5xx returns false/_",
			responses: map[string]fakeJSONResponse{
				actionsPath: {err: errors.New("HTTP 503: service unavailable")},
				tokenPath:   {body: map[string]any{"default_workflow_permissions": "write"}},
			},
		},
		{
			name: "actions endpoint missing field returns false/_",
			responses: map[string]fakeJSONResponse{
				actionsPath: {body: map[string]any{}},
				tokenPath:   {body: map[string]any{"default_workflow_permissions": "write"}},
			},
		},
		{
			name: "actions endpoint wrong type returns false/_",
			responses: map[string]fakeJSONResponse{
				actionsPath: {body: map[string]any{"enabled": "yes"}},
				tokenPath:   {body: map[string]any{"default_workflow_permissions": "write"}},
			},
		},
		{
			name: "actions endpoint non-object returns false/_",
			responses: map[string]fakeJSONResponse{
				actionsPath: {body: "not an object"},
				tokenPath:   {body: map[string]any{"default_workflow_permissions": "write"}},
			},
		},
		{
			name: "actions endpoint transport error returns false/_",
			responses: map[string]fakeJSONResponse{
				actionsPath: {err: errors.New("dial tcp: lookup api.github.com: no such host")},
				tokenPath:   {body: map[string]any{"default_workflow_permissions": "write"}},
			},
		},
		// Indeterminate paths — actions/permissions/workflow endpoint.
		{
			name: "token endpoint http_403 returns _/false",
			responses: map[string]fakeJSONResponse{
				actionsPath: {body: map[string]any{"enabled": true}},
				tokenPath:   {err: errors.New("HTTP 403: forbidden")},
			},
		},
		{
			name: "token endpoint http_5xx returns _/false",
			responses: map[string]fakeJSONResponse{
				actionsPath: {body: map[string]any{"enabled": true}},
				tokenPath:   {err: errors.New("HTTP 502: bad gateway")},
			},
		},
		{
			name: "token endpoint missing field returns _/false",
			responses: map[string]fakeJSONResponse{
				actionsPath: {body: map[string]any{"enabled": true}},
				tokenPath:   {body: map[string]any{}},
			},
		},
		{
			name: "token endpoint wrong type returns _/false",
			responses: map[string]fakeJSONResponse{
				actionsPath: {body: map[string]any{"enabled": true}},
				tokenPath:   {body: map[string]any{"default_workflow_permissions": 1}},
			},
		},
		{
			name: "token endpoint non-object returns _/false",
			responses: map[string]fakeJSONResponse{
				actionsPath: {body: map[string]any{"enabled": true}},
				tokenPath:   {body: []any{"not", "an", "object"}},
			},
		},
		{
			name: "token endpoint transport error returns _/false",
			responses: map[string]fakeJSONResponse{
				actionsPath: {body: map[string]any{"enabled": true}},
				tokenPath:   {err: errors.New("connection reset by peer")},
			},
		},
		{
			name: "token endpoint unknown enum value returns _/false",
			responses: map[string]fakeJSONResponse{
				actionsPath: {body: map[string]any{"enabled": true}},
				tokenPath:   {body: map[string]any{"default_workflow_permissions": "none"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withGhAPIJSON(t, tt.responses)
			gotActions, gotToken := checkActionsSettings(context.Background(), repo)
			if gotActions != tt.wantActions {
				t.Errorf("actionsDisabled = %v, want %v", gotActions, tt.wantActions)
			}
			if gotToken != tt.wantTokenReadOnly {
				t.Errorf("workflowTokenReadOnly = %v, want %v", gotToken, tt.wantTokenReadOnly)
			}
		})
	}
}

func TestCheckActionsSettings_DebugLogShape(t *testing.T) {
	const repo = "acme/widgets"
	const actionsPath = "/repos/acme/widgets/actions/permissions"
	const tokenPath = "/repos/acme/widgets/actions/permissions/workflow"

	withGhAPIJSON(t, map[string]fakeJSONResponse{
		actionsPath: {err: errors.New("HTTP 403: forbidden")},
		tokenPath:   {body: map[string]any{"default_workflow_permissions": "write"}},
	})

	var buf bytes.Buffer
	orig := zlog.Logger
	//nolint:reassign // zerolog's documented global-logger-replacement pattern; restored on cleanup
	zlog.Logger = zerolog.New(&buf).Level(zerolog.DebugLevel)
	t.Cleanup(func() {
		//nolint:reassign // restore in cleanup
		zlog.Logger = orig
	})

	_, _ = checkActionsSettings(context.Background(), repo)

	if buf.Len() == 0 {
		t.Fatalf("expected one debug line, got none")
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly one debug line, got %d:\n%s", len(lines), buf.String())
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &obj); err != nil {
		t.Fatalf("debug line not JSON: %v\nraw=%s", err, lines[0])
	}
	for _, key := range []string{"repo", "endpoint", "reason"} {
		if _, ok := obj[key]; !ok {
			t.Errorf("debug log missing field %q (got %v)", key, obj)
		}
	}
	if obj["repo"] != repo {
		t.Errorf("repo field = %v, want %q", obj["repo"], repo)
	}
	if obj["endpoint"] != actionsPath {
		t.Errorf("endpoint field = %v, want %q", obj["endpoint"], actionsPath)
	}
	if obj["reason"] != "http_403" {
		t.Errorf("reason field = %v, want %q", obj["reason"], "http_403")
	}
}

func TestSetupRequiredSection(t *testing.T) {
	const repo = "alice/widgets"
	const heading = "## ⚠ Setup required"
	const settingsURL = "https://github.com/alice/widgets/settings/actions"
	const secretSubBlock = "Engine secret missing on `alice/widgets`"
	const actionsSubBlock = "GitHub Actions is disabled on `alice/widgets`"
	const tokenSubBlock = "Workflow token is read-only on `alice/widgets`"

	tests := []struct {
		name        string
		res         *DeployResult
		wantEmpty   bool
		wantContain []string
		wantAbsent  []string
		// orderedBefore asserts each pair {a, b} appears with a's index < b's index.
		orderedBefore [][2]string
		wantHeadings  int
	}{
		{
			name:      "no findings returns empty",
			res:       &DeployResult{Repo: repo},
			wantEmpty: true,
		},
		{
			name: "actions only — heading + Actions sub-block",
			res:  &DeployResult{Repo: repo, ActionsDisabled: true},
			wantContain: []string{
				heading,
				actionsSubBlock,
				"Enable at: " + settingsURL,
			},
			wantAbsent: []string{
				tokenSubBlock,
				secretSubBlock,
			},
			wantHeadings: 1,
		},
		{
			name: "token only — heading + token sub-block",
			res:  &DeployResult{Repo: repo, WorkflowTokenReadOnly: true},
			wantContain: []string{
				heading,
				tokenSubBlock,
				"Read and write permissions",
			},
			wantAbsent: []string{
				actionsSubBlock,
				secretSubBlock,
			},
			wantHeadings: 1,
		},
		{
			name: "actions and token — fixed order Actions then token",
			res: &DeployResult{
				Repo:                  repo,
				ActionsDisabled:       true,
				WorkflowTokenReadOnly: true,
			},
			wantContain: []string{
				heading,
				actionsSubBlock,
				tokenSubBlock,
			},
			wantAbsent: []string{secretSubBlock},
			orderedBefore: [][2]string{
				{actionsSubBlock, tokenSubBlock},
			},
			wantHeadings: 1,
		},
		{
			name: "all three — Actions then token then secret",
			res: &DeployResult{
				Repo:                  repo,
				ActionsDisabled:       true,
				WorkflowTokenReadOnly: true,
				MissingSecret:         "ANTHROPIC_API_KEY",
				SecretKeyURL:          "https://console.anthropic.com/settings/keys",
			},
			wantContain: []string{
				heading,
				actionsSubBlock,
				tokenSubBlock,
				secretSubBlock,
				"gh secret set ANTHROPIC_API_KEY --repo alice/widgets",
			},
			orderedBefore: [][2]string{
				{actionsSubBlock, tokenSubBlock},
				{tokenSubBlock, secretSubBlock},
			},
			wantHeadings: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := setupRequiredSection(tt.res)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("expected empty string for healthy res; got %q", got)
				}
				return
			}
			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("section missing %q\nfull:\n%s", want, got)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("section unexpectedly contains %q\nfull:\n%s", absent, got)
				}
			}
			for _, pair := range tt.orderedBefore {
				ai := strings.Index(got, pair[0])
				bi := strings.Index(got, pair[1])
				if ai < 0 || bi < 0 {
					t.Errorf("ordering check missing substrings: %q@%d, %q@%d", pair[0], ai, pair[1], bi)
					continue
				}
				if ai >= bi {
					t.Errorf("expected %q to appear before %q (got indices %d, %d)", pair[0], pair[1], ai, bi)
				}
			}
			if tt.wantHeadings > 0 {
				count := strings.Count(got, heading)
				if count != tt.wantHeadings {
					t.Errorf("heading count = %d, want %d\nfull:\n%s", count, tt.wantHeadings, got)
				}
			}
		})
	}
}

func TestPRBodyContainsSetupRequiredSection(t *testing.T) {
	res := &DeployResult{
		Repo:                  "alice/widgets",
		ActionsDisabled:       true,
		WorkflowTokenReadOnly: true,
		Added: []WorkflowOutcome{
			{Name: "ci-doctor", Spec: "githubnext/agentics/ci-doctor@main"},
		},
	}
	got := prBody(res, res.Repo, len(res.Added))

	const heading = "## ⚠ Setup required"
	if count := strings.Count(got, heading); count != 1 {
		t.Errorf("expected exactly one %q heading; got %d\nbody:\n%s", heading, count, got)
	}
	for _, want := range []string{
		"GitHub Actions is disabled on `alice/widgets`",
		"Workflow token is read-only on `alice/widgets`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("PR body missing %q\nbody:\n%s", want, got)
		}
	}
	actionsIdx := strings.Index(got, "GitHub Actions is disabled")
	tokenIdx := strings.Index(got, "Workflow token is read-only")
	addedIdx := strings.Index(got, "## Added")
	if actionsIdx < 0 || tokenIdx < 0 || addedIdx < 0 {
		t.Fatalf("ordering substrings missing: actions=%d token=%d added=%d", actionsIdx, tokenIdx, addedIdx)
	}
	if !(actionsIdx < tokenIdx && tokenIdx < addedIdx) {
		t.Errorf("expected actions < token < ##Added; got %d, %d, %d", actionsIdx, tokenIdx, addedIdx)
	}
}

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

// newTestRepo creates a temp git repo with an initial commit and optional setup.
func newTestRepo(t *testing.T, setup func(dir string)) string {
	t.Helper()
	dir := t.TempDir()
	git := func(arg ...string) {
		t.Helper()
		cmd := exec.Command("git", arg...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v in %s: %v", arg, dir, err)
		}
	}
	git("init")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test")
	git("config", "commit.gpgsign", "false")
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	git("add", "README.md")
	git("commit", "-m", "init")
	if setup != nil {
		setup(dir)
	}
	return dir
}

func TestHasStagedOrUnstagedWorkflowChanges(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name  string
		setup func(dir string)
		want  bool
	}{
		{
			name:  "clean repo",
			setup: nil,
			want:  false,
		},
		{
			name: "staged workflow file",
			setup: func(dir string) {
				p := filepath.Join(dir, ".github", "workflows", "test.md")
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(p, []byte("workflow\n"), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
				cmd := exec.Command("git", "add", p)
				cmd.Dir = dir
				if err := cmd.Run(); err != nil {
					t.Fatalf("git add: %v", err)
				}
			},
			want: true,
		},
		{
			name: "unstaged workflow file",
			setup: func(dir string) {
				p := filepath.Join(dir, ".github", "workflows", "test.md")
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(p, []byte("workflow\n"), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
			},
			want: true,
		},
		{
			name: "staged non-workflow file is ignored",
			setup: func(dir string) {
				p := filepath.Join(dir, "foo.txt")
				if err := os.WriteFile(p, []byte("foo\n"), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
				cmd := exec.Command("git", "add", p)
				cmd.Dir = dir
				if err := cmd.Run(); err != nil {
					t.Fatalf("git add: %v", err)
				}
			},
			want: false,
		},
		{
			name: "non-workflow change alongside .github change still detects",
			setup: func(dir string) {
				if err := os.WriteFile(filepath.Join(dir, "foo.txt"), []byte("foo\n"), 0o644); err != nil {
					t.Fatalf("write foo: %v", err)
				}
				wp := filepath.Join(dir, ".github", "workflows", "test.md")
				if err := os.MkdirAll(filepath.Dir(wp), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(wp, []byte("workflow\n"), 0o644); err != nil {
					t.Fatalf("write workflow: %v", err)
				}
			},
			want: true,
		},
		{
			name: "mixed staged and unstaged workflow",
			setup: func(dir string) {
				p := filepath.Join(dir, ".github", "workflows", "mix.md")
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(p, []byte("v1\n"), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
				cmd := exec.Command("git", "add", p)
				cmd.Dir = dir
				if err := cmd.Run(); err != nil {
					t.Fatalf("git add: %v", err)
				}
				if err := os.WriteFile(p, []byte("v2\n"), 0o644); err != nil {
					t.Fatalf("rewrite: %v", err)
				}
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newTestRepo(t, tt.setup)
			got, err := hasStagedOrUnstagedWorkflowChanges(ctx, dir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGitCurrentBranch(t *testing.T) {
	ctx := context.Background()

	if _, err := gitCurrentBranch(ctx, t.TempDir()); err == nil {
		t.Error("expected error for non-git repo")
	}

	dir := newTestRepo(t, nil)
	branch, err := gitCurrentBranch(ctx, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch == "" {
		t.Error("expected non-empty branch name")
	}
}

func TestIsDefaultBranch(t *testing.T) {
	tests := []struct {
		branch string
		want   bool
	}{
		{"main", true},
		{"master", true},
		{"Main", false},
		{"fleet/deploy-2024-01-01-000000", false},
		{"feature/foo", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			if got := isDefaultBranch(tt.branch); got != tt.want {
				t.Errorf("isDefaultBranch(%q) = %v, want %v", tt.branch, got, tt.want)
			}
		})
	}
}

func TestCommittedWorkflowNames(t *testing.T) {
	ctx := context.Background()
	dir := newTestRepo(t, nil)

	names, err := committedWorkflowNames(ctx, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("got %v, want empty", names)
	}

	wp := filepath.Join(dir, ".github", "workflows", "ci.md")
	if err := os.MkdirAll(filepath.Dir(wp), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(wp, []byte("workflow\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	for _, args := range [][]string{
		{"add", wp},
		{"commit", "-m", "add workflow"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if runErr := cmd.Run(); runErr != nil {
			t.Fatalf("git %v: %v", args, runErr)
		}
	}

	names, err = committedWorkflowNames(ctx, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 1 || names[0] != "ci" {
		t.Errorf("got %v, want [ci]", names)
	}
}

func TestGitHasUnpushedCommits(t *testing.T) {
	ctx := context.Background()

	// Repo with no remote: HEAD is not on any remote branch.
	dir := newTestRepo(t, nil)

	has, err := gitHasUnpushedCommits(ctx, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("expected true for repo with no remotes")
	}
}

func TestBuildMissingSecretMessage(t *testing.T) {
	tests := []struct {
		name        string
		res         *DeployResult
		wantContain []string
		wantAbsent  []string
	}{
		{
			name: "with key URL",
			res: &DeployResult{
				Repo:          "acme/widgets",
				MissingSecret: "ANTHROPIC_API_KEY",
				SecretKeyURL:  "https://console.anthropic.com/settings/keys",
			},
			wantContain: []string{
				`"ANTHROPIC_API_KEY"`,
				"acme/widgets",
				"gh secret set ANTHROPIC_API_KEY --repo acme/widgets",
				"obtain the key at https://console.anthropic.com/settings/keys",
			},
		},
		{
			name: "without key URL omits the obtain-the-key clause",
			res: &DeployResult{
				Repo:          "acme/widgets",
				MissingSecret: "COPILOT_GITHUB_TOKEN",
				SecretKeyURL:  "",
			},
			wantContain: []string{
				`"COPILOT_GITHUB_TOKEN"`,
				"gh secret set COPILOT_GITHUB_TOKEN --repo acme/widgets",
			},
			wantAbsent: []string{"obtain the key at"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildMissingSecretMessage(tt.res)
			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("message missing %q\nfull message: %s", want, got)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("message unexpectedly contains %q\nfull message: %s", absent, got)
				}
			}
		})
	}
}

func TestPRBody_MissingSecret(t *testing.T) {
	const heading = "## ⚠ Setup required"
	const addedHeading = "## Added"

	tests := []struct {
		name        string
		res         *DeployResult
		wantContain []string
		wantAbsent  []string
		// orderedBefore asserts each pair {a, b} appears with a's index < b's index.
		orderedBefore [][2]string
	}{
		{
			name: "no missing secret omits the section entirely",
			res: &DeployResult{
				Repo: "acme/widgets",
				Added: []WorkflowOutcome{
					{Name: "ci-doctor", Spec: "githubnext/agentics/ci-doctor@main"},
				},
			},
			wantAbsent: []string{
				heading,
				"gh secret set",
				"Setup required",
			},
		},
		{
			name: "missing secret with URL surfaces section above ## Added",
			res: &DeployResult{
				Repo:          "acme/widgets",
				MissingSecret: "ANTHROPIC_API_KEY",
				SecretKeyURL:  "https://console.anthropic.com/settings/keys",
				Added: []WorkflowOutcome{
					{Name: "ci-doctor", Spec: "githubnext/agentics/ci-doctor@main"},
				},
			},
			wantContain: []string{
				heading,
				"`ANTHROPIC_API_KEY`",
				"`acme/widgets`",
				"```sh\ngh secret set ANTHROPIC_API_KEY --repo acme/widgets\n```",
				"Obtain the key at: https://console.anthropic.com/settings/keys",
			},
			orderedBefore: [][2]string{
				{heading, addedHeading},
			},
		},
		{
			name: "missing secret without URL omits the obtain-the-key line",
			res: &DeployResult{
				Repo:          "acme/widgets",
				MissingSecret: "OPENAI_API_KEY",
				SecretKeyURL:  "",
				Added: []WorkflowOutcome{
					{Name: "ci-doctor", Spec: "githubnext/agentics/ci-doctor@main"},
				},
			},
			wantContain: []string{
				heading,
				"```sh\ngh secret set OPENAI_API_KEY --repo acme/widgets\n```",
			},
			wantAbsent: []string{
				"Obtain the key at:",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prBody(tt.res, tt.res.Repo, len(tt.res.Added))
			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("body missing %q\nfull body:\n%s", want, got)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("body unexpectedly contains %q\nfull body:\n%s", absent, got)
				}
			}
			for _, pair := range tt.orderedBefore {
				ai := strings.Index(got, pair[0])
				bi := strings.Index(got, pair[1])
				if ai < 0 || bi < 0 {
					t.Errorf("ordering check missing substrings: %q@%d, %q@%d", pair[0], ai, pair[1], bi)
					continue
				}
				if ai >= bi {
					t.Errorf("expected %q to appear before %q (got indices %d, %d)", pair[0], pair[1], ai, bi)
				}
			}
		})
	}
}

// ----- US3: Security Findings PR-body section (FR-005, FR-010) -----

func TestSecurityFindingsSection_NoFindings(t *testing.T) {
	if got := security.RenderPRSection(nil); got != "" {
		t.Errorf("nil findings: got %q; want empty", got)
	}
	if got := security.RenderPRSection([]security.Finding{}); got != "" {
		t.Errorf("empty findings slice: got %q; want empty", got)
	}
}

func TestSecurityFindingsSection_SingleHigh(t *testing.T) {
	res := &DeployResult{
		SecurityFindings: []security.Finding{
			{
				RuleID:   "gitleaks:aws-access-key",
				Severity: security.SeverityHigh,
				File:     ".github/workflows/foo.md",
				Line:     23,
				Message:  "AWS Access Key (<redacted>)",
				Remedy:   "Rotate the credential.",
			},
		},
	}
	got := security.RenderPRSection(res.SecurityFindings)
	if !strings.Contains(got, "## Security Findings") {
		t.Errorf("missing heading; got %q", got)
	}
	if !strings.Contains(got, "**Summary**: 1 HIGH") {
		t.Errorf("missing or wrong summary; got %q", got)
	}
	if !strings.Contains(got, "`gitleaks:aws-access-key`") {
		t.Errorf("missing rule_id; got %q", got)
	}
	if !strings.Contains(got, ".github/workflows/foo.md:23") {
		t.Errorf("missing file:line; got %q", got)
	}
	if !strings.Contains(got, "AWS Access Key (<redacted>)") {
		t.Errorf("missing message; got %q", got)
	}
	if strings.HasSuffix(got, "## Security Findings\n\n") {
		t.Errorf("body appears empty after heading; got %q", got)
	}
}

func TestSecurityFindingsSection_MixedSeverities(t *testing.T) {
	res := &DeployResult{
		SecurityFindings: []security.Finding{
			{
				RuleID:   "gitleaks:aws-access-key",
				Severity: security.SeverityHigh,
				File:     ".github/workflows/a.md",
				Line:     1,
				Message:  "h1",
				Remedy:   "r",
			},
			{
				RuleID:   "fleet.permissions.write-on-schedule",
				Severity: security.SeverityHigh,
				File:     ".github/workflows/b.md",
				Line:     5,
				Message:  "h2",
				Remedy:   "r",
			},
			{
				RuleID:   "fleet.safe-outputs.draft-false",
				Severity: security.SeverityMedium,
				File:     ".github/workflows/c.md",
				Line:     12,
				Message:  "m",
				Remedy:   "r",
			},
			{
				RuleID:   "actionlint:not-installed",
				Severity: security.SeverityInfo,
				File:     "",
				Line:     0,
				Message:  "i",
				Remedy:   "r",
			},
		},
	}
	got := security.RenderPRSection(res.SecurityFindings)
	if !strings.Contains(got, "**Summary**: 2 HIGH, 1 MEDIUM, 1 INFO") {
		t.Errorf("wrong tally line; got %q", got)
	}
	// LOW is not in input → must not appear in summary line.
	if strings.Contains(got, "LOW") {
		t.Errorf("LOW should not appear when no LOW findings; got %q", got)
	}
}

func TestSecurityFindingsSection_StableSort(t *testing.T) {
	// Pre-sorted slice — RenderPRSection passes through to RenderForPRBody
	// which renders in the order received. The Run-level sort is what guarantees
	// the order; this test verifies the bullet rendering preserves the slice
	// order byte-identically.
	a := security.Finding{
		RuleID: "z", Severity: security.SeverityHigh,
		File: "a.md", Line: 1, Message: "m", Remedy: "r",
	}
	b := security.Finding{
		RuleID: "y", Severity: security.SeverityHigh,
		File: "a.md", Line: 5, Message: "m", Remedy: "r",
	}
	c := security.Finding{
		RuleID: "x", Severity: security.SeverityMedium,
		File: "a.md", Line: 1, Message: "m", Remedy: "r",
	}
	res := &DeployResult{SecurityFindings: []security.Finding{a, b, c}}
	got := security.RenderPRSection(res.SecurityFindings)
	ai := strings.Index(got, "`z`")
	bi := strings.Index(got, "`y`")
	ci := strings.Index(got, "`x`")
	if !(ai < bi && bi < ci) {
		t.Errorf("bullet order: z@%d, y@%d, x@%d (want strictly ascending)", ai, bi, ci)
	}
}

func TestPRBodyAppendsSecurityFindings(t *testing.T) {
	res := &DeployResult{
		Repo:          "x/y",
		MissingSecret: "ANTHROPIC_API_KEY",
		SecretKeyURL:  "https://example",
		Added:         []WorkflowOutcome{{Name: "wf", Spec: "spec@v1"}},
		SecurityFindings: []security.Finding{
			{
				RuleID:   "fleet.permissions.write-on-schedule",
				Severity: security.SeverityHigh,
				File:     ".github/workflows/x.md",
				Line:     5,
				Message:  "m",
				Remedy:   "r",
			},
		},
	}
	got := prBody(res, res.Repo, len(res.Added))
	setup := strings.Index(got, "## ⚠ Setup required")
	sec := strings.Index(got, "## Security Findings")
	if setup < 0 {
		t.Fatalf("missing ## ⚠ Setup required heading; body:\n%s", got)
	}
	if sec < 0 {
		t.Fatalf("missing ## Security Findings heading; body:\n%s", got)
	}
	if !(setup < sec) {
		t.Errorf("setup-required should appear before security-findings (got indices %d, %d)", setup, sec)
	}
}
