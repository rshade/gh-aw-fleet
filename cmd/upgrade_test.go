package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
	"github.com/rshade/gh-aw-fleet/internal/fleet/security"
	logpkg "github.com/rshade/gh-aw-fleet/internal/log"
	"github.com/rshade/gh-aw-fleet/internal/testutil"
)

func TestPrintUpgradeEmitsWarningsForNoChanges(t *testing.T) {
	buf := captureUpgradeWarnings(t, func(cmd *cobra.Command) {
		printUpgrade(cmd, upgradeResultWithFinding(func(res *fleet.UpgradeResult) {
			res.NoChanges = true
		}), false)
	})
	assertUpgradeWarning(t, buf)
}

func TestPrintUpgradeEmitsWarningsForConflicts(t *testing.T) {
	buf := captureUpgradeWarnings(t, func(cmd *cobra.Command) {
		printUpgrade(cmd, upgradeResultWithFinding(func(res *fleet.UpgradeResult) {
			res.Conflicts = []string{".github/workflows/test.md"}
		}), false)
	})
	assertUpgradeWarning(t, buf)
}

func TestPrintUpgradeEmitsWarningsForChangedFiles(t *testing.T) {
	buf := captureUpgradeWarnings(t, func(cmd *cobra.Command) {
		printUpgrade(cmd, upgradeResultWithFinding(func(res *fleet.UpgradeResult) {
			res.ChangedFiles = []string{".github/workflows/test.md"}
		}), false)
	})
	assertUpgradeWarning(t, buf)
}

func TestPrintUpgradeAllEmitsSummaryWarnings(t *testing.T) {
	buf := captureUpgradeWarnings(t, func(_ *cobra.Command) {
		printUpgradeSummary(&bytes.Buffer{}, []*fleet.UpgradeResult{
			upgradeResultWithFinding(func(res *fleet.UpgradeResult) {
				res.NoChanges = true
			}),
		})
	})
	assertUpgradeWarning(t, buf)
}

func captureUpgradeWarnings(t *testing.T, fn func(*cobra.Command)) string {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	return testutil.CaptureStderr(t, func() {
		if err := logpkg.Configure("info", "json"); err != nil {
			t.Fatal(err)
		}
		fn(cmd)
	})
}

func upgradeResultWithFinding(mut func(*fleet.UpgradeResult)) *fleet.UpgradeResult {
	res := &fleet.UpgradeResult{
		Repo:     "alice/widgets",
		CloneDir: "/tmp/clone",
		SecurityFindings: []security.Finding{{
			RuleID:   "fleet.test.finding",
			Severity: security.SeverityHigh,
			File:     ".github/workflows/test.md",
			Line:     7,
			Message:  "synthetic security finding",
			Remedy:   "Fix the synthetic finding.",
		}},
	}
	mut(res)
	return res
}

func assertUpgradeWarning(t *testing.T, got string) {
	t.Helper()
	for _, want := range []string{
		"fleet.test.finding",
		"synthetic security finding",
		".github/workflows/test.md",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr warning missing %q; got %q", want, got)
		}
	}
}
