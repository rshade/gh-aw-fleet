package fleet

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func TestDiagnostic_JSONShape(t *testing.T) {
	d := Diagnostic{
		Code:    DiagMissingSecret,
		Message: "secret missing",
		Fields:  map[string]any{"secret": "FOO"},
	}
	out, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"code":"missing_secret","message":"secret missing","fields":{"secret":"FOO"}}`
	if string(out) != want {
		t.Errorf("marshal = %s; want %s", out, want)
	}
}

func TestDiagnostic_FieldsOmittedWhenEmpty(t *testing.T) {
	d := Diagnostic{Code: DiagHint, Message: "x"}
	out, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"code":"hint","message":"x"}`
	if string(out) != want {
		t.Errorf("marshal = %s; want %s (fields should be omitempty)", out, want)
	}
}

func TestHints_AllHaveSnakeCaseCode(t *testing.T) {
	codePat := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	for _, h := range hints {
		if h.Code == "" {
			t.Errorf("hint %q has empty Code", h.Pattern)
			continue
		}
		if !codePat.MatchString(h.Code) {
			t.Errorf("hint %q Code = %q; want snake_case matching %s", h.Pattern, h.Code, codePat)
		}
	}
}

func TestCollectHintDiagnostics_UnknownProperty(t *testing.T) {
	got := CollectHintDiagnostics("Unknown property: foo")
	if len(got) != 1 {
		t.Fatalf("len(got) = %d; want 1; got=%+v", len(got), got)
	}
	if got[0].Code != DiagUnknownProperty {
		t.Errorf("Code = %q; want %q", got[0].Code, DiagUnknownProperty)
	}
	if got[0].Fields["hint"] != got[0].Message {
		t.Errorf("Fields[hint] = %v; want = Message %q", got[0].Fields["hint"], got[0].Message)
	}
}

func TestCollectHintDiagnostics_BillingQuotaExceeded(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"http_402", "gh: HTTP 402: spending limit reached"},
		{"payment_required", "gh: 402 Payment Required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CollectHintDiagnostics(tc.in)
			if len(got) != 1 {
				t.Fatalf("len(got) = %d; want 1; got=%+v", len(got), got)
			}
			if got[0].Code != DiagBillingQuotaExceeded {
				t.Errorf("Code = %q; want %q", got[0].Code, DiagBillingQuotaExceeded)
			}
			if got[0].Fields["hint"] != got[0].Message {
				t.Errorf("Fields[hint] = %v; want = Message %q", got[0].Fields["hint"], got[0].Message)
			}
			if !strings.Contains(got[0].Message, "Copilot") {
				t.Errorf("Message = %q; want GitHub-specific remediation naming Copilot", got[0].Message)
			}
			if !strings.Contains(got[0].Message, "github.com/settings/billing/spending_limit") {
				t.Errorf("Message = %q; want pointer to GitHub spending controls URL", got[0].Message)
			}
			if !strings.Contains(got[0].Message, "gh-aw-fleet consumption") {
				t.Errorf("Message = %q; want forward-reference to `gh-aw-fleet consumption` subcommand", got[0].Message)
			}
		})
	}
}

func TestCollectHintDiagnostics_EmptyInputReturnsNonNilEmptySlice(t *testing.T) {
	got := CollectHintDiagnostics("")
	if got == nil {
		t.Fatal("got = nil; want non-nil empty slice (FR-006: JSON arrays never null)")
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d; want 0", len(got))
	}
}

func TestCollectHintDiagnostics_DedupesWithinAndAcrossInputs(t *testing.T) {
	got := CollectHintDiagnostics(
		"Unknown property: foo and Unknown property: bar",
		"another Unknown property: baz",
	)
	if len(got) != 1 {
		t.Errorf("len(got) = %d; want 1 (deduped); got=%+v", len(got), got)
	}
}

func TestCollectHints_CompileStrictPatterns(t *testing.T) {
	cases := []struct {
		name          string
		input         string
		wantCode      string
		wantSubstring string
	}{
		{
			name:          "strict_mode_validation",
			input:         "✗ strict mode validation failed for workflow foo.md",
			wantCode:      DiagCompileStrictFailed,
			wantSubstring: "\"compile_strict\": false",
		},
		{
			name:          "strict_mode_requires",
			input:         "error: strict mode requires explicit triggers",
			wantCode:      DiagCompileStrictFailed,
			wantSubstring: "\"compile_strict\": false",
		},
		{
			name:          "unknown_flag_strict",
			input:         "Error: unknown flag: --strict",
			wantCode:      DiagGhAwTooOld,
			wantSubstring: "v0.79.2",
		},
		{
			name:          "unknown_long_flag_strict",
			input:         "pflag: unknown long flag '--strict'",
			wantCode:      DiagGhAwTooOld,
			wantSubstring: "gh extension upgrade aw",
		},
		{
			name:          "probe_abort_too_old_wrapped_error",
			input:         "gh aw is too old: v0.50.0 detected, minimum v0.68.3 required: <hint>",
			wantCode:      DiagGhAwTooOld,
			wantSubstring: "gh extension upgrade aw",
		},
		{
			name:          "executable_file_not_found",
			input:         "exec: \"gh\": executable file not found in $PATH",
			wantCode:      DiagGhAwMissing,
			wantSubstring: "gh extension install github/gh-aw",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CollectHintDiagnostics(tc.input)
			if len(got) != 1 {
				t.Fatalf("len(got) = %d; want 1; got=%+v", len(got), got)
			}
			if got[0].Code != tc.wantCode {
				t.Errorf("Code = %q; want %q", got[0].Code, tc.wantCode)
			}
			if !strings.Contains(got[0].Message, tc.wantSubstring) {
				t.Errorf("Message = %q; want substring %q", got[0].Message, tc.wantSubstring)
			}
		})
	}
}

func TestCollectHints_CompileStrictPatterns_NoFalsePositives(t *testing.T) {
	unrelated := []string{
		"git push: rejected (non-fast-forward)",
		"some random output without any matching tokens",
		"strict transport security policy update", // contains "strict" but not the trigger phrase
	}
	for _, in := range unrelated {
		got := CollectHintDiagnostics(in)
		for _, d := range got {
			switch d.Code {
			case DiagCompileStrictFailed, DiagGhAwTooOld, DiagGhAwMissing:
				t.Errorf("input %q produced unexpected diagnostic %s: %q", in, d.Code, d.Message)
			}
		}
	}
}
