package security

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/rshade/gh-aw-fleet/internal/fleet/fleetdiag"
)

// stageFixturesIntoClone copies every *.md and *.lock.yml fixture under
// fixturesRoot into a tmp gh-aw-style clone (`.github/workflows/<name>`).
// Returns the tmp dir.
func stageFixturesIntoClone(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".github", "workflows"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	entries, err := os.ReadDir(fixturesRoot)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".md") && !strings.HasSuffix(name, ".lock.yml") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(fixturesRoot, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(tmp, ".github", "workflows", name), raw, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return tmp
}

// TestRunEndToEnd asserts SC-001 coverage. Run is invoked against the entire
// fixture set and the union of findings is checked for at least one finding
// per expected rule.
//
// FR-015 ("structural rules MUST NOT emit LOW") is enforced per-scanner in
// TestStructuralScanner_AggregateNoLowSeverity. The Renovate, Dependabot,
// and cost scanners emit LOW by design; the integration test does not
// re-assert that invariant here.
func TestRunEndToEnd(t *testing.T) {
	dir := stageFixturesIntoClone(t)
	got := Run(context.Background(), dir)
	if len(got) == 0 {
		t.Fatal("expected at least some findings end-to-end; got 0")
	}

	// FR-015 scoped to structural rules only — the cost/renovate/dependabot
	// scanners emit LOW intentionally. Use an allowlist of advisory-scanner
	// prefixes: any LOW finding whose rule ID does NOT start with one of these
	// is unexpected and fails the test.
	allowedLowPrefixes := []string{rulePrefixCost, rulePrefixRenovate, rulePrefixDependabot}
	for _, f := range got {
		if f.Severity != SeverityLow {
			continue
		}
		allowed := false
		for _, p := range allowedLowPrefixes {
			if strings.HasPrefix(f.RuleID, p) {
				allowed = true
				break
			}
		}
		if !allowed {
			t.Errorf("FR-015 violated: structural rule %q emitted LOW", f.RuleID)
		}
	}

	expectedRuleIDs := []string{
		"fleet.permissions.write-on-schedule",
		"fleet.safe-outputs.draft-false",
		"fleet.safe-outputs.missing-protected-files",
		"fleet.engine.env.non-allowlist",
		"fleet.repo-memory.main-branch",
		"fleet.mcp.non-standard-server",
		"fleet.frontmatter.parse-error",
		ruleIDCostHighFrequencyTrigger,
		ruleIDCostReactiveNoSkipGuard,
		ruleIDCostScheduledNoSkipGuard,
	}
	for _, id := range expectedRuleIDs {
		if findRule(got, id, false) == nil {
			t.Errorf("expected at least one %q finding; got none", id)
		}
	}

	// gitleaks finding (synthetic AWS key in workflow-with-fake-secret.md)
	if findRule(got, "gitleaks:", true) == nil {
		t.Errorf("expected at least one gitleaks: finding")
	}
}

