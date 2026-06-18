package security

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/rshade/gh-aw-fleet/internal/fleet/fleetdiag"
)

// ---------------- Rule 1: high-frequency trigger ----------------

func TestCostScanner_PushNoSkipGuard_Medium(t *testing.T) {
	dir := stageFixture(t, "workflow-with-push-no-skip-guard.md")
	out := newCostScanner().Scan(context.Background(), dir)
	f := findRule(out, ruleIDCostHighFrequencyTrigger, false)
	if f == nil {
		t.Fatalf("expected %s finding; got %v", ruleIDCostHighFrequencyTrigger, out)
	}
	if f.Severity != SeverityMedium {
		t.Errorf("Severity = %v; want MEDIUM", f.Severity)
	}
	if f.File != ".github/workflows/workflow-with-push-no-skip-guard.md" {
		t.Errorf("File = %q; want repo-relative path", f.File)
	}
	if f.Remedy == "" {
		t.Error("Remedy is empty; should have skip-guard guidance")
	}
}

func TestCostScanner_CheckRunNoSkipGuard_Medium(t *testing.T) {
	dir := stageFixture(t, "workflow-with-check-run-no-skip-guard.md")
	out := newCostScanner().Scan(context.Background(), dir)
	f := findRule(out, ruleIDCostHighFrequencyTrigger, false)
	if f == nil {
		t.Fatalf("expected %s finding for check_run trigger; got %v", ruleIDCostHighFrequencyTrigger, out)
	}
	if f.Severity != SeverityMedium {
		t.Errorf("Severity = %v; want MEDIUM", f.Severity)
	}
}

func TestCostScanner_PushWithSkipMatchGuard_Clean(t *testing.T) {
	dir := stageFixture(t, "workflow-with-push-skip-guard.md")
	out := newCostScanner().Scan(context.Background(), dir)
	f := findRule(out, ruleIDCostHighFrequencyTrigger, false)
	if f != nil {
		t.Errorf("push with skip-if-match should produce no high-frequency finding; got %+v", f)
	}
}

// ---------------- Rule 2: reactive without skip guard ----------------

func TestCostScanner_PRNoSkipGuard_Low(t *testing.T) {
	dir := stageFixture(t, "workflow-with-pr-no-skip-guard.md")
	out := newCostScanner().Scan(context.Background(), dir)
	f := findRule(out, ruleIDCostReactiveNoSkipGuard, false)
	if f == nil {
		t.Fatalf("expected %s finding; got %v", ruleIDCostReactiveNoSkipGuard, out)
	}
	if f.Severity != SeverityLow {
		t.Errorf("Severity = %v; want LOW", f.Severity)
	}
	if f.File != ".github/workflows/workflow-with-pr-no-skip-guard.md" {
		t.Errorf("File = %q; want repo-relative path", f.File)
	}
	if f.Remedy == "" {
		t.Error("Remedy is empty; should have skip-guard guidance")
	}
}

func TestCostScanner_PRWithSkipNoMatchGuard_Clean(t *testing.T) {
	dir := stageFixture(t, "workflow-with-pr-skip-no-match.md")
	out := newCostScanner().Scan(context.Background(), dir)
	f := findRule(out, ruleIDCostReactiveNoSkipGuard, false)
	if f != nil {
		t.Errorf("pull_request with skip-if-no-match should produce no reactive finding; got %+v", f)
	}
}

func TestCostScanner_PRTargetNoSkipGuard_Low(t *testing.T) {
	dir := stageFixture(t, "workflow-with-prt-no-skip-guard.md")
	out := newCostScanner().Scan(context.Background(), dir)
	f := findRule(out, ruleIDCostReactiveNoSkipGuard, false)
	if f == nil {
		t.Fatalf("expected %s finding for pull_request_target trigger; got %v", ruleIDCostReactiveNoSkipGuard, out)
	}
	if f.Severity != SeverityLow {
		t.Errorf("Severity = %v; want LOW", f.Severity)
	}
}

// ---------------- User-dispatched triggers are clean ----------------

func TestCostScanner_WorkflowDispatch_Clean(t *testing.T) {
	// clean-agentics-workflow.md uses on: workflow_dispatch — should fire no cost findings.
	dir := stageFixture(t, "clean-agentics-workflow.md")
	out := newCostScanner().Scan(context.Background(), dir)
	if len(out) != 0 {
		t.Errorf("workflow_dispatch trigger should produce zero cost findings; got %d: %+v", len(out), out)
	}
}

