package fleet

import (
	"encoding/json"
	"regexp"
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
