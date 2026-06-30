package fleet

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rshade/gh-aw-fleet/internal/fleet/security"
)

// stdoutIsTerminal reports whether stdout is an interactive character device.
// It is the interactivity gate for the security-findings prompt: the question
// is written to stdout, so detection keys on stdout (not stdin) — `tool | tee`
// must not prompt into a redirected stream the operator never sees. Stdlib
// only (mirrors cmd/add.go's isStdinTerminal), so no golang.org/x/term direct
// dependency. Overridable in tests.
//
//nolint:gochecknoglobals // test-injection seam mirroring cmd/add.go isStdinTerminal
var stdoutIsTerminal = func() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// promptStdin and promptStdout are the production reader/writer the
// confirmSecurityFindings wrapper feeds to PromptUser. They are package-level
// seams so placement tests can inject scripted input and capture output
// without a real terminal.
//
//nolint:gochecknoglobals // test-injection seams for the interactive prompt
var (
	promptStdin  io.Reader = os.Stdin
	promptStdout io.Writer = os.Stdout
)

// PromptUser asks the operator to confirm proceeding to commit despite security
// findings. It returns (true, nil) on the non-interactive fast paths — findings
// is empty, yes is true, or stdout is not a terminal — without reading in or
// writing out. Otherwise it writes a one-line severity summary plus a `[y/N]`
// prompt to out and reads a single line from in: "y"/"Y"/"yes" (trimmed,
// case-insensitive) returns (true, nil); any other non-empty or empty line
// returns (false, nil); an EOF or read error before an answer returns
// (false, err). The safe default is No, so a bare Enter declines.
func PromptUser(findings []security.Finding, yes bool, in io.Reader, out io.Writer) (bool, error) {
	if len(findings) == 0 || yes || !stdoutIsTerminal() {
		return true, nil
	}
	fmt.Fprintf(out, "⚠  %s. Proceed with commit? [y/N] ", security.SeveritySummary(findings))
	line, readErr := bufio.NewReader(in).ReadString('\n')
	if readErr != nil && readErr != io.EOF {
		return false, readErr
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	case "":
		// A bare Enter is default-No (readErr nil); an empty read at EOF is a
		// no-answer decline that keeps the EOF cause for the Unwrap chain. The
		// guard above leaves readErr as exactly nil or io.EOF, so returning it
		// covers both. (default, by contrast, discards an incidental EOF on a
		// non-empty explicit answer.)
		return false, readErr
	default:
		return false, nil
	}
}

// OperatorDeclinedError is returned when the operator declines the interactive
// security-findings confirmation — an explicit "no", a bare Enter, EOF, or a
// read error. The cmd layer recognizes it via IsOperatorDeclinedError and
// surfaces it as a clean, non-crash, non-zero exit.
type OperatorDeclinedError struct {
	Repo     string // repository the apply was aborted for
	Findings int    // number of findings shown at the prompt
	Cause    error  // non-nil for EOF / read-error declines; nil for an explicit "no"
}

// Error returns the actionable abort message naming the finding count, the
// repo, and the --yes escape hatch.
func (e *OperatorDeclinedError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf(
		"aborted by operator: %d security finding(s) for %s not accepted; "+
			"re-run with --yes to skip the prompt",
		e.Findings, e.Repo,
	)
}

// Unwrap returns the read cause so EOF / read errors remain inspectable via
// errors.Is / errors.As.
func (e *OperatorDeclinedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// IsOperatorDeclinedError reports whether err is (or wraps) an
// *OperatorDeclinedError. Mirrors IsStrictSecurityError so cmd/output.go can
// map a deliberate decline to a clean exit without the hint engine.
func IsOperatorDeclinedError(err error) bool {
	var declined *OperatorDeclinedError
	return errors.As(err, &declined)
}

// confirmSecurityFindings runs PromptUser with the production stdin/stdout
// seams and, on decline, preserves the work-dir clone (sets *cleanupClone =
// false when the pointer is non-nil; resume paths already disable cleanup and
// pass nil) and returns a typed *OperatorDeclinedError. Returns nil when the
// operator proceeds — including every non-interactive fast path.
func confirmSecurityFindings(repo string, findings []security.Finding, opts SecurityOpts, cleanupClone *bool) error {
	proceed, err := PromptUser(findings, opts.Yes, promptStdin, promptStdout)
	if proceed {
		return nil
	}
	if cleanupClone != nil {
		*cleanupClone = false
	}
	return &OperatorDeclinedError{Repo: repo, Findings: len(findings), Cause: err}
}
