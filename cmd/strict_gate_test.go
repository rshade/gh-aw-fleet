package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
	"github.com/rshade/gh-aw-fleet/internal/fleet/security"
	logpkg "github.com/rshade/gh-aw-fleet/internal/log"
	"github.com/rshade/gh-aw-fleet/internal/testutil"
)

func TestDeployStrictFlagPropagatesAndCleanNoOps(t *testing.T) {
	dir := writeStrictCommandConfig(t)
	orig := runFleetDeploy
	t.Cleanup(func() { runFleetDeploy = orig })

	var got []fleet.DeployOpts
	runFleetDeploy = func(_ context.Context, _ *fleet.Config, repo string, opts fleet.DeployOpts) (*fleet.DeployResult, error) {
		got = append(got, opts)
		return &fleet.DeployResult{Repo: repo, SecurityFindings: []security.Finding{}}, nil
	}

	if err := executeStandaloneCommand(newDeployCmd(&dir), "acme/widgets"); err != nil {
		t.Fatalf("non-strict deploy error = %v", err)
	}
	if err := executeStandaloneCommand(newDeployCmd(&dir), "acme/widgets", "--strict"); err != nil {
		t.Fatalf("strict deploy error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("deploy calls = %d; want 2", len(got))
	}
	if got[0].Security.Strict {
		t.Fatal("non-strict deploy propagated Strict=true")
	}
	if !got[1].Security.Strict {
		t.Fatal("strict deploy propagated Strict=false")
	}
}

func TestSyncStrictFlagPropagates(t *testing.T) {
	dir := writeStrictCommandConfig(t)
	orig := runFleetSync
	t.Cleanup(func() { runFleetSync = orig })

	var got fleet.SyncOpts
	runFleetSync = func(_ context.Context, _ *fleet.Config, repo string, opts fleet.SyncOpts) (*fleet.SyncResult, error) {
		got = opts
		return &fleet.SyncResult{Repo: repo, SecurityFindings: []security.Finding{}}, nil
	}

	if err := executeStandaloneCommand(newSyncCmd(&dir), "acme/widgets", "--strict"); err != nil {
		t.Fatalf("sync error = %v", err)
	}
	if !got.Security.Strict {
		t.Fatal("sync propagated Strict=false")
	}
}

func TestUpgradeStrictFlagPropagatesAndAuditAccepted(t *testing.T) {
	dir := writeStrictCommandConfig(t)
	orig := runFleetUpgrade
	t.Cleanup(func() { runFleetUpgrade = orig })

	var got fleet.UpgradeOpts
	runFleetUpgrade = func(_ context.Context, _ *fleet.Config, repo string, opts fleet.UpgradeOpts) (*fleet.UpgradeResult, error) {
		got = opts
		return &fleet.UpgradeResult{Repo: repo, AuditJSON: json.RawMessage(`{"issues":0}`)}, nil
	}

	if err := executeStandaloneCommand(newUpgradeCmd(&dir), "acme/widgets", "--audit", "--strict"); err != nil {
		t.Fatalf("upgrade --audit --strict error = %v", err)
	}
	if !got.Audit {
		t.Fatal("upgrade propagated Audit=false")
	}
	if !got.Security.Strict {
		t.Fatal("upgrade propagated Strict=false")
	}
}

func TestStrictFlagsAppearInHelp(t *testing.T) {
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
			flag := tc.cmd.Flags().Lookup("strict")
			if flag == nil {
				t.Fatal("--strict flag missing")
			}
			for _, want := range []string{"HIGH Layer 1 security findings", "does not change gh aw compile --strict"} {
				if !strings.Contains(flag.Usage, want) {
					t.Fatalf("--strict usage = %q; want substring %q", flag.Usage, want)
				}
			}
		})
	}
}

func TestDeployAndSyncStrictEnvelopeIncludesFindingWarnings(t *testing.T) {
	finding := strictCommandFinding()
	strictErr := strictCommandError("x/y", finding)

	t.Run("deploy", func(t *testing.T) {
		cmd, stdout := envelopeCommandWithBuffer()
		stderr := testutil.CaptureStderr(t, func() {
			if err := logpkg.Configure("info", "json"); err != nil {
				t.Fatal(err)
			}
			err := emitDeployEnvelope(
				cmd,
				"x/y",
				false,
				&fleet.DeployResult{Repo: "x/y", SecurityFindings: []security.Finding{finding}},
				strictErr,
			)
			if !errors.Is(err, strictErr) {
				t.Fatalf("emitDeployEnvelope error = %v; want %v", err, strictErr)
			}
		})
		assertStrictEnvelopeWarning(t, stdout.Bytes(), stderr, finding.RuleID)
	})

	t.Run("sync", func(t *testing.T) {
		cmd, stdout := envelopeCommandWithBuffer()
		stderr := testutil.CaptureStderr(t, func() {
			if err := logpkg.Configure("info", "json"); err != nil {
				t.Fatal(err)
			}
			err := emitSyncEnvelope(
				cmd,
				"x/y",
				false,
				&fleet.SyncResult{Repo: "x/y", SecurityFindings: []security.Finding{finding}},
				strictErr,
			)
			if !errors.Is(err, strictErr) {
				t.Fatalf("emitSyncEnvelope error = %v; want %v", err, strictErr)
			}
		})
		assertStrictEnvelopeWarning(t, stdout.Bytes(), stderr, finding.RuleID)
	})
}

