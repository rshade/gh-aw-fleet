package fleet

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/rshade/gh-aw-fleet/internal/fleet/security"
)

// withStdoutTerminal overrides the stdoutIsTerminal seam for one test and
// restores it on cleanup.
func withStdoutTerminal(t *testing.T, isTTY bool) {
	t.Helper()
	orig := stdoutIsTerminal
	t.Cleanup(func() { stdoutIsTerminal = orig })
	stdoutIsTerminal = func() bool { return isTTY }
}

func oneHighFinding() []security.Finding {
	return []security.Finding{{RuleID: "fleet.permissions.write-on-schedule", Severity: security.SeverityHigh}}
}

func TestPromptUserDecisionTable(t *testing.T) {
	cases := []struct {
		name        string
		findings    []security.Finding
		yes         bool
		tty         bool
		stdin       string
		wantProceed bool
		wantErr     bool
		wantOutput  bool // whether the summary/prompt line was written
	}{
		{name: "a empty findings auto-proceeds silently", findings: nil, tty: true, wantProceed: true},
		{name: "b yes auto-proceeds silently", findings: oneHighFinding(), yes: true, tty: true, wantProceed: true},
		{name: "c non-tty auto-proceeds silently", findings: oneHighFinding(), tty: false, wantProceed: true},
		{
			name:        "d interactive y accepts",
			findings:    oneHighFinding(),
			tty:         true,
			stdin:       "y\n",
			wantProceed: true,
			wantOutput:  true,
		},
		{
			name:        "e interactive n declines",
			findings:    oneHighFinding(),
			tty:         true,
			stdin:       "n\n",
			wantProceed: false,
			wantOutput:  true,
		},
		{
			name:        "f interactive EOF declines with error",
			findings:    oneHighFinding(),
			tty:         true,
			stdin:       "",
			wantProceed: false,
			wantErr:     true,
			wantOutput:  true,
		},
		{
			name:        "empty line is default-No",
			findings:    oneHighFinding(),
			tty:         true,
			stdin:       "\n",
			wantProceed: false,
			wantOutput:  true,
		},
		{
			name:        "uppercase Y accepts",
			findings:    oneHighFinding(),
			tty:         true,
			stdin:       "Y\n",
			wantProceed: true,
			wantOutput:  true,
		},
		{
			name:        "YES accepts",
			findings:    oneHighFinding(),
			tty:         true,
			stdin:       "YES\n",
			wantProceed: true,
			wantOutput:  true,
		},
		{
			name:        "other word declines",
			findings:    oneHighFinding(),
			tty:         true,
			stdin:       "maybe\n",
			wantProceed: false,
			wantOutput:  true,
		},
		{
			name:        "INFO-only findings still fire the prompt (FR-015)",
			findings:    []security.Finding{{RuleID: "actionlint:not-installed", Severity: security.SeverityInfo}},
			tty:         true,
			stdin:       "n\n",
			wantProceed: false,
			wantOutput:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withStdoutTerminal(t, tc.tty)
			var out bytes.Buffer
			proceed, err := PromptUser(tc.findings, tc.yes, strings.NewReader(tc.stdin), &out)
			if proceed != tc.wantProceed {
				t.Errorf("proceed = %v; want %v", proceed, tc.wantProceed)
			}
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v; wantErr = %v", err, tc.wantErr)
			}
			if (out.Len() > 0) != tc.wantOutput {
				t.Errorf("output written = %q; wantOutput = %v", out.String(), tc.wantOutput)
			}
		})
	}
}

