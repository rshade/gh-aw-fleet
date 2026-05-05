// actionlint adapter — shells out to the optional `actionlint` binary
// once per `.lock.yml` file in the work-dir clone. If the binary is not
// on PATH the adapter emits exactly one INFO finding
// ("actionlint:not-installed") and the run continues — graceful
// degradation per FR-007 / SC-005. JSON-format expectation per
// research.md R2; severity mapping: actionlint exit code 1 (errors)
// → HIGH; exit code 2 (warnings) → MEDIUM; other → MEDIUM by default.

package security

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type actionlintScanner struct {
	binPath string
}

func newActionlintScanner() *actionlintScanner {
	bin, err := exec.LookPath("actionlint")
	if err != nil {
		return &actionlintScanner{binPath: ""}
	}
	return &actionlintScanner{binPath: bin}
}

// notInstalledFinding returns the single INFO finding emitted when the
// actionlint binary is missing from PATH (SC-005 graceful degradation).
func notInstalledFinding() Finding {
	return Finding{
		RuleID:   ruleIDActionlintNotInstalled,
		Severity: SeverityInfo,
		File:     "",
		Line:     0,
		Message:  "actionlint binary not found in PATH; compiled-YAML lint scanner skipped",
		Remedy: "Install actionlint (https://github.com/rhysd/actionlint) for " +
			"compiled-workflow validation. The fleet runs without it — this " +
			"is graceful degradation.",
	}
}

// actionlintDiagnostic mirrors the JSON shape emitted by
// `actionlint --format '{{json .}}'`. We only consume the fields we
// need; unknown fields are ignored (forward-compatible).
type actionlintDiagnostic struct {
	Message  string `json:"message"`
	Filepath string `json:"filepath"`
	Line     int    `json:"line"`
	Kind     string `json:"kind"`
}

// Scan runs actionlint per .lock.yml in the clone and projects each
// diagnostic into a Finding. When the binary is missing, returns one
// INFO Finding and stops.
func (s *actionlintScanner) Scan(ctx context.Context, cloneDir string) []Finding {
	if s.binPath == "" {
		return []Finding{notInstalledFinding()}
	}
	var out []Finding
	for _, w := range walkWorkflows(cloneDir, ".lock.yml") {
		out = append(out, s.scanLockFile(ctx, cloneDir, w.Rel)...)
	}
	return out
}

// scanLockFile invokes actionlint on one file and parses the JSON output.
// Both diagnostic-bearing exit codes (1 = errors, 2 = warnings) are
// expected; no diagnostics → exit 0. Exit code drives the severity
// mapping (errors → HIGH, warnings → MEDIUM).
func (s *actionlintScanner) scanLockFile(ctx context.Context, cloneDir, lockPath string) []Finding {
	// #nosec G204 -- s.binPath comes from exec.LookPath and lockPath is a
	// fixed-suffix file we walked from the gh-aw-managed clone directory.
	// Both are caller-provided paths; this is the documented contract for
	// the actionlint adapter.
	cmd := exec.CommandContext(ctx, s.binPath, "-format", "{{json .}}", lockPath)
	cmd.Dir = cloneDir
	stdout, runErr := cmd.Output()

	severity := SeverityMedium
	var ee *exec.ExitError
	if errors.As(runErr, &ee) {
		if ee.ExitCode() == 1 {
			severity = SeverityHigh
		}
	}

	if len(strings.TrimSpace(string(stdout))) == 0 {
		return nil
	}

	var diags []actionlintDiagnostic
	if jsonErr := json.Unmarshal(stdout, &diags); jsonErr != nil {
		return []Finding{{
			RuleID:   "actionlint:json-parse-error",
			Severity: SeverityInfo,
			File:     repoRelativePath(cloneDir, lockPath),
			Line:     0,
			Message: fmt.Sprintf(
				"actionlint JSON output could not be parsed: %v", jsonErr,
			),
			Remedy: "Confirm actionlint --version is a v1.x release; " +
				"the JSON shape changed or the binary is incompatible.",
		}}
	}

	out := make([]Finding, 0, len(diags))
	for _, d := range diags {
		file := d.Filepath
		if file == "" {
			file = repoRelativePath(cloneDir, lockPath)
		} else {
			file = repoRelativePath(cloneDir, file)
		}
		ruleID := "actionlint:" + d.Kind
		if d.Kind == "" {
			ruleID = "actionlint:diagnostic"
		}
		out = append(out, Finding{
			RuleID:   ruleID,
			Severity: severity,
			File:     file,
			Line:     d.Line,
			Message:  d.Message,
			Remedy:   "Fix the workflow YAML or update the source markdown that compiled to this lock file. See actionlint documentation for rule details.",
		})
	}
	return out
}

// repoRelativePath normalizes actionlint file paths so findings surface
// with .github/workflows/foo.lock.yml form, matching how the gitleaks and
// structural scanners emit File.
func repoRelativePath(cloneDir, path string) string {
	if filepath.IsAbs(path) {
		rel, err := filepath.Rel(cloneDir, path)
		if err != nil {
			return filepath.ToSlash(path)
		}
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(filepath.Clean(path))
}
