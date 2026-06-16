package security

import (
	"context"
	"testing"

	"github.com/rshade/gh-aw-fleet/internal/fleet/fleetdiag"
)

func TestSeverityString(t *testing.T) {
	cases := []struct {
		sev  Severity
		want string
	}{
		{SeverityInfo, "INFO"},
		{SeverityLow, "LOW"},
		{SeverityMedium, "MEDIUM"},
		{SeverityHigh, "HIGH"},
	}
	for _, tc := range cases {
		if got := tc.sev.String(); got != tc.want {
			t.Errorf("Severity(%d).String() = %q; want %q", tc.sev, got, tc.want)
		}
	}
}

func TestFindingSortDeterministicTieBreakers(t *testing.T) {
	in := []Finding{
		{RuleID: "a", Severity: SeverityMedium, File: "z.md", Line: 5},
		{RuleID: "b", Severity: SeverityHigh, File: "b.md", Line: 7},
		{RuleID: "c", Severity: SeverityHigh, File: "a.md", Line: 9},
		{RuleID: "d", Severity: SeverityHigh, File: "a.md", Line: 1},
		{RuleID: "e", Severity: SeverityInfo, File: "a.md", Line: 0},
		{RuleID: "f", Severity: SeverityMedium, File: "z.md", Line: 5},
		{RuleID: "same", Severity: SeverityHigh, File: "same.md", Line: 10, Message: "z", Remedy: "a"},
		{RuleID: "same", Severity: SeverityHigh, File: "same.md", Line: 10, Message: "a", Remedy: "z"},
		{RuleID: "same", Severity: SeverityHigh, File: "same.md", Line: 10, Message: "a", Remedy: "a"},
		{RuleID: "aaa", Severity: SeverityHigh, File: "same.md", Line: 10, Message: "z", Remedy: "z"},
	}
	got := runWithScanners(context.Background(), "", []Scanner{stubScanner{out: in}})

	wantOrder := []string{"d", "c", "b", "aaa", "same", "same", "same", "a", "f", "e"}
	if len(got) != len(wantOrder) {
		t.Fatalf("len(got) = %d; want %d", len(got), len(wantOrder))
	}
	for i, w := range wantOrder {
		if got[i].RuleID != w {
			t.Errorf("got[%d].RuleID = %q; want %q (full order: %v)", i, got[i].RuleID, w, ruleIDsOf(got))
		}
	}
	if got[4].Message != "a" || got[4].Remedy != "a" {
		t.Errorf("first same-line finding = %q/%q; want a/a", got[4].Message, got[4].Remedy)
	}
	if got[5].Message != "a" || got[5].Remedy != "z" {
		t.Errorf("second same-line finding = %q/%q; want a/z", got[5].Message, got[5].Remedy)
	}
	if got[6].Message != "z" || got[6].Remedy != "a" {
		t.Errorf("third same-line finding = %q/%q; want z/a", got[6].Message, got[6].Remedy)
	}
}

func TestToDiagnosticShape(t *testing.T) {
	f := Finding{
		RuleID:   "fleet.permissions.write-on-schedule",
		Severity: SeverityHigh,
		File:     ".github/workflows/x.md",
		Line:     12,
		Message:  "msg",
		Remedy:   "rem",
	}
	d := f.ToDiagnostic()
	if d.Code != fleetdiag.DiagSecurityWriteOnSchedule {
		t.Errorf("Code = %q; want %q", d.Code, fleetdiag.DiagSecurityWriteOnSchedule)
	}
	if d.Message != "msg" {
		t.Errorf("Message = %q; want %q", d.Message, "msg")
	}
	if d.Fields["severity"] != "HIGH" {
		t.Errorf("severity field = %v; want HIGH", d.Fields["severity"])
	}
	if d.Fields["rule_id"] != f.RuleID {
		t.Errorf("rule_id field = %v; want %q", d.Fields["rule_id"], f.RuleID)
	}
	if d.Fields["file"] != f.File {
		t.Errorf("file field = %v; want %q", d.Fields["file"], f.File)
	}
	if d.Fields["line"] != 12 {
		t.Errorf("line field = %v; want 12", d.Fields["line"])
	}
	if d.Fields["remedy"] != "rem" {
		t.Errorf("remedy field = %v; want rem", d.Fields["remedy"])
	}
}

func TestRunScannerCleanReturnsNonNilEmpty(t *testing.T) {
	got := runWithScanners(context.Background(), "", []Scanner{stubScanner{out: nil}})
	if got == nil {
		t.Fatal("clean scanner run returned nil; want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("len(got) = %d; want 0", len(got))
	}
}

func TestDiagCodeForRuleIDPrefixMapping(t *testing.T) {
	cases := map[string]string{
		"gitleaks:aws-access-key":                    fleetdiag.DiagSecurityCredential,
		"fleet.permissions.write-on-schedule":        fleetdiag.DiagSecurityWriteOnSchedule,
		"fleet.safe-outputs.draft-false":             fleetdiag.DiagSecurityDraftFalse,
		"fleet.safe-outputs.missing-protected-files": fleetdiag.DiagSecurityMissingProtectedFiles,
		"fleet.engine.env.non-allowlist":             fleetdiag.DiagSecurityEngineEnvNonAllowlist,
		"fleet.repo-memory.main-branch":              fleetdiag.DiagSecurityRepoMemoryMain,
		"fleet.mcp.non-standard-server":              fleetdiag.DiagSecurityMCPNonStandardHost,
		"actionlint:syntax-check":                    fleetdiag.DiagSecurityActionlint,
		"actionlint:not-installed":                   fleetdiag.DiagSecurityActionlint,
		"fleet.frontmatter.parse-error":              fleetdiag.DiagSecurityFrontmatterParseError,
		"fleet.renovate.gh-aw-actions-not-disabled":  fleetdiag.DiagSecurityRenovate,
		"fleet.renovate.lockfile-not-disabled":       fleetdiag.DiagSecurityRenovate,
		"fleet.renovate.parse-error":                 fleetdiag.DiagSecurityRenovate,
		"unknown:rule":                               fleetdiag.DiagHint,
	}
	for ruleID, want := range cases {
		if got := diagCodeForRuleID(ruleID); got != want {
			t.Errorf("diagCodeForRuleID(%q) = %q; want %q", ruleID, got, want)
		}
	}
}

func TestRunScannerPanicDoesNotAbortRun(t *testing.T) {
	scanners := []Scanner{
		panicScanner{},
		stubScanner{out: []Finding{
			{RuleID: "fleet.test.high", Severity: SeverityHigh, File: ".github/workflows/a.md", Line: 1, Message: "ok"},
		}},
	}
	got := runWithScanners(context.Background(), "", scanners)

	var sawPanic bool
	var sawHigh bool
	for _, f := range got {
		if f.RuleID == "fleet.scanner.panic" && f.Severity == SeverityInfo {
			sawPanic = true
		}
		if f.RuleID == "fleet.test.high" && f.Severity == SeverityHigh {
			sawHigh = true
		}
	}
	if !sawPanic {
		t.Errorf("expected one INFO finding with rule_id=fleet.scanner.panic, got %v", ruleIDsOf(got))
	}
	if !sawHigh {
		t.Errorf("expected the surviving scanner's HIGH finding to be present, got %v", ruleIDsOf(got))
	}
}

type stubScanner struct{ out []Finding }

func (s stubScanner) Scan(_ context.Context, _ string) []Finding { return s.out }

type panicScanner struct{}

func (panicScanner) Scan(_ context.Context, _ string) []Finding { panic("boom") }

func ruleIDsOf(fs []Finding) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.RuleID
	}
	return out
}
