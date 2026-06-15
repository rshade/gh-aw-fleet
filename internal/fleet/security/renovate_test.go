package security

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rshade/gh-aw-fleet/internal/fleet/fleetdiag"
)

// renovateFixtureRoot holds one subdirectory per scanner test case; each
// contains exactly one Renovate config file so the probe order and
// first-match-wins behavior are exercised cleanly.
const renovateFixtureRoot = "testdata/security/renovate"

// wantRuleARemedy / wantRuleBRemedy are independent copies of the canonical
// remediation blocks (research.md Decision 6). They are duplicated here on
// purpose: a drift between the scanner's emitted Remedy and the canonical
// text (contract C5 — byte-for-byte) must fail the test, not silently pass
// by comparing the scanner constant to itself.
const wantRuleARemedy = `{
  "matchPackageNames": [
    "github/gh-aw-actions",
    "github/gh-aw-actions/setup",
    "github/gh-aw/actions/setup-cli"
  ],
  "enabled": false,
  "description": "gh-aw-actions version is coupled to the gh aw compiler version baked into lock file metadata. Renovate bumping it directly breaks hash validation. Managed atomically by the fleet tool via: gh aw upgrade"
}`

const wantRuleBRemedy = `{
  "description": "gh-aw generates .github/workflows/*.lock.yml via 'gh aw compile'. Their pinned action SHAs and container tags are managed by gh-aw (recompiling bumps them), so Renovate must not touch them: edits would be reverted on the next compile and can break the gh-aw integrity manifest.",
  "matchFileNames": [
    ".github/workflows/*.lock.yml"
  ],
  "enabled": false
}`

func scanRenovateCase(t *testing.T, caseDir string) []Finding {
	t.Helper()
	return newRenovateScanner().Scan(context.Background(), filepath.Join(renovateFixtureRoot, caseDir))
}

func writeRenovateFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// ---------------- US1: Rule A (gh-aw-actions disable) ----------------

func TestRenovateScanner_RuleAMissing(t *testing.T) {
	got := scanRenovateCase(t, "missing-gh-aw-actions")
	if len(got) != 1 {
		t.Fatalf("findings = %d, want 1: %+v", len(got), got)
	}
	f := got[0]
	if f.RuleID != ruleIDRenovateGhAwActionsNotDisabled {
		t.Errorf("RuleID = %q, want %q", f.RuleID, ruleIDRenovateGhAwActionsNotDisabled)
	}
	if f.Severity != SeverityLow {
		t.Errorf("Severity = %v, want LOW", f.Severity)
	}
	if f.File != renovateDefaultConfigName {
		t.Errorf("File = %q, want renovate.json", f.File)
	}
	if f.Remedy != wantRuleARemedy {
		t.Errorf("Remedy not byte-for-byte canonical Rule A block.\n got: %q\nwant: %q", f.Remedy, wantRuleARemedy)
	}
}

func TestRenovateScanner_RuleAPresentInCorrect(t *testing.T) {
	got := scanRenovateCase(t, "correct")
	if f := findRule(got, ruleIDRenovateGhAwActionsNotDisabled, false); f != nil {
		t.Errorf("unexpected Rule A finding against correct config: %+v", f)
	}
}

// ---------------- US2: Rule B (lock-file exclusion) ----------------

func TestRenovateScanner_RuleBMissing(t *testing.T) {
	got := scanRenovateCase(t, "missing-lockfile")
	if len(got) != 1 {
		t.Fatalf("findings = %d, want 1: %+v", len(got), got)
	}
	f := got[0]
	if f.RuleID != ruleIDRenovateLockfileNotDisabled {
		t.Errorf("RuleID = %q, want %q", f.RuleID, ruleIDRenovateLockfileNotDisabled)
	}
	if f.Severity != SeverityLow {
		t.Errorf("Severity = %v, want LOW", f.Severity)
	}
	if f.Remedy != wantRuleBRemedy {
		t.Errorf("Remedy not byte-for-byte canonical Rule B block.\n got: %q\nwant: %q", f.Remedy, wantRuleBRemedy)
	}
}

func TestRenovateScanner_RuleBPresentInCorrect(t *testing.T) {
	got := scanRenovateCase(t, "correct")
	if f := findRule(got, ruleIDRenovateLockfileNotDisabled, false); f != nil {
		t.Errorf("unexpected Rule B finding against correct config: %+v", f)
	}
}

// ---------------- US3: quiet & safe ----------------

