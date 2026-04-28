package fleet

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
