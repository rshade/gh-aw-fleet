package fleet

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rshade/gh-aw-fleet/internal/fleet/security"
)

func TestEvaluateStrictSecurityGateDefaultAndNonBlocking(t *testing.T) {
	high := testSecurityFinding("fleet.high", security.SeverityHigh)
	lower := []security.Finding{
		testSecurityFinding("fleet.info", security.SeverityInfo),
		testSecurityFinding("fleet.low", security.SeverityLow),
		testSecurityFinding("fleet.medium", security.SeverityMedium),
	}

	tests := []struct {
		name     string
		opts     SecurityOpts
		findings []security.Finding
	}{
		{name: "default opts are advisory", findings: []security.Finding{high}},
		{name: "strict clean proceeds", opts: SecurityOpts{Strict: true}, findings: []security.Finding{}},
		{name: "strict lower severities proceed", opts: SecurityOpts{Strict: true}, findings: lower},
		{
			name:     "strict prompt injection high proceeds",
			opts:     SecurityOpts{Strict: true},
			findings: []security.Finding{testSecurityFinding("promptinj:indirect", security.SeverityHigh)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EvaluateStrictSecurityGate("acme/widgets", t.TempDir(), tt.opts, tt.findings)
			if err != nil {
				t.Fatalf("EvaluateStrictSecurityGate returned error: %v", err)
			}
		})
	}
}

func TestEvaluateStrictSecurityGateBlocksAndWritesBreadcrumb(t *testing.T) {
	dir := t.TempDir()
	findings := []security.Finding{
		testSecurityFinding("fleet.permissions.write-on-schedule", security.SeverityHigh),
		testSecurityFinding("fleet.safe-outputs.draft-false", security.SeverityMedium),
	}

	err := EvaluateStrictSecurityGate("acme/widgets", dir, SecurityOpts{Strict: true}, findings)
	var strictErr *StrictSecurityError
	if !errors.As(err, &strictErr) {
		t.Fatalf("error = %T %[1]v; want *StrictSecurityError", err)
	}
	if strictErr.BlockingCount != 1 {
		t.Fatalf("BlockingCount = %d; want 1", strictErr.BlockingCount)
	}
	if strictErr.Repo != "acme/widgets" {
		t.Errorf("Repo = %q; want acme/widgets", strictErr.Repo)
	}
	if strictErr.BreadcrumbPath != filepath.Join(dir, "findings.json") {
		t.Errorf("BreadcrumbPath = %q; want findings.json under clone", strictErr.BreadcrumbPath)
	}
	for _, want := range []string{"strict security gate", "1 HIGH", "acme/widgets", "without --strict", strictErr.BreadcrumbPath} {
		if !strings.Contains(strictErr.Error(), want) {
			t.Errorf("Error() missing %q: %s", want, strictErr.Error())
		}
	}

	data, readErr := os.ReadFile(strictErr.BreadcrumbPath)
	if readErr != nil {
		t.Fatalf("read breadcrumb: %v", readErr)
	}
	var decoded []security.Finding
	if decodeErr := json.Unmarshal(data, &decoded); decodeErr != nil {
		t.Fatalf("decode breadcrumb: %v", decodeErr)
	}
	if !reflect.DeepEqual(decoded, findings) {
		t.Fatalf("decoded findings = %#v; want %#v", decoded, findings)
	}

	var raw []map[string]any
	if decodeErr := json.Unmarshal(data, &raw); decodeErr != nil {
		t.Fatalf("decode raw breadcrumb: %v", decodeErr)
	}
	first := raw[0]
	for _, key := range []string{"rule_id", "severity", "file", "line", "message", "remedy"} {
		if _, ok := first[key]; !ok {
			t.Errorf("breadcrumb finding missing JSON field %q: %#v", key, first)
		}
	}
	if first["severity"] != float64(security.SeverityHigh) {
		t.Errorf("severity = %v; want numeric %d", first["severity"], security.SeverityHigh)
	}
}

func TestEvaluateStrictSecurityGatePreservesFindingOrderAndContent(t *testing.T) {
	findings := []security.Finding{
		testSecurityFinding("fleet.z", security.SeverityHigh),
		testSecurityFinding("fleet.a", security.SeverityLow),
		testSecurityFinding("fleet.m", security.SeverityHigh),
	}
	before := append([]security.Finding(nil), findings...)

	err := EvaluateStrictSecurityGate("acme/widgets", t.TempDir(), SecurityOpts{Strict: true}, findings)
	if err == nil {
		t.Fatal("EvaluateStrictSecurityGate returned nil; want strict error")
	}
	if !reflect.DeepEqual(findings, before) {
		t.Fatalf("findings mutated:\ngot  %#v\nwant %#v", findings, before)
	}
}

func TestEvaluateStrictSecurityGateReportsBreadcrumbWriteFailure(t *testing.T) {
	cloneFile := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(cloneFile, []byte("file\n"), 0o644); err != nil {
		t.Fatalf("write cloneFile: %v", err)
	}

	err := EvaluateStrictSecurityGate(
		"acme/widgets",
		cloneFile,
		SecurityOpts{Strict: true},
		[]security.Finding{testSecurityFinding("fleet.high", security.SeverityHigh)},
	)
	var strictErr *StrictSecurityError
	if !errors.As(err, &strictErr) {
		t.Fatalf("error = %T %[1]v; want *StrictSecurityError", err)
	}
	if strictErr.BlockingCount != 1 {
		t.Fatalf("BlockingCount = %d; want 1", strictErr.BlockingCount)
	}
	if errors.Unwrap(strictErr) == nil {
		t.Fatal("Unwrap() = nil; want breadcrumb write error")
	}
	if !strings.Contains(strictErr.Error(), "failed to save findings") {
		t.Fatalf("Error() = %q; want breadcrumb write failure", strictErr.Error())
	}
}

