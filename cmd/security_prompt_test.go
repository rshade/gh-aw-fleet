package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
	"github.com/rshade/gh-aw-fleet/internal/fleet/security"
	logpkg "github.com/rshade/gh-aw-fleet/internal/log"
	"github.com/rshade/gh-aw-fleet/internal/testutil"
)

// --- T019: --yes flag registration, help text, and propagation ---

func TestYesFlagAppearsInHelp(t *testing.T) {
	dir := writeStrictCommandConfig(t)
	for _, tc := range []struct {
		name string
		cmd  *cobra.Command
	}{
		{name: "deploy", cmd: newDeployCmd(&dir)},
		{name: "sync", cmd: newSyncCmd(&dir)},
		{name: "upgrade", cmd: newUpgradeCmd(&dir)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flag := tc.cmd.Flags().Lookup("yes")
			if flag == nil {
				t.Fatal("--yes flag missing")
			}
			for _, want := range []string{"Skip the interactive", "stderr", "PR body"} {
				if !strings.Contains(flag.Usage, want) {
					t.Fatalf("--yes usage = %q; want substring %q", flag.Usage, want)
				}
			}
		})
	}
}

func TestDeployYesFlagPropagates(t *testing.T) {
	dir := writeStrictCommandConfig(t)
	orig := runFleetDeploy
	t.Cleanup(func() { runFleetDeploy = orig })

	var got []fleet.DeployOpts
	runFleetDeploy = func(_ context.Context, _ *fleet.Config, repo string, opts fleet.DeployOpts) (*fleet.DeployResult, error) {
		got = append(got, opts)
		return &fleet.DeployResult{Repo: repo, SecurityFindings: []security.Finding{}}, nil
	}

	if err := executeStandaloneCommand(newDeployCmd(&dir), "acme/widgets"); err != nil {
		t.Fatalf("deploy error = %v", err)
	}
	if err := executeStandaloneCommand(newDeployCmd(&dir), "acme/widgets", "--yes"); err != nil {
		t.Fatalf("deploy --yes error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("deploy calls = %d; want 2", len(got))
	}
	if got[0].Security.Yes {
		t.Error("default deploy propagated Yes=true")
	}
	if !got[1].Security.Yes {
		t.Error("deploy --yes propagated Yes=false")
	}
}

func TestSyncYesFlagPropagates(t *testing.T) {
	dir := writeStrictCommandConfig(t)
	orig := runFleetSync
	t.Cleanup(func() { runFleetSync = orig })

	var got fleet.SyncOpts
	runFleetSync = func(_ context.Context, _ *fleet.Config, repo string, opts fleet.SyncOpts) (*fleet.SyncResult, error) {
		got = opts
		return &fleet.SyncResult{Repo: repo, SecurityFindings: []security.Finding{}}, nil
	}

	if err := executeStandaloneCommand(newSyncCmd(&dir), "acme/widgets", "--yes"); err != nil {
		t.Fatalf("sync --yes error = %v", err)
	}
	if !got.Security.Yes {
		t.Error("sync --yes propagated Yes=false")
	}
}

func TestUpgradeYesFlagPropagates(t *testing.T) {
	dir := writeStrictCommandConfig(t)
	orig := runFleetUpgrade
	t.Cleanup(func() { runFleetUpgrade = orig })

	var got fleet.UpgradeOpts
	runFleetUpgrade = func(_ context.Context, _ *fleet.Config, repo string, opts fleet.UpgradeOpts) (*fleet.UpgradeResult, error) {
		got = opts
		return &fleet.UpgradeResult{Repo: repo}, nil
	}

	if err := executeStandaloneCommand(newUpgradeCmd(&dir), "acme/widgets", "--yes"); err != nil {
		t.Fatalf("upgrade --yes error = %v", err)
	}
	if !got.Security.Yes {
		t.Error("upgrade --yes propagated Yes=false")
	}
}

// --- T026 / T027: --output json forces Yes (FR-018), envelope stays valid ---

func TestJSONModeForcesYesOnDeploy(t *testing.T) {
	dir := writeStrictCommandConfig(t)
	orig := runFleetDeploy
	t.Cleanup(func() { runFleetDeploy = orig })

	var got fleet.DeployOpts
	var stdout bytes.Buffer
	runFleetDeploy = func(_ context.Context, _ *fleet.Config, repo string, opts fleet.DeployOpts) (*fleet.DeployResult, error) {
		got = opts
		return &fleet.DeployResult{
			Repo:             repo,
			SecurityFindings: []security.Finding{strictCommandFinding()},
		}, nil
	}

	root := NewRootCmd()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--dir", dir, "-o", "json", "deploy", "acme/widgets", "--apply"})
	if err := root.Execute(); err != nil {
		t.Fatalf("deploy -o json error = %v", err)
	}
	if !got.Security.Yes {
		t.Error("json mode did not force Security.Yes=true (FR-018)")
	}
	// The JSON envelope must be valid and uncorrupted by any prompt text.
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("envelope not valid JSON: %v\nraw=%s", err, stdout.String())
	}
	if strings.Contains(stdout.String(), "Proceed with commit") {
		t.Error("prompt text leaked into the JSON envelope")
	}
}

