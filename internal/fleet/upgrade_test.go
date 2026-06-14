package fleet

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParseGitStatusPorcelain(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "leading space on first line preserved",
			in:   " M .github/workflows/ci-doctor.lock.yml\n M .github/workflows/pr-fix.lock.yml\n",
			want: []string{
				".github/workflows/ci-doctor.lock.yml",
				".github/workflows/pr-fix.lock.yml",
			},
		},
		{
			name: "no trailing newline",
			in:   " M .github/a\n M .github/b",
			want: []string{".github/a", ".github/b"},
		},
		{
			name: "empty output",
			in:   "",
			want: nil,
		},
		{
			name: "only newline",
			in:   "\n",
			want: nil,
		},
		{
			name: "root-level file",
			in:   " M README.md\n",
			want: []string{"README.md"},
		},
		{
			name: "added file (A in index column)",
			in:   "A  .github/workflows/new.yml\n",
			want: []string{".github/workflows/new.yml"},
		},
		{
			name: "modified in both index and worktree",
			in:   "MM .github/workflows/both.yml\n",
			want: []string{".github/workflows/both.yml"},
		},
		{
			name: "short line skipped",
			in:   " M\n M .github/ok.yml\n",
			want: []string{".github/ok.yml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGitStatusPorcelain(tt.in)
			if !slices.Equal(got, tt.want) {
				t.Errorf("parseGitStatusPorcelain(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestUpgrade_AutoPublicPath_InvokesStrictCompile(t *testing.T) {
	res := &UpgradeResult{Repo: "rshade/test", CloneDir: t.TempDir()}
	seams := &compileStrictSeams{
		visibility: "public",
		helpOut:    "  --strict  enable strict validation\n",
		compileOut: "ok",
	}
	installCompileStrictSeams(t, seams)
	buf := captureZlog(t)

	cfg := &Config{Repos: map[string]RepoSpec{"rshade/test": {}}}
	if err := runCompileStrictIfNeeded(context.Background(), res, cfg, "rshade/test"); err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if !res.CompileStrictApplied {
		t.Errorf("CompileStrictApplied = false; want true")
	}
	if res.CompileStrictSource != "auto-public" {
		t.Errorf("CompileStrictSource = %q; want auto-public", res.CompileStrictSource)
	}
	if seams.visibilityCalls != 1 || seams.helpCalls != 1 || seams.compileCalls != 1 {
		t.Errorf("seam calls: visibility=%d help=%d compile=%d; want 1/1/1",
			seams.visibilityCalls, seams.helpCalls, seams.compileCalls)
	}
	evt := findZlogEvent(t, buf, "compile_strict_resolved")
	if evt == nil {
		t.Fatalf("no compile_strict_resolved event; log=%s", buf.String())
	}
	if evt["source"] != "auto-public" || evt["effective"] != true {
		t.Errorf("event fields = %+v; want source=auto-public effective=true", evt)
	}
}

func TestUpgrade_VisibilityLookupFails_FailSecureStrictOn(t *testing.T) {
	res := &UpgradeResult{Repo: "rshade/test", CloneDir: t.TempDir()}
	seams := &compileStrictSeams{
		visibilityErr: errors.New("HTTP 403 Forbidden"),
		helpOut:       "  --strict\n",
		compileOut:    "ok",
	}
	installCompileStrictSeams(t, seams)
	buf := captureZlog(t)

	cfg := &Config{Repos: map[string]RepoSpec{"rshade/test": {}}}
	if err := runCompileStrictIfNeeded(context.Background(), res, cfg, "rshade/test"); err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if !res.CompileStrictApplied {
		t.Errorf("CompileStrictApplied = false; want true")
	}
	if res.CompileStrictSource != "auto-fallback" {
		t.Errorf("CompileStrictSource = %q; want auto-fallback", res.CompileStrictSource)
	}
	warn := findZlogEvent(t, buf, "compile_strict_lookup_failed")
	if warn == nil {
		t.Fatalf("no compile_strict_lookup_failed event; log=%s", buf.String())
	}
}

func TestUpgrade_CompileFails_EmitsDiagCompileStrictFailed(t *testing.T) {
	cloneDir := t.TempDir()
	res := &UpgradeResult{Repo: "rshade/test", CloneDir: cloneDir}
	const rawStderr = "✗ strict mode validation failed for workflow foo.md"
	seams := &compileStrictSeams{
		visibility: "public",
		helpOut:    "  --strict\n",
		compileOut: rawStderr,
		compileErr: errors.New("exit 1"),
	}
	installCompileStrictSeams(t, seams)
	_ = captureZlog(t)

	cfg := &Config{Repos: map[string]RepoSpec{"rshade/test": {}}}
	err := runCompileStrictIfNeeded(context.Background(), res, cfg, "rshade/test")
	if err == nil {
		t.Fatal("err = nil; want non-nil")
	}
	if !strings.Contains(err.Error(), rawStderr) {
		t.Errorf("err = %q; want raw stderr %q preserved (FR-009)", err.Error(), rawStderr)
	}
	if _, statErr := os.Stat(cloneDir); statErr != nil {
		t.Errorf("clone dir %q removed by helper; FR-009 requires preservation: %v", cloneDir, statErr)
	}
}

func TestUpgrade_ProbeFlagAbsent_EmitsDiagGhAwTooOld(t *testing.T) {
	res := &UpgradeResult{Repo: "rshade/test", CloneDir: t.TempDir()}
	seams := &compileStrictSeams{
		visibility: "public",
		helpOut:    "  --some-other-flag\n",
		versionOut: "v0.50.0",
	}
	installCompileStrictSeams(t, seams)
	_ = captureZlog(t)

	cfg := &Config{Repos: map[string]RepoSpec{"rshade/test": {}}}
	err := runCompileStrictIfNeeded(context.Background(), res, cfg, "rshade/test")
	if err == nil {
		t.Fatal("err = nil; want non-nil")
	}
	for _, want := range []string{"v0.79.2", "v0.50.0"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q; want substring %q", err.Error(), want)
		}
	}
	if seams.compileCalls != 0 {
		t.Errorf("compile seam invoked despite flag-absent probe; calls=%d", seams.compileCalls)
	}
}

func TestUpgrade_ProbeFailed_EmitsDiagGhAwMissing(t *testing.T) {
	res := &UpgradeResult{Repo: "rshade/test", CloneDir: t.TempDir()}
	seams := &compileStrictSeams{
		visibility: "public",
		helpErr:    errors.New("exec: \"gh\": executable file not found in $PATH"),
	}
	installCompileStrictSeams(t, seams)
	_ = captureZlog(t)

	cfg := &Config{Repos: map[string]RepoSpec{"rshade/test": {}}}
	err := runCompileStrictIfNeeded(context.Background(), res, cfg, "rshade/test")
	if err == nil {
		t.Fatal("err = nil; want non-nil")
	}
	if !strings.Contains(err.Error(), "gh extension install") {
		t.Errorf("err = %q; want substring gh extension install", err.Error())
	}
	if seams.compileCalls != 0 {
		t.Errorf("compile seam invoked despite probe-failed; calls=%d", seams.compileCalls)
	}
}

// installFakeGhForUpgrade puts a fake `gh` on PATH handling the upgrade
// pipeline's gh calls (repo clone, aw init, aw upgrade, aw update). Compile-strict
// is supplied separately via installCompileStrictSeams; `gh pr create` is left
// unhandled so the run fails after init+manifest, mirroring installFakeGhForSync.
func installFakeGhForUpgrade(t *testing.T, remote string) {
	t.Helper()
	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "fake-gh.log")
	script := `#!/bin/sh
set -eu

if [ "$1" = "repo" ] && [ "$2" = "clone" ]; then
	git clone "$FLEET_TEST_REMOTE" "$4"
	git -C "$4" config commit.gpgsign false
	git -C "$4" config user.email test@example.com
	git -C "$4" config user.name Test
	exit 0
fi

if [ "$1" = "aw" ] && [ "$2" = "init" ]; then
	mkdir -p .github/agents
	printf '%s\n' 'agent setup' > .github/agents/agentic-workflows.md
	printf 'init\n' >> "${FAKE_GH_LOG:?}"
	exit 0
fi

if [ "$1" = "aw" ] && { [ "$2" = "upgrade" ] || [ "$2" = "update" ]; }; then
	printf '%s\n' "$2" >> "${FAKE_GH_LOG:?}"
	exit 0
fi

echo "unexpected gh args: $*" >&2
exit 1
`
	if err := os.WriteFile(filepath.Join(binDir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("FLEET_TEST_REMOTE", remote)
	t.Setenv("FAKE_GH_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func seedCurrentInitMarker(dir string) {
	marker := filepath.Join(dir, ".github", "agents", "agentic-workflows.md")
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(marker, []byte("agent setup\n"), 0o644); err != nil {
		panic(err)
	}
	gitInDir(dir, "add", ".github/agents/agentic-workflows.md")
	gitInDir(dir, "commit", "-m", "seed current init marker")
}

// TestUpgrade_RunsInitAndWritesManifest verifies the upgrade gap fix: Upgrade
// refreshes init artifacts (ensureInit, setting InitWasRun) and records the
// fleet manifest into the clone before the PR step. The run fails later at
// `gh pr create` (no real remote), but init + manifest happen first — mirrors
// TestSync_PruneOnlyPath_WritesManifest.
func TestUpgrade_RunsInitAndWritesManifest(t *testing.T) {
	repo := "rshade/upgrade-manifest"
	remote := newTestRepo(t, nil)
	installFakeGhForUpgrade(t, remote)
	installCompileStrictSeams(t, &compileStrictSeams{
		visibility: "public",
		helpOut:    "  --strict\n",
		compileOut: "ok",
	})

	cfg := &Config{
		Profiles: map[string]Profile{
			"default": {
				Sources:   map[string]SourcePin{"github/gh-aw": {Ref: "v0.79.2"}},
				Workflows: []ProfileWorkflow{{Name: "ci-doctor", Source: "github/gh-aw"}},
			},
		},
		Repos: map[string]RepoSpec{
			repo: {Profiles: []string{"default"}},
		},
	}

	res, _ := Upgrade(context.Background(), cfg, repo, UpgradeOpts{Apply: true})
	if res == nil {
		t.Fatal("Upgrade returned nil result")
	}
	if res.CloneDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	}
	if !res.InitWasRun {
		t.Error("InitWasRun = false; want true (fresh clone has no fleet manifest)")
	}
	manifestPath := filepath.Join(res.CloneDir, FleetManifestPath)
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("manifest not written to %s before PR step: %v", manifestPath, err)
	}
}

func TestUpgradeApplyBackfillsManifestWhenNoUpgradeDiffs(t *testing.T) {
	repo := "rshade/upgrade-manifest-clean"
	remote := newTestRepo(t, seedCurrentInitMarker)
	installFakeGhForUpgrade(t, remote)
	seams := &compileStrictSeams{
		visibility: "public",
		helpOut:    "  --strict\n",
		compileOut: "ok",
	}
	installCompileStrictSeams(t, seams)

	cfg := &Config{
		Profiles: map[string]Profile{
			"default": {
				Sources:   map[string]SourcePin{"github/gh-aw": {Ref: "v0.79.2"}},
				Workflows: []ProfileWorkflow{{Name: "ci-doctor", Source: "github/gh-aw"}},
			},
		},
		Repos: map[string]RepoSpec{
			repo: {Profiles: []string{"default"}},
		},
	}

	res, _ := Upgrade(context.Background(), cfg, repo, UpgradeOpts{Apply: true})
	if res == nil {
		t.Fatal("Upgrade returned nil result")
	}
	if res.CloneDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	}
	if res.NoChanges {
		t.Fatal("NoChanges = true; want manifest-only change to continue to PR path")
	}
	if !slices.Equal(res.ChangedFiles, []string{FleetManifestPath}) {
		t.Fatalf("ChangedFiles = %q, want [%s]", res.ChangedFiles, FleetManifestPath)
	}
	if seams.compileCalls != 0 {
		t.Fatalf("compile seam calls = %d, want 0 for manifest-only backfill", seams.compileCalls)
	}
	manifestPath := filepath.Join(res.CloneDir, FleetManifestPath)
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("manifest not written to %s: %v", manifestPath, err)
	}
}
