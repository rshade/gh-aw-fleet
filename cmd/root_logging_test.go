package cmd

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
	logpkg "github.com/rshade/gh-aw-fleet/internal/log"
	"github.com/rshade/gh-aw-fleet/internal/testutil"
)

func TestPersistentLogFlagsRegistered(t *testing.T) {
	root := NewRootCmd()

	fLevel := root.PersistentFlags().Lookup("log-level")
	if fLevel == nil {
		t.Fatal("--log-level not registered as persistent flag")
	}
	if fLevel.DefValue != "info" {
		t.Errorf("--log-level default = %q; want info", fLevel.DefValue)
	}

	fFormat := root.PersistentFlags().Lookup("log-format")
	if fFormat == nil {
		t.Fatal("--log-format not registered as persistent flag")
	}
	if fFormat.DefValue != "console" {
		t.Errorf("--log-format default = %q; want console", fFormat.DefValue)
	}
}

func TestInvalidLogLevelBlocksSubcommand(t *testing.T) {
	ran := false
	root := &cobra.Command{
		Use:          "test-root",
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			level, _ := cmd.Flags().GetString("log-level")
			format, _ := cmd.Flags().GetString("log-format")
			return logpkg.Configure(level, format)
		},
	}
	root.PersistentFlags().String("log-level", "info", "")
	root.PersistentFlags().String("log-format", "console", "")
	sub := &cobra.Command{
		Use: "list",
		RunE: func(_ *cobra.Command, _ []string) error {
			ran = true
			return nil
		},
	}
	root.AddCommand(sub)
	root.SetArgs([]string{"list", "--log-level=shouting"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() = nil; want error for invalid --log-level")
	}
	if !strings.Contains(err.Error(), "--log-level") {
		t.Errorf("error %q missing --log-level identifier", err.Error())
	}
	if ran {
		t.Error("subcommand RunE executed despite invalid --log-level")
	}
}

func TestDeployWarningEmitsStructuredJSON(t *testing.T) {
	buf := testutil.CaptureStderr(t, func() {
		if err := logpkg.Configure("info", "json"); err != nil {
			t.Fatal(err)
		}
		res := &fleet.DeployResult{
			Repo:          "acme/api",
			MissingSecret: "DEPLOY_TOKEN",
		}
		emitDeployWarnings(res)
	})
	line := strings.TrimSpace(buf)
	if line == "" {
		t.Fatal("no output emitted")
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("not valid JSON: %v; raw=%q", err, line)
	}
	if obj["level"] != "warn" {
		t.Errorf("level = %v; want warn", obj["level"])
	}
	if obj["repo"] != "acme/api" {
		t.Errorf("repo = %v; want acme/api", obj["repo"])
	}
	if obj["secret"] != "DEPLOY_TOKEN" {
		t.Errorf("secret = %v; want DEPLOY_TOKEN", obj["secret"])
	}
}

func TestSyncDriftEmitsStructuredJSON(t *testing.T) {
	buf := testutil.CaptureStderr(t, func() {
		if err := logpkg.Configure("info", "json"); err != nil {
			t.Fatal(err)
		}
		res := &fleet.SyncResult{
			Repo:  "acme/api",
			Drift: []string{"legacy-workflow", "experiment"},
		}
		emitSyncWarnings(res)
	})
	line := strings.TrimSpace(buf)
	if line == "" {
		t.Fatal("no output emitted")
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("not valid JSON: %v; raw=%q", err, line)
	}
	if obj["level"] != "warn" {
		t.Errorf("level = %v; want warn", obj["level"])
	}
	if obj["repo"] != "acme/api" {
		t.Errorf("repo = %v; want acme/api", obj["repo"])
	}
	drift, ok := obj["drift"].([]any)
	if !ok {
		t.Fatalf("drift field not an array: %v", obj["drift"])
	}
	if len(drift) != 2 {
		t.Errorf("drift length = %d; want 2", len(drift))
	}
	names := make(map[string]bool, len(drift))
	for _, d := range drift {
		if s, ok := d.(string); ok {
			names[s] = true
		}
	}
	for _, want := range []string{"legacy-workflow", "experiment"} {
		if !names[want] {
			t.Errorf("drift missing %q; got %v", want, drift)
		}
	}
}

func TestHintEmitsStructuredWarn(t *testing.T) {
	buf := testutil.CaptureStderr(t, func() {
		if err := logpkg.Configure("info", "json"); err != nil {
			t.Fatal(err)
		}
		emitHints("acme/api", fleet.CollectHints("Unknown property: mount-as-clis"))
	})
	var found bool
	for line := range strings.SplitSeq(strings.TrimSpace(buf), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}
		if obj["level"] != "warn" {
			continue
		}
		hint, ok := obj["hint"].(string)
		if !ok {
			continue
		}
		if !strings.Contains(hint, "mount-as-clis") {
			continue
		}
		if obj["repo"] != "acme/api" {
			t.Errorf("hint event repo = %v; want acme/api", obj["repo"])
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("no structured hint event found in stderr: %q", buf)
	}
}
