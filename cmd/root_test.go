package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	logpkg "github.com/rshade/gh-aw-fleet/internal/log"
	"github.com/rshade/gh-aw-fleet/internal/testutil"
)

// TestRootSilenceFlagsSet pins the two cobra silence flags on NewRootCmd.
// Issue #30: errors double-printed because cobra and main.go both wrote.
// SilenceErrors=true makes main.go the single error surface; SilenceUsage=true
// keeps help text out of error output. Flipping either regresses the fix.
func TestRootSilenceFlagsSet(t *testing.T) {
	root := NewRootCmd()
	if !root.SilenceErrors {
		t.Error("root.SilenceErrors = false; want true (see issue #30 — main.go owns error output)")
	}
	if !root.SilenceUsage {
		t.Error("root.SilenceUsage = false; want true")
	}
}

// TestRootExecuteKeepsCobraErrorOffStderr verifies the behavioral effect of
// SilenceErrors: when a subcommand returns an error, cobra must not write to
// stderr, and Execute must return the error unchanged so main.go can print it.
func TestRootExecuteKeepsCobraErrorOffStderr(t *testing.T) {
	sentinel := errors.New("sentinel-root-test-failure")

	root := NewRootCmd()
	root.AddCommand(&cobra.Command{
		Use: "__test_fail__",
		RunE: func(_ *cobra.Command, _ []string) error {
			return sentinel
		},
	})
	root.SetArgs([]string{"__test_fail__"})

	t.Cleanup(func() {
		// CaptureStderr swaps os.Stderr and PersistentPreRunE reconfigures the
		// global logger against it; restore a live sink for downstream tests.
		_ = logpkg.Configure("info", "console")
	})

	var execErr error
	stderr := testutil.CaptureStderr(t, func() {
		execErr = root.Execute()
	})

	if !errors.Is(execErr, sentinel) {
		t.Fatalf("Execute() = %v; want sentinel %v", execErr, sentinel)
	}
	if strings.Contains(stderr, sentinel.Error()) {
		t.Errorf("cobra wrote error message to stderr (want silent); stderr=%q", stderr)
	}
	if strings.Contains(stderr, "Error:") {
		t.Errorf("cobra's 'Error:' prefix appeared on stderr — SilenceErrors regressed; stderr=%q", stderr)
	}
}
