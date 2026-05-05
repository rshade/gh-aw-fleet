package security

import "testing"

func TestRenderForStderrEmpty(t *testing.T) {
	if got := RenderForStderr(nil); got != "" {
		t.Errorf("RenderForStderr(nil) = %q; want empty", got)
	}
	if got := RenderForStderr([]Finding{}); got != "" {
		t.Errorf("RenderForStderr([]) = %q; want empty", got)
	}
}

func TestRenderForStderrMultiFindingGolden(t *testing.T) {
	in := []Finding{
		{
			RuleID:   "gitleaks:aws-access-key",
			Severity: SeverityHigh,
			File:     ".github/workflows/foo.md",
			Line:     23,
			Message:  "AWS Access Key (<redacted>)",
		},
		{
			RuleID:   "actionlint:not-installed",
			Severity: SeverityInfo,
			File:     "",
			Line:     0,
			Message:  "actionlint binary not found in PATH; compiled-YAML lint scanner skipped",
		},
	}
	want := "[HIGH] gitleaks:aws-access-key  .github/workflows/foo.md:23  AWS Access Key (<redacted>)\n" +
		"[INFO] actionlint:not-installed  -  actionlint binary not found in PATH; compiled-YAML lint scanner skipped"
	if got := RenderForStderr(in); got != want {
		t.Errorf("RenderForStderr() mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestRenderForStderrFileSlotCorners(t *testing.T) {
	cases := []struct {
		f    Finding
		want string
	}{
		{Finding{RuleID: "x", Severity: SeverityHigh, File: "foo.md", Line: 0, Message: "m"}, "[HIGH] x  foo.md  m"},
		{Finding{RuleID: "y", Severity: SeverityInfo, File: "", Line: 0, Message: "m"}, "[INFO] y  -  m"},
		{Finding{RuleID: "z", Severity: SeverityMedium, File: "a.md", Line: 5, Message: "m"}, "[MEDIUM] z  a.md:5  m"},
	}
	for _, tc := range cases {
		if got := RenderForStderr([]Finding{tc.f}); got != tc.want {
			t.Errorf("file-slot for %+v\n got %q\nwant %q", tc.f, got, tc.want)
		}
	}
}
