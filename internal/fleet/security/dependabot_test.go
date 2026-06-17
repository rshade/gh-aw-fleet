package security

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rshade/gh-aw-fleet/internal/fleet/fleetdiag"
)

// dependabotFixtureRoot holds one subdirectory per scanner test case; each
// contains exactly one Dependabot config at <case>/.github/dependabot.yml so
// the probe order and first-match-wins behavior are exercised cleanly.
const dependabotFixtureRoot = "testdata/security/dependabot"

// dependabotConfigName is the canonical probe path (and the expected
// Finding.File) for every fixture case.
const dependabotConfigName = ".github/dependabot.yml"

func scanDependabotCase(t *testing.T, caseDir string) []Finding {
	t.Helper()
	return newDependabotScanner().Scan(context.Background(), filepath.Join(dependabotFixtureRoot, caseDir))
}

func writeDependabotFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// assertCanonicalRemedy verifies the conflict finding carries both the
// copy-pasteable ignore: block (the three gh-aw dependency-name entries,
// FR-010) and the name-only caveat (FR-004).
func assertCanonicalRemedy(t *testing.T, remedy string) {
	t.Helper()
	for _, want := range []string{
		`dependency-name: "github/gh-aw-actions"`,
		`dependency-name: "github/gh-aw-actions/setup"`,
		`dependency-name: "github/gh-aw/actions/setup-cli"`,
		"ignore by dependency NAME", // the name-only caveat (FR-004)
	} {
		if !strings.Contains(remedy, want) {
			t.Errorf("Remedy missing %q.\n got: %q", want, remedy)
		}
	}
}

// ---------------- US1: gh-aw-actions not ignored ----------------

func TestDependabotScanner_MissingIgnore(t *testing.T) {
	got := scanDependabotCase(t, "missing-ignore")
	if len(got) != 1 {
		t.Fatalf("findings = %d, want 1: %+v", len(got), got)
	}
	f := got[0]
	if f.RuleID != ruleIDDependabotGhAwActionsNotIgnored {
		t.Errorf("RuleID = %q, want %q", f.RuleID, ruleIDDependabotGhAwActionsNotIgnored)
	}
	if f.Severity != SeverityLow {
		t.Errorf("Severity = %v, want LOW", f.Severity)
	}
	if f.File != dependabotConfigName {
		t.Errorf("File = %q, want %q", f.File, dependabotConfigName)
	}
	assertCanonicalRemedy(t, f.Remedy)
}

func TestDependabotScanner_PartialUnrelatedIgnore(t *testing.T) {
	got := scanDependabotCase(t, "partial-unrelated-ignore")
	if len(got) != 1 {
		t.Fatalf("findings = %d, want 1: %+v", len(got), got)
	}
	f := got[0]
	if f.RuleID != ruleIDDependabotGhAwActionsNotIgnored {
		t.Errorf("RuleID = %q, want %q", f.RuleID, ruleIDDependabotGhAwActionsNotIgnored)
	}
	if f.Severity != SeverityLow {
		t.Errorf("Severity = %v, want LOW", f.Severity)
	}
}

func TestDependabotScanner_MultipleUnprotected(t *testing.T) {
	got := scanDependabotCase(t, "multiple-unprotected")
	if len(got) != 2 {
		t.Fatalf("findings = %d, want 2 (one per github-actions entry): %+v", len(got), got)
	}
	for _, f := range got {
		if f.RuleID != ruleIDDependabotGhAwActionsNotIgnored {
			t.Errorf("RuleID = %q, want %q", f.RuleID, ruleIDDependabotGhAwActionsNotIgnored)
		}
	}
	if got[0].Message == got[1].Message {
		t.Errorf("per-entry findings should have distinct Messages naming each directory; both = %q", got[0].Message)
	}
	joined := got[0].Message + "\n" + got[1].Message
	for _, dir := range []string{`"/"`, `"/tools"`} {
		if !strings.Contains(joined, dir) {
			t.Errorf("expected a finding Message naming directory %s; got:\n%s", dir, joined)
		}
	}
}