func TestTextModeDoesNotForceYes(t *testing.T) {
	dir := writeStrictCommandConfig(t)
	orig := runFleetDeploy
	t.Cleanup(func() { runFleetDeploy = orig })

	var got fleet.DeployOpts
	runFleetDeploy = func(_ context.Context, _ *fleet.Config, repo string, opts fleet.DeployOpts) (*fleet.DeployResult, error) {
		got = opts
		return &fleet.DeployResult{Repo: repo, SecurityFindings: []security.Finding{}}, nil
	}

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--dir", dir, "deploy", "acme/widgets", "--apply"})
	if err := root.Execute(); err != nil {
		t.Fatalf("deploy error = %v", err)
	}
	if got.Security.Yes {
		t.Error("text mode without --yes should leave Security.Yes=false")
	}
}

// --- T009: operator-decline mapping ---

func TestMapOperatorDeclineCleanExit(t *testing.T) {
	declineErr := &fleet.OperatorDeclinedError{Repo: "x/y", Findings: 2}
	var stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetErr(&stderr)
	cmd.SetOut(&bytes.Buffer{})

	mapped := mapOperatorDecline(cmd, declineErr)
	if mapped == nil {
		t.Fatal("mapped error is nil; want a non-zero exit error")
	}
	code, ok := ExitCodeForError(mapped)
	if !ok || code != 1 {
		t.Errorf("ExitCodeForError = (%d, %v); want (1, true)", code, ok)
	}
	if !SuppressErrorLog(mapped) {
		t.Error("decline should suppress the fatal-crash log")
	}
	if !fleet.IsOperatorDeclinedError(mapped) {
		t.Error("mapped error should still unwrap to *OperatorDeclinedError")
	}
	if !strings.Contains(stderr.String(), "aborted by operator") {
		t.Errorf("stderr = %q; want the abort message", stderr.String())
	}
}

func TestMapOperatorDeclinePassthrough(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetOut(&bytes.Buffer{})

	if got := mapOperatorDecline(cmd, nil); got != nil {
		t.Errorf("mapOperatorDecline(nil) = %v; want nil", got)
	}
	other := errors.New("boom")
	if got := mapOperatorDecline(cmd, other); !errors.Is(got, other) {
		t.Errorf("mapOperatorDecline(other) = %v; want passthrough of %v", got, other)
	}
}

// --- T021: --yes still emits the stderr findings surface (cmd layer) ---

func TestSecurityFindingWarningsIndependentOfPrompt(t *testing.T) {
	// emitSecurityFindingWarnings is the cmd-layer stderr surface; it never
	// consults --yes, so findings always print regardless of the prompt choice.
	finding := strictCommandFinding()
	buf := testutil.CaptureStderr(t, func() {
		if err := logpkg.Configure("info", "json"); err != nil {
			t.Fatal(err)
		}
		emitSecurityFindingWarnings([]security.Finding{finding})
	})
	if !strings.Contains(buf, finding.RuleID) {
		t.Errorf("stderr = %q; want finding rule_id %q", buf, finding.RuleID)
	}
}