// TestRunEndToEndRenovate proves the Renovate scanner's findings flow through
// the full pipeline with no caller changes (FR-013): a clone with a deficient
// renovate.json yields LOW findings on the stderr render, in the PR section,
// and as security_renovate codes in the JSON-envelope projection.
func TestRunEndToEndRenovate(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Deficient config: neither conflict rule present → two LOW findings.
	if err := os.WriteFile(filepath.Join(dir, "renovate.json"), []byte(`{"packageRules":[]}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := Run(context.Background(), dir)

	low := 0
	for _, f := range got {
		if !strings.HasPrefix(f.RuleID, "fleet.renovate.") {
			continue
		}
		low++
		if f.Severity != SeverityLow {
			t.Errorf("renovate finding %q severity = %v, want LOW", f.RuleID, f.Severity)
		}
		if code := f.ToDiagnostic().Code; code != fleetdiag.DiagSecurityRenovate {
			t.Errorf("renovate finding %q diag code = %q, want %q", f.RuleID, code, fleetdiag.DiagSecurityRenovate)
		}
	}
	if low != 2 {
		t.Fatalf("renovate LOW findings = %d, want 2: %+v", low, got)
	}

	stderr := RenderForStderr(got)
	for _, want := range []string{
		"[LOW] fleet.renovate.gh-aw-actions-not-disabled",
		"[LOW] fleet.renovate.lockfile-not-disabled",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr render missing %q:\n%s", want, stderr)
		}
	}

	pr := RenderPRSection(got)
	if !strings.Contains(pr, "## Security Findings") {
		t.Errorf("PR section missing heading:\n%s", pr)
	}
	if !strings.Contains(pr, "**LOW**") || !strings.Contains(pr, "fleet.renovate.") {
		t.Errorf("PR section missing LOW renovate bullet:\n%s", pr)
	}
}

// TestRunReproducibility asserts SC-006: two consecutive Run invocations
// produce byte-identical sorted output (stable sort + reproducibility).
func TestRunReproducibility(t *testing.T) {
	dir := stageFixturesIntoClone(t)
	first, _ := json.Marshal(Run(context.Background(), dir))
	second, _ := json.Marshal(Run(context.Background(), dir))
	if string(first) != string(second) {
		t.Errorf("Run not reproducible (SC-006):\nfirst:  %s\nsecond: %s", first, second)
	}
}

// TestStderrMatchesPRBody asserts SC-004: the rule_ids visible in
// RenderForStderr equal the rule_ids visible in RenderForPRBody (same
// set, same per-finding location triples).
func TestStderrMatchesPRBody(t *testing.T) {
	dir := stageFixturesIntoClone(t)
	got := Run(context.Background(), dir)
	if len(got) == 0 {
		t.Fatal("no findings — cannot test parity with empty input")
	}
	stderrLines := strings.Split(RenderForStderr(got), "\n")
	bodyLines := strings.Split(RenderForPRBody(got), "\n")

	// Extract (severity, rule_id) tuples from each surface. The body has a
	// summary line we drop; everything else (one bullet per finding) is a
	// per-finding entry.
	stderrTuples := make([]string, 0, len(stderrLines))
	for _, line := range stderrLines {
		if line == "" {
			continue
		}
		stderrTuples = append(stderrTuples, extractStderrTuple(line))
	}
	bodyTuples := make([]string, 0, len(bodyLines))
	for _, line := range bodyLines {
		if !strings.HasPrefix(line, "- **") {
			continue
		}
		bodyTuples = append(bodyTuples, extractBodyTuple(line))
	}

	sort.Strings(stderrTuples)
	sort.Strings(bodyTuples)

	if len(stderrTuples) != len(bodyTuples) {
		t.Errorf("count mismatch: stderr=%d, body=%d", len(stderrTuples), len(bodyTuples))
		return
	}
	for i := range stderrTuples {
		if stderrTuples[i] != bodyTuples[i] {
			t.Errorf("tuple mismatch [%d]: stderr=%q body=%q", i, stderrTuples[i], bodyTuples[i])
		}
	}
}

// TestStderrMatchesEnvelopeWarnings asserts the cmd-layer wiring contract:
// every stderr-rendered finding has a matching ToDiagnostic projection
// with the same rule_id and severity.
func TestStderrMatchesEnvelopeWarnings(t *testing.T) {
	dir := stageFixturesIntoClone(t)
	got := Run(context.Background(), dir)
	if len(got) == 0 {
		t.Fatal("no findings — cannot test parity with empty input")
	}

	stderrIDs := map[string]bool{}
	for line := range strings.SplitSeq(RenderForStderr(got), "\n") {
		if line == "" {
			continue
		}
		// Format: "[SEV] rule_id  ..."
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			stderrIDs[fields[1]] = true
		}
	}
	envIDs := map[string]bool{}
	for _, f := range got {
		d := f.ToDiagnostic()
		envIDs[d.Fields["rule_id"].(string)] = true
	}
	if len(stderrIDs) != len(envIDs) {
		t.Errorf("rule_id set sizes differ: stderr=%d envelope=%d", len(stderrIDs), len(envIDs))
	}
	for id := range stderrIDs {
		if !envIDs[id] {
			t.Errorf("stderr has rule_id %q but envelope does not", id)
		}
	}
	for id := range envIDs {
		if !stderrIDs[id] {
			t.Errorf("envelope has rule_id %q but stderr does not", id)
		}
	}
}

// extractStderrTuple turns "[HIGH] rule  file:line  msg" into "HIGH|rule|file:line".
func extractStderrTuple(line string) string {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return line
	}
	sev := strings.Trim(fields[0], "[]")
	rule := fields[1]
	loc := fields[2]
	return sev + "|" + rule + "|" + loc
}

// extractBodyTuple turns "- **HIGH** `rule` — `file:line` — msg — rem"
// into "HIGH|rule|file:line".
func extractBodyTuple(line string) string {
	// Strip the "- **" prefix.
	line = strings.TrimPrefix(line, "- **")
	sev, rest, ok := strings.Cut(line, "**")
	if !ok {
		return line
	}
	// rest starts with " `rule_id` — `file:line` — ..."
	rest = strings.TrimLeft(rest, " ")
	rest = strings.TrimPrefix(rest, "`")
	rule, rest, ok := strings.Cut(rest, "`")
	if !ok {
		return line
	}
	// Advance past the separator to the next backtick-delimited token.
	_, rest, ok = strings.Cut(rest, "`")
	if !ok {
		return sev + "|" + rule + "|"
	}
	loc, _, _ := strings.Cut(rest, "`")
	if loc == "" {
		loc = "-"
	}
	return sev + "|" + rule + "|" + loc
}