// TestPromptUserSummaryContainsSeverityTally asserts the prompt line embeds the
// shared SeveritySummary string so the count matches the PR-body summary.
func TestPromptUserSummaryContainsSeverityTally(t *testing.T) {
	withStdoutTerminal(t, true)
	findings := []security.Finding{
		{Severity: security.SeverityHigh},
		{Severity: security.SeverityHigh},
		{Severity: security.SeverityMedium},
	}
	var out bytes.Buffer
	if _, err := PromptUser(findings, false, strings.NewReader("y\n"), &out); err != nil {
		t.Fatalf("PromptUser err = %v", err)
	}
	want := security.SeveritySummary(findings) // "2 HIGH, 1 MEDIUM"
	if !strings.Contains(out.String(), want) {
		t.Errorf("prompt %q does not contain summary %q", out.String(), want)
	}
	if !strings.Contains(out.String(), "[y/N]") {
		t.Errorf("prompt %q missing [y/N] default marker", out.String())
	}
}

// TestPromptUserFastPathsNeverRead proves the empty/yes/non-tty branches return
// before consulting the reader (so a non-interactive run never blocks on stdin).
func TestPromptUserFastPathsNeverRead(t *testing.T) {
	cases := []struct {
		name     string
		findings []security.Finding
		yes      bool
		tty      bool
	}{
		{name: "empty findings", findings: nil, tty: true},
		{name: "yes set", findings: oneHighFinding(), yes: true, tty: true},
		{name: "non-tty", findings: oneHighFinding(), tty: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withStdoutTerminal(t, tc.tty)
			var out bytes.Buffer
			proceed, err := PromptUser(tc.findings, tc.yes, failingReader{t}, &out)
			if !proceed || err != nil {
				t.Fatalf("proceed=%v err=%v; want true/nil", proceed, err)
			}
			if out.Len() != 0 {
				t.Errorf("fast path wrote %q; want no output", out.String())
			}
		})
	}
}

// TestConfirmSecurityFindingsDeclinePreservesAndTypes verifies the wrapper flips
// the cleanup flag and returns a typed *OperatorDeclinedError carrying the read
// cause on EOF.
func TestConfirmSecurityFindingsDeclinePreservesAndTypes(t *testing.T) {
	withStdoutTerminal(t, true)
	restoreIn := promptStdin
	restoreOut := promptStdout
	t.Cleanup(func() { promptStdin = restoreIn; promptStdout = restoreOut })
	promptStdin = strings.NewReader("") // EOF
	promptStdout = io.Discard

	cleanup := true
	err := confirmSecurityFindings("rshade/example", oneHighFinding(), SecurityOpts{}, &cleanup)
	if !IsOperatorDeclinedError(err) {
		t.Fatalf("err = %T %[1]v; want *OperatorDeclinedError", err)
	}
	if cleanup {
		t.Error("cleanupClone not flipped to false on decline")
	}
	var declined *OperatorDeclinedError
	if errors.As(err, &declined) {
		if declined.Repo != "rshade/example" || declined.Findings != 1 {
			t.Errorf("declined = %+v; want repo=rshade/example findings=1", declined)
		}
		if !errors.Is(err, io.EOF) {
			t.Error("EOF decline should keep io.EOF in the Unwrap chain")
		}
	}
}

// TestConfirmSecurityFindingsProceed verifies the wrapper returns nil and does
// not touch the cleanup flag when the operator proceeds.
func TestConfirmSecurityFindingsProceed(t *testing.T) {
	withStdoutTerminal(t, true)
	restoreIn := promptStdin
	restoreOut := promptStdout
	t.Cleanup(func() { promptStdin = restoreIn; promptStdout = restoreOut })
	promptStdin = strings.NewReader("y\n")
	promptStdout = io.Discard

	cleanup := true
	if err := confirmSecurityFindings("rshade/example", oneHighFinding(), SecurityOpts{}, &cleanup); err != nil {
		t.Fatalf("confirmSecurityFindings err = %v; want nil", err)
	}
	if !cleanup {
		t.Error("cleanupClone should be untouched when the operator proceeds")
	}
}

// failingReader fails the test if Read is ever called — used to prove the
// fast paths never consult stdin.
type failingReader struct{ t *testing.T }

func (r failingReader) Read([]byte) (int, error) {
	r.t.Fatal("Read called on a fast-path that should never read stdin")
	return 0, io.EOF
}