func TestRenovateScanner_QuietAndSafe(t *testing.T) {
	cases := []struct {
		name      string
		caseDir   string
		wantLow   int
		wantInfo  int
		wantTotal int
	}{
		{name: "both rules present", caseDir: "correct", wantTotal: 0},
		{name: "both missing", caseDir: "missing-both", wantLow: 2, wantTotal: 2},
		{name: "equivalent forms", caseDir: "equivalent-forms", wantTotal: 0},
		{name: "JWCC comments", caseDir: "comments", wantTotal: 0},
		{name: "root disabled", caseDir: "disabled", wantTotal: 0},
		{name: "malformed", caseDir: "malformed", wantInfo: 1, wantTotal: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scanRenovateCase(t, tc.caseDir)
			if len(got) != tc.wantTotal {
				t.Fatalf("findings = %d, want %d: %+v", len(got), tc.wantTotal, got)
			}
			if n := countSeverity(got, SeverityLow); n != tc.wantLow {
				t.Errorf("LOW findings = %d, want %d", n, tc.wantLow)
			}
			if n := countSeverity(got, SeverityInfo); n != tc.wantInfo {
				t.Errorf("INFO findings = %d, want %d", n, tc.wantInfo)
			}
		})
	}
}

func TestRenovateScanner_NoConfigIsSilent(t *testing.T) {
	got := newRenovateScanner().Scan(context.Background(), t.TempDir())
	if len(got) != 0 {
		t.Errorf("no config should produce zero findings; got %d: %+v", len(got), got)
	}
}

func TestRenovateScanner_MalformedIsInfoNotError(t *testing.T) {
	got := scanRenovateCase(t, "malformed")
	if len(got) != 1 {
		t.Fatalf("findings = %d, want exactly 1: %+v", len(got), got)
	}
	f := got[0]
	if f.RuleID != ruleIDRenovateParseError {
		t.Errorf("RuleID = %q, want %q", f.RuleID, ruleIDRenovateParseError)
	}
	if f.Severity != SeverityInfo {
		t.Errorf("Severity = %v, want INFO", f.Severity)
	}
	if f.File != renovateDefaultConfigName {
		t.Errorf("File = %q, want renovate.json", f.File)
	}
}

func TestRenovateScanner_ProbeOrderFirstWins(t *testing.T) {
	dir := t.TempDir()
	// Deficient config at the highest-priority probe location...
	writeRenovateFile(t, filepath.Join(dir, renovateDefaultConfigName), `{"packageRules":[]}`)
	// ...and a correct config at a lower-priority location that must be ignored.
	writeRenovateFile(t, filepath.Join(dir, ".github", renovateDefaultConfigName), wantCorrectConfig)

	got := newRenovateScanner().Scan(context.Background(), dir)
	if len(got) != 2 {
		t.Fatalf("findings = %d, want 2 (only renovate.json inspected): %+v", len(got), got)
	}
	for _, f := range got {
		if f.File != renovateDefaultConfigName {
			t.Errorf("probe order violated: File = %q, want renovate.json", f.File)
		}
	}
}

func TestRenovateScanner_AlternateLocationsHonored(t *testing.T) {
	cases := []struct{ name, rel string }{
		{"dotfile renovaterc", ".renovaterc"},
		{"github json5", ".github/renovate.json5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeRenovateFile(t, filepath.Join(dir, filepath.FromSlash(tc.rel)), `{"packageRules":[]}`)
			got := newRenovateScanner().Scan(context.Background(), dir)
			if len(got) != 2 {
				t.Fatalf("findings = %d, want 2 for sole config at %s: %+v", len(got), tc.rel, got)
			}
			for _, f := range got {
				if f.File != tc.rel {
					t.Errorf("File = %q, want %q", f.File, tc.rel)
				}
			}
		})
	}
}

func TestRenovateScanner_ReadOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, renovateDefaultConfigName)
	writeRenovateFile(t, path, wantCorrectConfig)

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}
	_ = newRenovateScanner().Scan(context.Background(), dir)
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("FR-009 violated: Scan mutated the config file")
	}
}

func TestRenovateScanner_DiagCodeIsRenovateFamily(t *testing.T) {
	got := scanRenovateCase(t, "missing-both")
	if len(got) == 0 {
		t.Fatal("expected findings against missing-both")
	}
	for _, f := range got {
		if code := f.ToDiagnostic().Code; code != fleetdiag.DiagSecurityRenovate {
			t.Errorf("ToDiagnostic().Code = %q, want %q", code, fleetdiag.DiagSecurityRenovate)
		}
	}
}

// wantCorrectConfig is an inline config satisfying both rules, used by the
// probe-order and read-only tests that build clones in t.TempDir().
const wantCorrectConfig = `{
  "packageRules": [
    {"matchPackageNames": ["github/gh-aw-actions"], "enabled": false},
    {"matchFileNames": [".github/workflows/*.lock.yml"], "enabled": false}
  ]
}`
