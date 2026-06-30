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

func TestSeveritySummaryEmpty(t *testing.T) {
	if got := SeveritySummary(nil); got != "" {
		t.Errorf("SeveritySummary(nil) = %q; want empty", got)
	}
	if got := SeveritySummary([]Finding{}); got != "" {
		t.Errorf("SeveritySummary([]) = %q; want empty", got)
	}
}

func TestSeveritySummaryTallyOrderAndOmission(t *testing.T) {
	cases := []struct {
		name string
		in   []Finding
		want string
	}{
		{
			name: "single high",
			in:   []Finding{{Severity: SeverityHigh}},
			want: "1 HIGH",
		},
		{
			name: "mixed orders HIGH before MEDIUM regardless of input order",
			in: []Finding{
				{Severity: SeverityMedium},
				{Severity: SeverityHigh},
				{Severity: SeverityHigh},
			},
			want: "2 HIGH, 1 MEDIUM",
		},
		{
			name: "all four buckets in HIGH→MEDIUM→LOW→INFO order",
			in: []Finding{
				{Severity: SeverityInfo},
				{Severity: SeverityLow},
				{Severity: SeverityMedium},
				{Severity: SeverityHigh},
			},
			want: "1 HIGH, 1 MEDIUM, 1 LOW, 1 INFO",
		},
		{
			name: "zero counts omitted (only LOW present)",
			in:   []Finding{{Severity: SeverityLow}, {Severity: SeverityLow}},
			want: "2 LOW",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SeveritySummary(tc.in); got != tc.want {
				t.Errorf("SeveritySummary() = %q; want %q", got, tc.want)
			}
		})
	}
}
