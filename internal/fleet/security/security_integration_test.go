package security

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
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

// TestRunEndToEnd asserts SC-001 coverage and FR-015 ("v1 MUST NOT emit
// LOW"). Run is invoked against the entire fixture set and the union of
// findings is checked for: at least one finding per expected rule, and
// zero LOW findings overall.
func TestRunEndToEnd(t *testing.T) {
	dir := stageFixturesIntoClone(t)
	got := Run(context.Background(), dir)
	if len(got) == 0 {
		t.Fatal("expected at least some findings end-to-end; got 0")
	}

	// FR-015 invariant: v1 must not emit LOW.
	for _, f := range got {
		if f.Severity == SeverityLow {
			t.Errorf("FR-015 violated: %+v has SeverityLow", f)
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
	endSev := strings.Index(line, "**")
	if endSev < 0 {
		return line
	}
	sev := line[:endSev]
	rest := line[endSev+2:]
	// rest starts with " `rule_id` — `file:line` — ..."
	rest = strings.TrimLeft(rest, " ")
	rest = strings.TrimPrefix(rest, "`")
	endRule := strings.Index(rest, "`")
	if endRule < 0 {
		return line
	}
	rule := rest[:endRule]
	rest = rest[endRule+1:]
	idx := strings.Index(rest, "`")
	if idx < 0 {
		return sev + "|" + rule + "|"
	}
	rest = rest[idx+1:]
	endLoc := strings.Index(rest, "`")
	if endLoc < 0 {
		return sev + "|" + rule + "|"
	}
	loc := rest[:endLoc]
	if loc == "" {
		loc = "-"
	}
	return sev + "|" + rule + "|" + loc
}