func TestStrictGateKeepsActionlintAdvisoryAndBlocksGitleaks(t *testing.T) {
	actionlint := testSecurityFinding("actionlint:syntax-check", security.SeverityMedium)
	err := EvaluateStrictSecurityGate(
		"acme/widgets",
		t.TempDir(),
		SecurityOpts{Strict: true},
		[]security.Finding{actionlint},
	)
	if err != nil {
		t.Fatalf("actionlint MEDIUM blocked --strict: %v", err)
	}

	gitleaks := testSecurityFinding("gitleaks:generic-api-key", security.SeverityHigh)
	err = EvaluateStrictSecurityGate(
		"acme/widgets",
		t.TempDir(),
		SecurityOpts{Strict: true},
		[]security.Finding{actionlint, gitleaks},
	)
	var strictErr *StrictSecurityError
	if !errors.As(err, &strictErr) {
		t.Fatalf("error = %T %[1]v; want *StrictSecurityError", err)
	}
	if strictErr.BlockingCount != 1 {
		t.Fatalf("BlockingCount = %d; want 1 (gitleaks only)", strictErr.BlockingCount)
	}
}

func TestBlockingSecurityFindingsPromptInjectionCarveOut(t *testing.T) {
	prompt := testSecurityFinding("promptinj:indirect", security.SeverityHigh)
	layer1 := testSecurityFinding("fleet.permissions.write-on-schedule", security.SeverityHigh)

	if got := BlockingSecurityFindings([]security.Finding{prompt}, nil); len(got) != 0 {
		t.Fatalf("promptinj-only blockers = %#v; want empty", got)
	}
	got := BlockingSecurityFindings([]security.Finding{prompt, layer1}, nil)
	if len(got) != 1 {
		t.Fatalf("mixed blockers = %#v; want one Layer 1 blocker", got)
	}
	if got[0].RuleID != layer1.RuleID {
		t.Fatalf("blocker RuleID = %q; want %q", got[0].RuleID, layer1.RuleID)
	}
}

func TestStrictGateSuppressions(t *testing.T) {
	lock := ".github/workflows/ci-doctor.lock.yml"
	actionlint := security.Finding{
		RuleID:   "actionlint:syntax-check",
		Severity: security.SeverityHigh,
		File:     lock,
		Line:     12,
		Message:  "unexpected key queue",
		Remedy:   "Leave the generated lock file alone.",
	}
	gitleaks := testSecurityFinding("gitleaks:generic-api-key", security.SeverityHigh)
	rules := []SuppressionRule{{
		Rule:   "actionlint:*",
		File:   ".github/workflows/*.lock.yml",
		Reason: "compiler output",
	}}

	tests := []struct {
		name       string
		findings   []security.Finding
		rules      []SuppressionRule
		wantBlock  int
		wantRuleID string
	}{
		{
			name:      "matching rule and file stay advisory",
			findings:  []security.Finding{actionlint},
			rules:     rules,
			wantBlock: 0,
		},
		{
			name:       "unmatched rule still blocks",
			findings:   []security.Finding{actionlint, gitleaks},
			rules:      rules,
			wantBlock:  1,
			wantRuleID: gitleaks.RuleID,
		},
		{
			name: "file mismatch still blocks",
			findings: []security.Finding{{
				RuleID:   "actionlint:syntax-check",
				Severity: security.SeverityHigh,
				File:     ".github/workflows/ci.md",
				Message:  "authored file",
				Remedy:   "Fix the markdown.",
			}},
			rules:      rules,
			wantBlock:  1,
			wantRuleID: "actionlint:syntax-check",
		},
		{
			name:     "empty rule is a wildcard",
			findings: []security.Finding{gitleaks},
			rules: []SuppressionRule{{
				File: ".github/workflows/test.md",
			}},
			wantBlock: 0,
		},
		{
			name:      "empty file is a wildcard",
			findings:  []security.Finding{gitleaks},
			rules:     []SuppressionRule{{Rule: "gitleaks:*"}},
			wantBlock: 0,
		},
		{
			name:      "no rules block the high finding",
			findings:  []security.Finding{actionlint},
			wantBlock: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := append([]security.Finding(nil), tt.findings...)
			err := EvaluateStrictSecurityGate(
				"acme/widgets",
				t.TempDir(),
				SecurityOpts{Strict: true, Suppressions: tt.rules},
				tt.findings,
			)
			if !reflect.DeepEqual(tt.findings, before) {
				t.Fatalf("findings mutated:\ngot  %#v\nwant %#v", tt.findings, before)
			}
			if tt.wantBlock == 0 {
				if err != nil {
					t.Fatalf("gate error = %v; want nil", err)
				}
				return
			}
			var strictErr *StrictSecurityError
			if !errors.As(err, &strictErr) {
				t.Fatalf("error = %T %[1]v; want *StrictSecurityError", err)
			}
			if strictErr.BlockingCount != tt.wantBlock {
				t.Fatalf("BlockingCount = %d; want %d", strictErr.BlockingCount, tt.wantBlock)
			}
			if tt.wantRuleID != "" && strictErr.BlockingFindings[0].RuleID != tt.wantRuleID {
				t.Fatalf("blocker = %q; want %q", strictErr.BlockingFindings[0].RuleID, tt.wantRuleID)
			}
		})
	}
}

func testSecurityFinding(ruleID string, severity security.Severity) security.Finding {
	return security.Finding{
		RuleID:   ruleID,
		Severity: severity,
		File:     ".github/workflows/test.md",
		Line:     7,
		Message:  "synthetic security finding",
		Remedy:   "Fix the synthetic finding.",
	}
}