// TestDependabotScanner_MixedProtectedAndUnprotected covers scanner-contract C2
// row 7: a config with one protected entry and one unprotected entry yields
// exactly the one finding, exercising the loop's skip-protected-then-append path
// that TestDependabotScanner_MultipleUnprotected (both unprotected) does not.
func TestDependabotScanner_MixedProtectedAndUnprotected(t *testing.T) {
	got := scanDependabotCase(t, "mixed-protected")
	if len(got) != 1 {
		t.Fatalf("findings = %d, want 1 (only the unprotected entry): %+v", len(got), got)
	}
	f := got[0]
	if f.RuleID != ruleIDDependabotGhAwActionsNotIgnored {
		t.Errorf("RuleID = %q, want %q", f.RuleID, ruleIDDependabotGhAwActionsNotIgnored)
	}
	if !strings.Contains(f.Message, `"/tools"`) {
		t.Errorf("finding should name the unprotected directory %q; got Message = %q", "/tools", f.Message)
	}
}

func TestDependabotScanner_Correct(t *testing.T) {
	got := scanDependabotCase(t, "correct")
	if len(got) != 0 {
		t.Errorf("correct config should produce zero findings; got %d: %+v", len(got), got)
	}
}

func TestDependabotScanner_ScopedGhAwIgnoreStillWarns(t *testing.T) {
	got := scanDependabotCase(t, "scoped-ignore")
	if len(got) != 1 {
		t.Fatalf("findings = %d, want 1 (scoped ignore still permits some gh-aw updates): %+v", len(got), got)
	}
	f := got[0]
	if f.RuleID != ruleIDDependabotGhAwActionsNotIgnored {
		t.Errorf("RuleID = %q, want %q", f.RuleID, ruleIDDependabotGhAwActionsNotIgnored)
	}
	if f.Severity != SeverityLow {
		t.Errorf("Severity = %v, want LOW", f.Severity)
	}
}

// ---------------- US2: quiet & safe ----------------

func TestDependabotScanner_QuietAndSafe(t *testing.T) {
	cases := []struct {
		name      string
		caseDir   string
		wantLow   int
		wantInfo  int
		wantTotal int
	}{
		{name: "gomod only", caseDir: "gomod-only", wantTotal: 0},
		{name: "wildcard ignore", caseDir: "wildcard-ignore", wantTotal: 0},
		{name: "pr limit zero", caseDir: "pr-limit-zero", wantTotal: 0},
		{name: "malformed", caseDir: "malformed", wantInfo: 1, wantTotal: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scanDependabotCase(t, tc.caseDir)
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

func TestDependabotScanner_NoConfigIsSilent(t *testing.T) {
	got := newDependabotScanner().Scan(context.Background(), t.TempDir())
	if len(got) != 0 {
		t.Errorf("no config should produce zero findings; got %d: %+v", len(got), got)
	}
}

func TestDependabotScanner_MalformedIsInfoNotError(t *testing.T) {
	got := scanDependabotCase(t, "malformed")
	if len(got) != 1 {
		t.Fatalf("findings = %d, want exactly 1: %+v", len(got), got)
	}
	f := got[0]
	if f.RuleID != ruleIDDependabotParseError {
		t.Errorf("RuleID = %q, want %q", f.RuleID, ruleIDDependabotParseError)
	}
	if f.Severity != SeverityInfo {
		t.Errorf("Severity = %v, want INFO", f.Severity)
	}
	if f.File != dependabotConfigName {
		t.Errorf("File = %q, want %q", f.File, dependabotConfigName)
	}
}

func TestDependabotScanner_ProbeOrderFirstWins(t *testing.T) {
	dir := t.TempDir()
	// Deficient config at the highest-priority probe location (.yml)...
	writeDependabotFile(t, filepath.Join(dir, ".github", "dependabot.yml"), missingIgnoreConfig)
	// ...and a correct config at the lower-priority .yaml that must be ignored.
	writeDependabotFile(t, filepath.Join(dir, ".github", "dependabot.yaml"), correctConfig)

	got := newDependabotScanner().Scan(context.Background(), dir)
	if len(got) != 1 {
		t.Fatalf("findings = %d, want 1 (only .yml inspected): %+v", len(got), got)
	}
	if got[0].File != dependabotConfigName {
		t.Errorf("probe order violated: File = %q, want %q", got[0].File, dependabotConfigName)
	}
}

func TestDependabotScanner_ReadOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".github", "dependabot.yml")
	writeDependabotFile(t, path, missingIgnoreConfig)

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}
	_ = newDependabotScanner().Scan(context.Background(), dir)
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("FR-009 violated: Scan mutated the config file")
	}
}

