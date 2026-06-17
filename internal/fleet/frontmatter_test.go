package fleet

import (
	"reflect"
	"testing"
)

func TestExtractWorkflowMeta_NestedSkipGuards(t *testing.T) {
	fm := map[string]any{
		"on": map[string]any{
			"schedule":          "daily",
			"skip-if-match":     `is:pr is:open in:title "[code-simplifier]"`,
			"skip-if-no-match":  []any{"*.md", "*.go"},
			"workflow_dispatch": nil,
		},
	}

	var tw TemplateWorkflow
	ExtractWorkflowMeta(fm, &tw)

	if want := []string{"schedule", "workflow_dispatch"}; !reflect.DeepEqual(tw.Triggers, want) {
		t.Errorf("Triggers = %v; want %v", tw.Triggers, want)
	}
	if want := []string{`is:pr is:open in:title "[code-simplifier]"`}; !reflect.DeepEqual(tw.SkipIfMatch, want) {
		t.Errorf("SkipIfMatch = %v; want %v", tw.SkipIfMatch, want)
	}
	if want := []string{"*.go", "*.md"}; !reflect.DeepEqual(tw.SkipIfNoMatch, want) {
		t.Errorf("SkipIfNoMatch = %v; want %v", tw.SkipIfNoMatch, want)
	}
}

func TestExtractWorkflowMeta_TopLevelSkipGuardFallback(t *testing.T) {
	fm := map[string]any{
		"on":            "push",
		"skip-if-match": []any{"skip-ci"},
	}

	var tw TemplateWorkflow
	ExtractWorkflowMeta(fm, &tw)

	if want := []string{"skip-ci"}; !reflect.DeepEqual(tw.SkipIfMatch, want) {
		t.Errorf("SkipIfMatch = %v; want %v", tw.SkipIfMatch, want)
	}
}
