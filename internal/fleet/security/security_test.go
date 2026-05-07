package security

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// fixturesRoot is the path to the on-disk testdata directory for a single
// fixture file. Each test that exercises a single rule copies the named
// fixture into a tmp clone-style directory, then calls security.Run on
// that tmp dir so assertions are independent of which other fixtures
// happen to coexist.
const fixturesRoot = "testdata/security"

// stageFixture copies one fixture file into a tmp directory shaped like a
// gh-aw work-dir clone (`.github/workflows/<name>`), and returns the tmp
// dir. The clone-style layout is what security.Run walks.
func stageFixture(t *testing.T, fixtureName string) string {
	t.Helper()
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".github", "workflows"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := filepath.Join(fixturesRoot, fixtureName)
	content, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture %s: %v", src, err)
	}
	dst := filepath.Join(tmp, ".github", "workflows", fixtureName)
	if err := os.WriteFile(dst, content, 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", dst, err)
	}
	return tmp
}

// findRule returns the first finding in fs whose RuleID equals (or starts
// with, when prefix=true) the given id. Returns nil when none matches.
func findRule(fs []Finding, id string, prefix bool) *Finding {
	for i := range fs {
		if prefix && strings.HasPrefix(fs[i].RuleID, id) {
			return &fs[i]
		}
		if !prefix && fs[i].RuleID == id {
			return &fs[i]
		}
	}
	return nil
}

// countSeverity returns how many findings have the given severity.
func countSeverity(fs []Finding, sev Severity) int {
	n := 0
	for _, f := range fs {
		if f.Severity == sev {
			n++
		}
	}
	return n
}

// ---------------- US1: gitleaks ----------------

func TestGitleaksScanner_FakeSecret(t *testing.T) {
	dir := stageFixture(t, "workflow-with-fake-secret.md")
	out := newGitleaksScanner().Scan(context.Background(), dir)
	if len(out) == 0 {
		t.Fatal("expected at least one gitleaks finding for fake AKIA secret; got 0")
	}
	f := out[0]
	if !strings.HasPrefix(f.RuleID, "gitleaks:") {
		t.Errorf("RuleID = %q; want prefix gitleaks:", f.RuleID)
	}
	if f.Severity != SeverityHigh {
		t.Errorf("Severity = %v; want HIGH", f.Severity)
	}
	if f.File != ".github/workflows/workflow-with-fake-secret.md" {
		t.Errorf("File = %q; want relative .github/workflows/workflow-with-fake-secret.md", f.File)
	}
	if f.Line == 0 {
		t.Errorf("Line = 0; want non-zero (gitleaks reports source line)")
	}
	if f.Remedy == "" {
		t.Error("Remedy is empty; should have rotation guidance")
	}
}

func TestGitleaksScanner_RedactionEnforced(t *testing.T) {
	dir := stageFixture(t, "workflow-with-fake-secret.md")
	out := newGitleaksScanner().Scan(context.Background(), dir)
	if len(out) == 0 {
		t.Fatal("expected at least one gitleaks finding")
	}
	for _, f := range out {
		if strings.Contains(f.Message, "AKIAJ7Z8X3K9Q4MNP2WV") {
			t.Errorf("FR-008a violated: matched literal leaked into Message: %q", f.Message)
		}
		if !strings.Contains(f.Message, "<redacted>") {
			t.Errorf("Message %q should contain <redacted> placeholder", f.Message)
		}
	}
}

func TestGitleaksScanner_CleanAgenticsWorkflow(t *testing.T) {
	dir := stageFixture(t, "clean-agentics-workflow.md")
	out := newGitleaksScanner().Scan(context.Background(), dir)
	if len(out) != 0 {
		t.Errorf("clean workflow should produce zero gitleaks findings; got %d: %+v", len(out), out)
	}
}

// ---------------- US2: structural ----------------