func TestDependabotScanner_DiagCodeIsDependabotFamily(t *testing.T) {
	got := scanDependabotCase(t, "missing-ignore")
	if len(got) == 0 {
		t.Fatal("expected findings against missing-ignore")
	}
	for _, f := range got {
		if code := f.ToDiagnostic().Code; code != fleetdiag.DiagSecurityDependabot {
			t.Errorf("ToDiagnostic().Code = %q, want %q", code, fleetdiag.DiagSecurityDependabot)
		}
	}
}

// TestDependabotEntryLabel unit-tests the pure label-fallback function
// directly (per the project's unit-testing guidance for pure transformation
// functions): the classic directory wins, then the first directories value,
// then a placeholder when an entry omits both.
func TestDependabotEntryLabel(t *testing.T) {
	cases := []struct {
		name  string
		entry dependabotUpdate
		want  string
	}{
		{
			name:  "directory preferred over directories",
			entry: dependabotUpdate{Directory: "/", Directories: []string{"/ignored"}},
			want:  "/",
		},
		{
			name:  "directories fallback when directory empty",
			entry: dependabotUpdate{Directories: []string{"/tools", "/web"}},
			want:  "/tools",
		},
		{
			name:  "placeholder when neither set",
			entry: dependabotUpdate{},
			want:  "(unspecified directory)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dependabotEntryLabel(tc.entry); got != tc.want {
				t.Errorf("dependabotEntryLabel() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDependabotEntryProtectedCoverage(t *testing.T) {
	limitZero := 0
	cases := []struct {
		name  string
		entry dependabotUpdate
		want  bool
	}{
		{
			name: "canonical exact ignores",
			entry: dependabotUpdate{Ignore: []dependabotIgnore{
				{DependencyName: "github/gh-aw-actions"},
				{DependencyName: "github/gh-aw-actions/setup"},
				{DependencyName: "github/gh-aw/actions/setup-cli"},
			}},
			want: true,
		},
		{
			name:  "broad github wildcard",
			entry: dependabotUpdate{Ignore: []dependabotIgnore{{DependencyName: "github/*"}}},
			want:  true,
		},
		{
			name: "partial family wildcard",
			entry: dependabotUpdate{Ignore: []dependabotIgnore{
				{DependencyName: "github/gh-aw-actions*"},
			}},
			want: false,
		},
		{
			name: "version scoped wildcard",
			entry: dependabotUpdate{Ignore: []dependabotIgnore{
				{DependencyName: "github/*", Versions: []string{">=0.79.0"}},
			}},
			want: false,
		},
		{
			name: "update type scoped wildcard",
			entry: dependabotUpdate{Ignore: []dependabotIgnore{
				{DependencyName: "github/*", UpdateTypes: []string{"version-update:semver-major"}},
			}},
			want: false,
		},
		{
			name:  "open pr limit zero",
			entry: dependabotUpdate{OpenPullRequestsLimit: &limitZero},
			want:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dependabotEntryProtected(tc.entry); got != tc.want {
				t.Errorf("dependabotEntryProtected() = %v, want %v", got, tc.want)
			}
		})
	}
}

// missingIgnoreConfig / correctConfig are inline fixtures used by the
// probe-order and read-only tests that build clones in t.TempDir().
const missingIgnoreConfig = `version: 2
updates:
  - package-ecosystem: "github-actions"
    directory: "/"
    schedule:
      interval: "weekly"
`

const correctConfig = `version: 2
updates:
  - package-ecosystem: "github-actions"
    directory: "/"
    schedule:
      interval: "weekly"
    ignore:
      - dependency-name: "github/gh-aw-actions"
      - dependency-name: "github/gh-aw-actions/setup"
      - dependency-name: "github/gh-aw/actions/setup-cli"
`