func TestUpgradeStrictEnvelopeIncludesFindingWarnings(t *testing.T) {
	finding := strictCommandFinding()
	strictErr := strictCommandError("x/y", finding)
	cmd, stdout := envelopeCommandWithBuffer()

	stderr := testutil.CaptureStderr(t, func() {
		if err := logpkg.Configure("info", "json"); err != nil {
			t.Fatal(err)
		}
		err := emitUpgradeEnvelope(
			cmd,
			"x/y",
			false,
			&fleet.UpgradeResult{Repo: "x/y", SecurityFindings: []security.Finding{finding}},
			strictErr,
		)
		if !errors.Is(err, strictErr) {
			t.Fatalf("emitUpgradeEnvelope error = %v; want %v", err, strictErr)
		}
	})
	assertStrictEnvelopeWarning(t, stdout.Bytes(), stderr, finding.RuleID)
}

func TestUpgradeAllJSONStrictStopsAfterBlockedRepo(t *testing.T) {
	orig := runFleetUpgrade
	t.Cleanup(func() { runFleetUpgrade = orig })

	finding := strictCommandFinding()
	strictErr := strictCommandError("a/blocked", finding)
	var calls []string
	runFleetUpgrade = func(_ context.Context, _ *fleet.Config, repo string, _ fleet.UpgradeOpts) (*fleet.UpgradeResult, error) {
		calls = append(calls, repo)
		if repo != "a/blocked" {
			t.Fatalf("runFleetUpgrade called for %s after strict blocker", repo)
		}
		return &fleet.UpgradeResult{Repo: repo, SecurityFindings: []security.Finding{finding}}, strictErr
	}

	cfg := &fleet.Config{
		Repos: map[string]fleet.RepoSpec{
			"z/later":   {},
			"a/blocked": {},
		},
	}
	cmd, stdout := envelopeCommandWithBuffer()
	err := runUpgradeAllJSON(cmd, cfg, fleet.UpgradeOpts{Security: fleet.SecurityOpts{Strict: true}}, false)
	if !errors.Is(err, strictErr) {
		t.Fatalf("runUpgradeAllJSON error = %v; want %v", err, strictErr)
	}
	if len(calls) != 1 || calls[0] != "a/blocked" {
		t.Fatalf("calls = %v; want only a/blocked", calls)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("NDJSON lines = %d; want 1; output=%s", len(lines), stdout.String())
	}
	var env Envelope
	if decodeErr := json.Unmarshal([]byte(lines[0]), &env); decodeErr != nil {
		t.Fatalf("decode envelope: %v", decodeErr)
	}
	if env.Repo != "a/blocked" {
		t.Fatalf("envelope repo = %q; want a/blocked", env.Repo)
	}
	if len(env.Warnings) != 1 || env.Warnings[0].Fields["rule_id"] != finding.RuleID {
		t.Fatalf("warnings = %#v; want finding diagnostic", env.Warnings)
	}
}

func writeStrictCommandConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	body := `{"version":1,"profiles":{"default":{"sources":{},"workflows":[]}},"repos":{"acme/widgets":{"profiles":["default"]}}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "fleet.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write fleet.json: %v", err)
	}
	return dir
}

func executeStandaloneCommand(cmd *cobra.Command, args ...string) error {
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	return cmd.Execute()
}

func envelopeCommandWithBuffer() (*cobra.Command, *bytes.Buffer) {
	var stdout bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	return cmd, &stdout
}

func strictCommandFinding() security.Finding {
	return security.Finding{
		RuleID:   "fleet.permissions.write-on-schedule",
		Severity: security.SeverityHigh,
		File:     ".github/workflows/test.md",
		Line:     7,
		Message:  "synthetic security finding",
		Remedy:   "Fix the synthetic finding.",
	}
}

func strictCommandError(repo string, finding security.Finding) *fleet.StrictSecurityError {
	return &fleet.StrictSecurityError{
		Repo:             repo,
		BlockingCount:    1,
		BlockingFindings: []security.Finding{finding},
		BreadcrumbPath:   "/tmp/gh-aw-fleet-test/findings.json",
	}
}

func assertStrictEnvelopeWarning(t *testing.T, stdout []byte, stderr, ruleID string) {
	t.Helper()
	if !strings.Contains(stderr, ruleID) {
		t.Fatalf("stderr = %q; want rule ID %q", stderr, ruleID)
	}
	var env Envelope
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v; raw=%s", err, stdout)
	}
	if len(env.Warnings) != 1 {
		t.Fatalf("len(warnings) = %d; want 1: %#v", len(env.Warnings), env.Warnings)
	}
	if env.Warnings[0].Fields["rule_id"] != ruleID {
		t.Fatalf("warning rule_id = %v; want %s", env.Warnings[0].Fields["rule_id"], ruleID)
	}
	if len(env.Hints) != 1 || !strings.Contains(env.Hints[0].Message, "strict security gate") {
		t.Fatalf("hints = %#v; want strict security gate failure hint", env.Hints)
	}
}