func TestStructuralScanner_WriteOnSchedule(t *testing.T) {
	dir := stageFixture(t, "workflow-with-write-on-schedule.md")
	out := newStructuralScanner().Scan(context.Background(), dir)
	f := findRule(out, "fleet.permissions.write-on-schedule", false)
	if f == nil {
		t.Fatalf("expected fleet.permissions.write-on-schedule finding; got %v", out)
	}
	if f.Severity != SeverityHigh {
		t.Errorf("Severity = %v; want HIGH", f.Severity)
	}
	if f.File != ".github/workflows/workflow-with-write-on-schedule.md" {
		t.Errorf("File = %q; want relative path", f.File)
	}
}

func TestStructuralScanner_DraftFalse(t *testing.T) {
	dir := stageFixture(t, "workflow-with-draft-false.md")
	out := newStructuralScanner().Scan(context.Background(), dir)
	f := findRule(out, "fleet.safe-outputs.draft-false", false)
	if f == nil {
		t.Fatalf("expected fleet.safe-outputs.draft-false finding; got %v", out)
	}
	if f.Severity != SeverityMedium {
		t.Errorf("Severity = %v; want MEDIUM", f.Severity)
	}
}

func TestStructuralScanner_MissingProtectedFiles(t *testing.T) {
	dir := stageFixture(t, "workflow-with-missing-protected-files.md")
	out := newStructuralScanner().Scan(context.Background(), dir)
	f := findRule(out, "fleet.safe-outputs.missing-protected-files", false)
	if f == nil {
		t.Fatalf("expected fleet.safe-outputs.missing-protected-files finding; got %v", out)
	}
	if f.Severity != SeverityMedium {
		t.Errorf("Severity = %v; want MEDIUM", f.Severity)
	}
}

func TestStructuralScanner_EngineEnvNonAllowlist(t *testing.T) {
	dir := stageFixture(t, "workflow-with-engine-env-non-allowlist.md")
	out := newStructuralScanner().Scan(context.Background(), dir)
	f := findRule(out, "fleet.engine.env.non-allowlist", false)
	if f == nil {
		t.Fatalf("expected fleet.engine.env.non-allowlist finding; got %v", out)
	}
	if f.Severity != SeverityHigh {
		t.Errorf("Severity = %v; want HIGH (engine resolved as claude); got %v", f.Severity, out)
	}
}

func TestStructuralScanner_MissingEngine_INFO(t *testing.T) {
	dir := stageFixture(t, "workflow-with-missing-engine.md")
	out := newStructuralScanner().Scan(context.Background(), dir)
	f := findRule(out, "fleet.engine.env.non-allowlist", false)
	if f == nil {
		t.Fatalf("expected fleet.engine.env.non-allowlist INFO finding; got %v", out)
	}
	if f.Severity != SeverityInfo {
		t.Errorf("Severity = %v; want INFO (FR-018); got finding %+v", f.Severity, f)
	}
}

func TestStructuralScanner_RepoMemoryMain(t *testing.T) {
	dir := stageFixture(t, "workflow-with-repo-memory-main.md")
	out := newStructuralScanner().Scan(context.Background(), dir)
	f := findRule(out, "fleet.repo-memory.main-branch", false)
	if f == nil {
		t.Fatalf("expected fleet.repo-memory.main-branch finding; got %v", out)
	}
	if f.Severity != SeverityHigh {
		t.Errorf("Severity = %v; want HIGH", f.Severity)
	}
}

func TestStructuralScanner_MCPNonStandardHost(t *testing.T) {
	dir := stageFixture(t, "workflow-with-mcp-npm-host.md")
	out := newStructuralScanner().Scan(context.Background(), dir)
	f := findRule(out, "fleet.mcp.non-standard-server", false)
	if f == nil {
		t.Fatalf("expected fleet.mcp.non-standard-server finding; got %v", out)
	}
	if f.Severity != SeverityHigh {
		t.Errorf("Severity = %v; want HIGH", f.Severity)
	}
}

