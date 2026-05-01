package fleet

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncDryRunPreflightTreatsPreparedCloneAsInternal(t *testing.T) {
	repo := "rshade/sync-missing"
	remote := newTestRepo(t, nil)
	installFakeGhForSync(t, remote)

	cfg := &Config{
		Version: SchemaVersion,
		Profiles: map[string]Profile{
			"default": {
				Sources: map[string]SourcePin{
					"githubnext/agentics": {Ref: "v1.0.0"},
				},
				Workflows: []ProfileWorkflow{
					{Name: "ci-doctor", Source: "githubnext/agentics"},
				},
			},
		},
		Repos: map[string]RepoSpec{
			repo: {Profiles: []string{"default"}},
		},
	}

	res, err := Sync(context.Background(), cfg, repo, SyncOpts{})
	if err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	if len(res.Missing) != 1 || res.Missing[0] != "ci-doctor" {
		t.Fatalf("Missing = %v, want [ci-doctor]", res.Missing)
	}
	if res.DeployPreflight == nil {
		t.Fatal("DeployPreflight = nil, want preflight result")
	}
	if len(res.DeployPreflight.Added) != 1 || res.DeployPreflight.Added[0].Name != "ci-doctor" {
		t.Fatalf("DeployPreflight.Added = %#v, want ci-doctor", res.DeployPreflight.Added)
	}
}

func installFakeGhForSync(t *testing.T, remote string) {
	t.Helper()
	binDir := t.TempDir()
	ghPath := filepath.Join(binDir, "gh")
	script := `#!/bin/sh
set -eu

if [ "$1" = "repo" ] && [ "$2" = "clone" ]; then
	git clone "$FLEET_TEST_REMOTE" "$4"
	exit $?
fi

if [ "$1" = "aw" ] && [ "$2" = "init" ]; then
	mkdir -p .github/agents
	printf '%s\n' 'agent setup' > .github/agents/agentic-workflows.agent.md
	exit 0
fi

if [ "$1" = "aw" ] && [ "$2" = "add" ]; then
	spec="$3"
	name="${spec##*/}"
	name="${name%@*}"
	name="${name%.md}"
	mkdir -p .github/workflows
	printf '%s\n' '---' "source: $spec" '---' > ".github/workflows/$name.md"
	exit 0
fi

echo "unexpected gh args: $*" >&2
exit 1
`
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("FLEET_TEST_REMOTE", remote)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