// ---------------- Rule 3: scheduled without skip guard ----------------

func TestCostScanner_ScheduleNoSkipGuard_Low(t *testing.T) {
	dir := stageFixture(t, "workflow-with-write-on-schedule.md")
	out := newCostScanner().Scan(context.Background(), dir)
	f := findRule(out, ruleIDCostScheduledNoSkipGuard, false)
	if f == nil {
		t.Fatalf("expected %s finding for schedule trigger; got %v", ruleIDCostScheduledNoSkipGuard, out)
	}
	if f.Severity != SeverityLow {
		t.Errorf("Severity = %v; want LOW", f.Severity)
	}
	if f.File != ".github/workflows/workflow-with-write-on-schedule.md" {
		t.Errorf("File = %q; want repo-relative path", f.File)
	}
	if f.Remedy == "" {
		t.Error("Remedy is empty; should have skip-guard guidance")
	}
}

func TestCostScanner_ScheduleWithSkipGuard_Clean(t *testing.T) {
	dir := stageFixture(t, "workflow-with-schedule-skip-guard.md")
	out := newCostScanner().Scan(context.Background(), dir)
	f := findRule(out, ruleIDCostScheduledNoSkipGuard, false)
	if f != nil {
		t.Errorf("schedule with skip-if-match should produce no scheduled finding; got %+v", f)
	}
}

// ---------------- Diagnostic code ----------------

func TestCostScanner_DiagCodeIsCostFamily(t *testing.T) {
	dir := stageFixture(t, "workflow-with-push-no-skip-guard.md")
	out := newCostScanner().Scan(context.Background(), dir)
	if len(out) == 0 {
		t.Fatal("expected findings against push-no-skip-guard fixture")
	}
	for _, f := range out {
		if code := f.ToDiagnostic().Code; code != fleetdiag.DiagCostTriggerRisk {
			t.Errorf("ToDiagnostic().Code = %q; want %q", code, fleetdiag.DiagCostTriggerRisk)
		}
	}
}

// ---------------- Read-only contract ----------------

func TestCostScanner_ReadOnly(t *testing.T) {
	// Staging push-no-skip-guard guarantees there is at least one .md file
	// to scan. The scanner must not modify any file (Scanner contract).
	dir := stageFixture(t, "workflow-with-push-no-skip-guard.md")
	before := dirSnapshot(t, dir)
	_ = newCostScanner().Scan(context.Background(), dir)
	after := dirSnapshot(t, dir)
	if before != after {
		t.Errorf("cost scanner mutated files in the clone dir")
	}
}

// dirSnapshot returns a fingerprint of the directory by joining each file's
// relative path and byte length. This catches both creation/deletion of files
// and in-place overwrites (a scanner writing to an existing file changes its
// length unless the output is byte-identical, which is a practical guarantee).
func dirSnapshot(t *testing.T, dir string) string {
	t.Helper()
	entries := walkWorkflows(dir, ".md")
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		data, err := os.ReadFile(e.Full)
		if err != nil {
			t.Fatalf("dirSnapshot: read %s: %v", e.Full, err)
		}
		parts = append(parts, e.Rel+":"+strconv.Itoa(len(data)))
	}
	return strings.Join(parts, ",")
}

// ---------------- Empty / missing directory ----------------

func TestCostScanner_EmptyDir_Silent(t *testing.T) {
	out := newCostScanner().Scan(context.Background(), t.TempDir())
	if len(out) != 0 {
		t.Errorf("empty dir should produce zero findings; got %d: %+v", len(out), out)
	}
}

// ---------------- Malformed frontmatter is skipped silently ----------------

func TestCostScanner_MalformedFrontmatter_Silent(t *testing.T) {
	// The structural scanner emits the parse-error INFO finding; the cost
	// scanner must silently skip the file (no double-reporting).
	dir := stageFixture(t, "workflow-with-malformed-frontmatter.md")
	out := newCostScanner().Scan(context.Background(), dir)
	if len(out) != 0 {
		t.Errorf("malformed frontmatter should produce zero cost findings; got %d: %+v", len(out), out)
	}
}