func TestStructuralScanner_MalformedFrontmatter_INFO(t *testing.T) {
	dir := stageFixture(t, "workflow-with-malformed-frontmatter.md")
	out := newStructuralScanner().Scan(context.Background(), dir)
	f := findRule(out, "fleet.frontmatter.parse-error", false)
	if f == nil {
		t.Fatalf("expected fleet.frontmatter.parse-error INFO finding; got %v", out)
	}
	if f.Severity != SeverityInfo {
		t.Errorf("Severity = %v; want INFO", f.Severity)
	}
	// Other scanners (gitleaks) should still scan the file by raw bytes.
	gl := newGitleaksScanner().Scan(context.Background(), dir)
	// Asserts gitleaks ran without panicking even on malformed YAML.
	_ = gl
}

func TestStructuralScanner_CleanAgenticsWorkflow(t *testing.T) {
	dir := stageFixture(t, "clean-agentics-workflow.md")
	out := newStructuralScanner().Scan(context.Background(), dir)
	if len(out) != 0 {
		t.Errorf("clean workflow should produce zero structural findings; got %d: %+v", len(out), out)
	}
}

func TestADR26919AllowlistMatchesFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(fixturesRoot, "adr-26919-allowlist.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture struct {
		Engines map[string][]string `json:"engines"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if len(fixture.Engines) != len(adr26919Allowlist) {
		t.Errorf("engine count: fixture has %d, code has %d", len(fixture.Engines), len(adr26919Allowlist))
	}
	for engine, secrets := range fixture.Engines {
		codeSet, ok := adr26919Allowlist[engine]
		if !ok {
			t.Errorf("fixture has engine %q but code map does not", engine)
			continue
		}
		if len(secrets) != len(codeSet) {
			t.Errorf("engine %q: fixture has %d secrets, code has %d", engine, len(secrets), len(codeSet))
		}
		for _, s := range secrets {
			if !codeSet[s] {
				t.Errorf("engine %q: fixture lists %q but code map does not contain it", engine, s)
			}
		}
		// Reverse direction: every code entry must be in fixture.
		fixSet := map[string]bool{}
		for _, s := range secrets {
			fixSet[s] = true
		}
		for s := range codeSet {
			if !fixSet[s] {
				t.Errorf("engine %q: code lists %q but fixture does not", engine, s)
			}
		}
		// Fixture must be sorted (drift-detection contract).
		sorted := append([]string(nil), secrets...)
		sort.Strings(sorted)
		for i := range secrets {
			if secrets[i] != sorted[i] {
				t.Errorf("engine %q: fixture secrets not sorted; got %v want %v", engine, secrets, sorted)
				break
			}
		}
	}
}

// ---------------- US4: actionlint ----------------

func TestActionlintScanner_ErrorMapsToHigh(t *testing.T) {
	if _, err := exec.LookPath("actionlint"); err != nil {
		t.Skip("actionlint binary missing; FR-007 graceful degradation contract — test skipped")
	}
	dir := stageLockFixture(t, "compiled-with-actionlint-error.lock.yml")
	out := newActionlintScanner().Scan(context.Background(), dir)
	f := findRule(out, "actionlint:", true)
	if f == nil {
		t.Fatalf("expected at least one actionlint finding; got %v", out)
	}
	if f.Severity != SeverityHigh {
		t.Errorf("Severity = %v; want HIGH (errors); got %+v", f.Severity, out)
	}
	assertRepoRelativeActionlintFile(t, dir, f.File)
}

func TestActionlintScanner_WarningMapsToMedium(t *testing.T) {
	if _, err := exec.LookPath("actionlint"); err != nil {
		t.Skip("actionlint binary missing; FR-007 graceful degradation contract — test skipped")
	}
	dir := stageLockFixture(t, "compiled-with-actionlint-warning.lock.yml")
	out := newActionlintScanner().Scan(context.Background(), dir)
	if len(out) == 0 {
		t.Skip("actionlint produced no diagnostics on the warning fixture; binary version may not flag the construct")
	}
	f := findRule(out, "actionlint:", true)
	if f == nil {
		t.Fatalf("expected actionlint finding; got %v", out)
	}
	if f.Severity != SeverityMedium {
		t.Errorf("Severity = %v; want MEDIUM (warnings)", f.Severity)
	}
	assertRepoRelativeActionlintFile(t, dir, f.File)
}

func TestActionlintScanner_CleanLockFile(t *testing.T) {
	if _, err := exec.LookPath("actionlint"); err != nil {
		t.Skip("actionlint binary missing")
	}
	dir := stageLockFixture(t, "compiled-clean.lock.yml")
	out := newActionlintScanner().Scan(context.Background(), dir)
	if len(out) != 0 {
		t.Errorf("clean lock file should produce zero actionlint findings; got %d: %+v", len(out), out)
	}
}

func TestActionlintScanner_MissingBinary_PATHStripped(t *testing.T) {
	t.Setenv("PATH", "")
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755)
	out := newActionlintScanner().Scan(context.Background(), dir)
	if len(out) != 1 {
		t.Fatalf("expected exactly 1 INFO finding; got %d: %+v", len(out), out)
	}
	f := out[0]
	if f.RuleID != "actionlint:not-installed" {
		t.Errorf("RuleID = %q; want actionlint:not-installed", f.RuleID)
	}
	if f.Severity != SeverityInfo {
		t.Errorf("Severity = %v; want INFO", f.Severity)
	}
	if !strings.Contains(f.Message, "PATH") {
		t.Errorf("Message %q should mention PATH (SC-005)", f.Message)
	}
}

func stageLockFixture(t *testing.T, fixtureName string) string {
	t.Helper()
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".github", "workflows"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := filepath.Join(fixturesRoot, fixtureName)
	content, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture %s: %v", src, err)
	}
	dst := filepath.Join(tmp, ".github", "workflows", fixtureName)
	if err := os.WriteFile(dst, content, 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", dst, err)
	}
	return tmp
}

func assertRepoRelativeActionlintFile(t *testing.T, cloneDir, got string) {
	t.Helper()
	if filepath.IsAbs(got) {
		t.Fatalf("actionlint finding file is absolute: %q", got)
	}
	if strings.Contains(got, cloneDir) {
		t.Fatalf("actionlint finding file contains clone dir %q: %q", cloneDir, got)
	}
	if !strings.HasPrefix(got, ".github/workflows/") {
		t.Fatalf("actionlint finding file = %q; want .github/workflows/... path", got)
	}
}

func TestRunRegistersAllScanners(t *testing.T) {
	// Smoke test: ensure default Run wires gitleaks + structural + actionlint.
	scanners := defaultScanners()
	if len(scanners) != 3 {
		t.Fatalf("defaultScanners() len = %d; want 3", len(scanners))
	}
	if _, ok := scanners[0].(*gitleaksScanner); !ok {
		t.Errorf("scanner[0] type = %T; want *gitleaksScanner", scanners[0])
	}
	if _, ok := scanners[1].(*structuralScanner); !ok {
		t.Errorf("scanner[1] type = %T; want *structuralScanner", scanners[1])
	}
	if _, ok := scanners[2].(*actionlintScanner); !ok {
		t.Errorf("scanner[2] type = %T; want *actionlintScanner", scanners[2])
	}
}

func TestStructuralScanner_AggregateNoLowSeverity(t *testing.T) {
	// FR-015: v1 MUST NOT emit LOW. Aggregate over every fixture's
	// structural output and assert no LOW slipped in.
	entries, err := os.ReadDir(fixturesRoot)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	scanner := newStructuralScanner()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		dir := stageFixture(t, e.Name())
		out := scanner.Scan(context.Background(), dir)
		if c := countSeverity(out, SeverityLow); c > 0 {
			t.Errorf("FR-015 violated: %s produced %d LOW findings", e.Name(), c)
		}
	}
}
