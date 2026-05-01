package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
	logpkg "github.com/rshade/gh-aw-fleet/internal/log"
	"github.com/rshade/gh-aw-fleet/internal/testutil"
)

const (
	settingsURLAlice  = "https://github.com/alice/widgets/settings/actions"
	consequenceClause = "workflows that push commits or create reviews will fail"
)

func TestEmitDeployWarnings_HealthyEmitsNothing(t *testing.T) {
	buf := testutil.CaptureStderr(t, func() {
		if err := logpkg.Configure("info", "json"); err != nil {
			t.Fatal(err)
		}
		emitDeployWarnings(&fleet.DeployResult{
			Repo: "alice/widgets",
		})
	})
	if strings.TrimSpace(buf) != "" {
		t.Errorf("expected zero stderr output for healthy DeployResult; got %q", buf)
	}
}

func TestEmitDeployWarningsActionsDisabled(t *testing.T) {
	buf := testutil.CaptureStderr(t, func() {
		if err := logpkg.Configure("info", "json"); err != nil {
			t.Fatal(err)
		}
		emitDeployWarnings(&fleet.DeployResult{
			Repo:            "alice/widgets",
			ActionsDisabled: true,
		})
	})
	line := strings.TrimSpace(buf)
	if line == "" {
		t.Fatal("no output emitted for ActionsDisabled=true")
	}
	for _, want := range []string{"alice/widgets", settingsURLAlice} {
		if !strings.Contains(line, want) {
			t.Errorf("warning missing %q\nfull: %s", want, line)
		}
	}
}

func TestEmitDeployWarningsTokenReadOnly(t *testing.T) {
	buf := testutil.CaptureStderr(t, func() {
		if err := logpkg.Configure("info", "json"); err != nil {
			t.Fatal(err)
		}
		emitDeployWarnings(&fleet.DeployResult{
			Repo:                  "alice/widgets",
			WorkflowTokenReadOnly: true,
		})
	})
	line := strings.TrimSpace(buf)
	if line == "" {
		t.Fatal("no output emitted for WorkflowTokenReadOnly=true")
	}
	for _, want := range []string{
		"alice/widgets",
		consequenceClause,
		"Workflow permissions",
		"Read and write permissions",
		settingsURLAlice,
	} {
		if !strings.Contains(line, want) {
			t.Errorf("warning missing %q\nfull: %s", want, line)
		}
	}
}

func TestEmitDeployWarningsBothFindings(t *testing.T) {
	buf := testutil.CaptureStderr(t, func() {
		if err := logpkg.Configure("info", "json"); err != nil {
			t.Fatal(err)
		}
		emitDeployWarnings(&fleet.DeployResult{
			Repo:                  "alice/widgets",
			ActionsDisabled:       true,
			WorkflowTokenReadOnly: true,
			MissingSecret:         "ANTHROPIC_API_KEY",
			SecretKeyURL:          "https://console.anthropic.com/settings/keys",
		})
	})
	out := strings.TrimSpace(buf)
	actionsIdx := strings.Index(out, "GitHub Actions is disabled")
	tokenIdx := strings.Index(out, "GITHUB_TOKEN is read-only")
	secretIdx := strings.Index(out, "ANTHROPIC_API_KEY")
	if actionsIdx < 0 || tokenIdx < 0 || secretIdx < 0 {
		t.Fatalf("missing warnings; actions=%d token=%d secret=%d\nfull:\n%s", actionsIdx, tokenIdx, secretIdx, out)
	}
	if !(actionsIdx < tokenIdx && tokenIdx < secretIdx) {
		t.Errorf("expected fixed order Actions < token < secret; got %d, %d, %d\nfull:\n%s",
			actionsIdx, tokenIdx, secretIdx, out)
	}
}

// captureEnvelope runs emitDeployEnvelope against a synthetic cobra command
// whose stdout is redirected to a buffer, returning the parsed envelope.
// emitDeployEnvelope dual-emits to stderr — that side-channel is discarded
// here because the JSON tests only assert on stdout.
func captureEnvelope(t *testing.T, res *fleet.DeployResult) map[string]any {
	t.Helper()
	var stdout bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	_ = testutil.CaptureStderr(t, func() {
		if err := emitDeployEnvelope(cmd, res.Repo, false, res, nil); err != nil {
			t.Fatalf("emitDeployEnvelope: %v", err)
		}
	})
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("envelope not JSON: %v\nraw=%s", err, stdout.String())
	}
	return env
}

func TestEmitDeployEnvelopeActionsDisabled(t *testing.T) {
	env := captureEnvelope(t, &fleet.DeployResult{
		Repo:            "alice/widgets",
		ActionsDisabled: true,
	})
	warnings, _ := env["warnings"].([]any)
	if len(warnings) == 0 {
		t.Fatalf("envelope warnings empty; got %v", env)
	}
	first, _ := warnings[0].(map[string]any)
	if first["code"] != fleet.DiagActionsDisabled {
		t.Errorf("warnings[0].code = %v, want %q", first["code"], fleet.DiagActionsDisabled)
	}
	fields, _ := first["fields"].(map[string]any)
	if fields["url"] != settingsURLAlice {
		t.Errorf("warnings[0].fields.url = %v, want %q", fields["url"], settingsURLAlice)
	}
}

func TestEmitDeployEnvelopeTokenReadOnly(t *testing.T) {
	env := captureEnvelope(t, &fleet.DeployResult{
		Repo:                  "alice/widgets",
		WorkflowTokenReadOnly: true,
	})
	warnings, _ := env["warnings"].([]any)
	if len(warnings) == 0 {
		t.Fatalf("envelope warnings empty; got %v", env)
	}
	first, _ := warnings[0].(map[string]any)
	if first["code"] != fleet.DiagWorkflowTokenReadOnly {
		t.Errorf("warnings[0].code = %v, want %q", first["code"], fleet.DiagWorkflowTokenReadOnly)
	}
	message, _ := first["message"].(string)
	for _, want := range []string{
		consequenceClause,
		"Workflow permissions",
		"Read and write permissions",
		settingsURLAlice,
	} {
		if !strings.Contains(message, want) {
			t.Errorf("warnings[0].message missing %q\nfull: %s", want, message)
		}
	}
}

func TestEmitDeployEnvelopeFixedOrder(t *testing.T) {
	env := captureEnvelope(t, &fleet.DeployResult{
		Repo:                  "alice/widgets",
		ActionsDisabled:       true,
		WorkflowTokenReadOnly: true,
		MissingSecret:         "ANTHROPIC_API_KEY",
		SecretKeyURL:          "https://console.anthropic.com/settings/keys",
	})
	warnings, _ := env["warnings"].([]any)
	if len(warnings) != 3 {
		t.Fatalf("expected 3 warnings; got %d (%v)", len(warnings), warnings)
	}
	codes := make([]string, len(warnings))
	for i, w := range warnings {
		m, _ := w.(map[string]any)
		codes[i], _ = m["code"].(string)
	}
	want := []string{
		fleet.DiagActionsDisabled,
		fleet.DiagWorkflowTokenReadOnly,
		fleet.DiagMissingSecret,
	}
	for i, c := range want {
		if codes[i] != c {
			t.Errorf("warnings[%d].code = %q, want %q (full order: %v)", i, codes[i], c, codes)
		}
	}
}
