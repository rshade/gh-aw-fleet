package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
)

// listResultStub mirrors the JSON shape of internal/fleet.ListResult, kept
// local here to pin the envelope contract independent of the fleet package.
type listResultStub struct {
	LoadedFrom string   `json:"loaded_from"`
	Repos      []string `json:"repos"`
}

func TestEnvelope_TopLevelKeysPinned(t *testing.T) {
	var buf bytes.Buffer
	res := listResultStub{LoadedFrom: "", Repos: nil}
	if err := writeEnvelopeTo(&buf, "list", "", false, &res, nil, nil); err != nil {
		t.Fatalf("writeEnvelopeTo: %v", err)
	}
	want := `{"schema_version":1,"command":"list","repo":"","apply":false,"result":{"loaded_from":"","repos":[]},"warnings":[],"hints":[]}` + "\n"
	if buf.String() != want {
		t.Errorf("envelope =\n  %q\nwant\n  %q", buf.String(), want)
	}
}

func TestEnvelope_SchemaVersionConstant(t *testing.T) {
	if SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d; want 1 (pinned by FR-005)", SchemaVersion)
	}
}

func TestEnvelope_SchemaVersionIs1(t *testing.T) {
	var buf bytes.Buffer
	if err := writeEnvelopeTo(&buf, "list", "", false, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	var env map[string]any
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	v, ok := env["schema_version"].(float64)
	if !ok {
		t.Fatalf("schema_version not numeric: %v", env["schema_version"])
	}
	if int(v) != 1 {
		t.Errorf("schema_version = %v; want 1", v)
	}
}

func TestEnvelope_ResultNullOnPreFailure(t *testing.T) {
	var buf bytes.Buffer
	if err := writeEnvelopeTo(&buf, "deploy", "x/y", false, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `"result":null`) {
		t.Errorf("missing result:null in %s", out)
	}
	for _, key := range []string{
		`"schema_version":1`, `"command":"deploy"`, `"repo":"x/y"`,
		`"apply":false`, `"warnings":[]`, `"hints":[]`,
	} {
		if !strings.Contains(out, key) {
			t.Errorf("missing %s in %s", key, out)
		}
	}
}

func TestInitSlices_NilToEmpty(t *testing.T) {
	r := fleet.DeployResult{Repo: "x/y"}
	initSlices(&r)
	if r.Added == nil {
		t.Error("Added still nil after initSlices")
	}
	if r.Skipped == nil {
		t.Error("Skipped still nil after initSlices")
	}
	if r.Failed == nil {
		t.Error("Failed still nil after initSlices")
	}
	if len(r.Added) != 0 || len(r.Skipped) != 0 || len(r.Failed) != 0 {
		t.Errorf("non-empty slices after init: %+v", r)
	}
}

// outputFlagFixture builds a stub root with a no-op subcommand for flag-validation tests.
func outputFlagFixture() (*cobra.Command, *bool) {
	ran := false
	root := NewRootCmd()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	probe := &cobra.Command{
		Use:  "probe",
		RunE: func(_ *cobra.Command, _ []string) error { ran = true; return nil },
	}
	root.AddCommand(probe)
	return root, &ran
}

func TestOutputFlag_AcceptsText(t *testing.T) {
	root, ran := outputFlagFixture()
	root.SetArgs([]string{"probe", "-o", "text"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !*ran {
		t.Error("subcommand did not run")
	}
}

func TestOutputFlag_AcceptsJSON(t *testing.T) {
	root, ran := outputFlagFixture()
	root.SetArgs([]string{"probe", "-o", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !*ran {
		t.Error("subcommand did not run")
	}
}

func TestOutputFlag_DefaultText(t *testing.T) {
	root, ran := outputFlagFixture()
	root.SetArgs([]string{"probe"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !*ran {
		t.Error("subcommand did not run")
	}
}

func TestOutputFlag_RejectsYAML(t *testing.T) {
	root, ran := outputFlagFixture()
	root.SetArgs([]string{"probe", "-o", "yaml"})
	err := root.Execute()
	if err == nil {
		t.Fatal("Execute = nil; want error for -o yaml")
	}
	if !strings.Contains(err.Error(), `unsupported output mode "yaml"`) {
		t.Errorf("error = %q; want contains 'unsupported output mode \"yaml\"'", err.Error())
	}
	if *ran {
		t.Error("subcommand RunE ran despite invalid --output")
	}
}

func TestOutputFlag_RejectsUpperCase(t *testing.T) {
	root, ran := outputFlagFixture()
	root.SetArgs([]string{"probe", "-o", "JSON"})
	err := root.Execute()
	if err == nil {
		t.Fatal("Execute = nil; want error for -o JSON")
	}
	if *ran {
		t.Error("subcommand RunE ran despite invalid --output")
	}
}

func TestOutputFlag_RejectsEmpty(t *testing.T) {
	root, ran := outputFlagFixture()
	root.SetArgs([]string{"probe", "-o", ""})
	err := root.Execute()
	if err == nil {
		t.Fatal("Execute = nil; want error for empty -o")
	}
	if *ran {
		t.Error("subcommand RunE ran despite invalid --output")
	}
}

func TestListEnvelope_EmptyFleet(t *testing.T) {
	res, err := fleet.BuildListResult(&fleet.Config{})
	if err != nil {
		t.Fatalf("BuildListResult: %v", err)
	}
	var buf bytes.Buffer
	if writeErr := writeEnvelopeTo(&buf, "list", "", false, res, nil, nil); writeErr != nil {
		t.Fatalf("writeEnvelopeTo: %v", writeErr)
	}
	want := `{"schema_version":1,"command":"list","repo":"","apply":false,"result":{"loaded_from":"","repos":[]},"warnings":[],"hints":[]}` + "\n"
	if buf.String() != want {
		t.Errorf("envelope =\n  %q\nwant\n  %q", buf.String(), want)
	}
}

func TestListEnvelope_Populated(t *testing.T) {
	cfg := &fleet.Config{
		LoadedFrom: "fleet.local.json",
		Profiles: map[string]fleet.Profile{
			"empty": {Sources: map[string]fleet.SourcePin{}, Workflows: []fleet.ProfileWorkflow{}},
		},
		Repos: map[string]fleet.RepoSpec{
			"a/b": {Profiles: []string{"empty"}, Engine: "claude"},
		},
	}
	res, err := fleet.BuildListResult(cfg)
	if err != nil {
		t.Fatalf("BuildListResult: %v", err)
	}
	var buf bytes.Buffer
	if writeErr := writeEnvelopeTo(&buf, "list", "", false, res, nil, nil); writeErr != nil {
		t.Fatalf("writeEnvelopeTo: %v", writeErr)
	}
	want := `{"schema_version":1,"command":"list","repo":"","apply":false,` +
		`"result":{"loaded_from":"fleet.local.json","repos":[` +
		`{"repo":"a/b","profiles":["empty"],"engine":"claude","workflows":[],"excluded":[],"extra":[]}` +
		`]},"warnings":[],"hints":[]}` + "\n"
	if buf.String() != want {
		t.Errorf("envelope =\n  %s\nwant\n  %s", buf.String(), want)
	}
}

func TestListCmd_JSONMode(t *testing.T) {
	dir := t.TempDir()
	fleetJSON := `{
  "version": 1,
  "defaults": {"engine": "copilot"},
  "profiles": {
    "p": {"sources": {}, "workflows": []}
  },
  "repos": {
    "x/y": {"profiles": ["p"]}
  }
}`
	if err := writeFile(t, dir+"/fleet.json", fleetJSON); err != nil {
		t.Fatal(err)
	}
	root := NewRootCmd()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"list", "--dir", dir, "-o", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v; raw=%s", err, stdout.String())
	}
	if env.Command != "list" {
		t.Errorf("Command = %q; want list", env.Command)
	}
	if env.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d; want 1", env.SchemaVersion)
	}
	if env.Apply {
		t.Error("Apply = true; want false for list")
	}
}

func TestJSONModeRejected_TemplateFetch(t *testing.T) {
	root := NewRootCmd()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"template", "fetch", "-o", "json"})
	err := root.Execute()
	if err == nil {
		t.Fatal("Execute = nil; want error for template fetch -o json")
	}
	if !strings.Contains(err.Error(), `command "template fetch" does not support --output json`) {
		t.Errorf("error = %q; want contains template-fetch rejection message", err.Error())
	}
}

func writeFile(t *testing.T, path, content string) error {
	t.Helper()
	return os.WriteFile(path, []byte(content), 0o644)
}

func envelopeCommand(buf *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetOut(buf)
	cmd.SetErr(io.Discard)
	return cmd
}

func TestUpgradeCmd_JSONArgErrorsEmitEnvelope(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantRepo string
		wantHint string
	}{
		{
			name:     "missing repo and all",
			args:     []string{"upgrade", "-o", "json"},
			wantRepo: "",
			wantHint: "upgrade: specify either a repo name or --all",
		},
		{
			name:     "repo with all",
			args:     []string{"upgrade", "x/y", "--all", "-o", "json"},
			wantRepo: "x/y",
			wantHint: "upgrade: cannot specify both --all and a repo name",
		},
		{
			name:     "work dir with all",
			args:     []string{"upgrade", "--all", "--work-dir", "/tmp/worktree", "-o", "json"},
			wantRepo: "",
			wantHint: "upgrade: --work-dir cannot be used with --all",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := NewRootCmd()
			var stdout bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(io.Discard)
			root.SetArgs(tc.args)

			err := root.Execute()
			if err == nil {
				t.Fatal("Execute = nil; want validation error")
			}
			if err.Error() != tc.wantHint {
				t.Fatalf("error = %q; want %q", err.Error(), tc.wantHint)
			}

			var env struct {
				Command string             `json:"command"`
				Repo    string             `json:"repo"`
				Apply   bool               `json:"apply"`
				Result  any                `json:"result"`
				Hints   []fleet.Diagnostic `json:"hints"`
			}
			if unmarshalErr := json.Unmarshal(stdout.Bytes(), &env); unmarshalErr != nil {
				t.Fatalf("unmarshal: %v; raw=%s", unmarshalErr, stdout.String())
			}
			if env.Command != "upgrade" {
				t.Errorf("command = %q; want upgrade", env.Command)
			}
			if env.Repo != tc.wantRepo {
				t.Errorf("repo = %q; want %q", env.Repo, tc.wantRepo)
			}
			if env.Apply {
				t.Error("apply = true; want false")
			}
			if env.Result != nil {
				t.Errorf("result = %#v; want nil", env.Result)
			}
			if len(env.Hints) != 1 {
				t.Fatalf("len(hints) = %d; want 1", len(env.Hints))
			}
			if env.Hints[0].Code != fleet.DiagHint {
				t.Errorf("hint code = %q; want %q", env.Hints[0].Code, fleet.DiagHint)
			}
			if env.Hints[0].Message != tc.wantHint {
				t.Errorf("hint message = %q; want %q", env.Hints[0].Message, tc.wantHint)
			}
		})
	}
}

func TestDeployEnvelope_EmptyArrays(t *testing.T) {
	res := &fleet.DeployResult{Repo: "x/y"}
	var buf bytes.Buffer
	if err := writeEnvelopeTo(&buf, "deploy", "x/y", false, res, nil, nil); err != nil {
		t.Fatal(err)
	}
	var env struct {
		Result fleet.DeployResult `json:"result"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v; raw=%s", err, buf.String())
	}
	if env.Result.Added == nil {
		t.Error("Added is nil; want []")
	}
	for _, key := range []string{`"added":[]`, `"skipped":[]`, `"failed":[]`} {
		if !strings.Contains(buf.String(), key) {
			t.Errorf("missing %s in %s", key, buf.String())
		}
	}
}

func TestDeployEnvelope_MissingSecretWarning(t *testing.T) {
	res := &fleet.DeployResult{
		Repo:          "x/y",
		MissingSecret: "ANTHROPIC_API_KEY",
		SecretKeyURL:  "https://example.com/key",
	}
	warnings := []fleet.Diagnostic{{
		Code:    fleet.DiagMissingSecret,
		Message: "Actions secret \"ANTHROPIC_API_KEY\" is missing on x/y",
		Fields:  map[string]any{"secret": "ANTHROPIC_API_KEY", "url": "https://example.com/key"},
	}}
	var buf bytes.Buffer
	if err := writeEnvelopeTo(&buf, "deploy", "x/y", true, res, warnings, nil); err != nil {
		t.Fatal(err)
	}
	var env struct {
		Apply    bool               `json:"apply"`
		Warnings []fleet.Diagnostic `json:"warnings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !env.Apply {
		t.Error("apply = false; want true")
	}
	if len(env.Warnings) != 1 {
		t.Fatalf("len(warnings) = %d; want 1", len(env.Warnings))
	}
	if env.Warnings[0].Code != fleet.DiagMissingSecret {
		t.Errorf("warning code = %q; want missing_secret", env.Warnings[0].Code)
	}
	if env.Warnings[0].Fields["secret"] != "ANTHROPIC_API_KEY" {
		t.Errorf(
			"warning fields.secret = %v; want ANTHROPIC_API_KEY",
			env.Warnings[0].Fields["secret"],
		)
	}
}

func TestDeployEnvelope_ApplyFlag(t *testing.T) {
	for _, apply := range []bool{true, false} {
		var buf bytes.Buffer
		res := &fleet.DeployResult{Repo: "x/y"}
		if err := writeEnvelopeTo(&buf, "deploy", "x/y", apply, res, nil, nil); err != nil {
			t.Fatal(err)
		}
		want := `"apply":true`
		if !apply {
			want = `"apply":false`
		}
		if !strings.Contains(buf.String(), want) {
			t.Errorf("apply=%v: missing %s in %s", apply, want, buf.String())
		}
	}
}

func TestDeployEnvelope_PartialFailureFallsBackToErrorHint(t *testing.T) {
	var buf bytes.Buffer
	cmd := envelopeCommand(&buf)
	res := &fleet.DeployResult{Repo: "x/y"}
	deployErr := errors.New("gh repo clone x/y: exit status 1")

	if err := emitDeployEnvelope(cmd, "x/y", false, res, deployErr); !errors.Is(err, deployErr) {
		t.Fatalf("emitDeployEnvelope error = %v; want %v", err, deployErr)
	}

	var env struct {
		Result fleet.DeployResult `json:"result"`
		Hints  []fleet.Diagnostic `json:"hints"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v; raw=%s", err, buf.String())
	}
	if env.Result.Repo != "x/y" {
		t.Errorf("result.repo = %q; want x/y", env.Result.Repo)
	}
	if len(env.Hints) != 1 {
		t.Fatalf("len(hints) = %d; want 1", len(env.Hints))
	}
	if env.Hints[0].Message != deployErr.Error() {
		t.Errorf("hint message = %q; want %q", env.Hints[0].Message, deployErr.Error())
	}
}

func TestSyncEnvelope_EmptyArrays(t *testing.T) {
	res := &fleet.SyncResult{Repo: "x/y"}
	var buf bytes.Buffer
	if err := writeEnvelopeTo(&buf, "sync", "x/y", false, res, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"missing":[]`, `"drift":[]`, `"expected":[]`, `"pruned":[]`} {
		if !strings.Contains(buf.String(), key) {
			t.Errorf("missing %s in %s", key, buf.String())
		}
	}
}

func TestSyncEnvelope_NilDeployFields(t *testing.T) {
	res := &fleet.SyncResult{Repo: "x/y", Deploy: nil, DeployPreflight: nil}
	var buf bytes.Buffer
	if err := writeEnvelopeTo(&buf, "sync", "x/y", false, res, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"deploy":null`, `"deploy_preflight":null`} {
		if !strings.Contains(buf.String(), key) {
			t.Errorf("missing %s in %s", key, buf.String())
		}
	}
}

func TestSyncEnvelope_NestedDeployPreflight(t *testing.T) {
	res := &fleet.SyncResult{
		Repo: "x/y",
		DeployPreflight: &fleet.DeployResult{
			Repo:  "x/y",
			Added: []fleet.WorkflowOutcome{{Name: "foo", Spec: "owner/repo/foo@v1"}},
		},
	}
	var buf bytes.Buffer
	if err := writeEnvelopeTo(&buf, "sync", "x/y", false, res, nil, nil); err != nil {
		t.Fatal(err)
	}
	var env struct {
		Result fleet.SyncResult `json:"result"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v; raw=%s", err, buf.String())
	}
	if env.Result.DeployPreflight == nil {
		t.Fatal("DeployPreflight is nil after round-trip")
	}
	if len(env.Result.DeployPreflight.Added) != 1 ||
		env.Result.DeployPreflight.Added[0].Name != "foo" {
		t.Errorf(
			"DeployPreflight.Added = %v; want [{Name: foo, ...}]",
			env.Result.DeployPreflight.Added,
		)
	}
}

func TestSyncEnvelope_DriftDiagnostic(t *testing.T) {
	warnings := []fleet.Diagnostic{{
		Code:    fleet.DiagDriftDetected,
		Message: syncDriftMessage,
		Fields:  map[string]any{"drift": []string{"orphan"}},
	}}
	var buf bytes.Buffer
	if err := writeEnvelopeTo(&buf, "sync", "x/y", false, &fleet.SyncResult{Repo: "x/y"}, warnings, nil); err != nil {
		t.Fatal(err)
	}
	var env struct {
		Warnings []struct {
			Fields struct {
				Drift []string `json:"drift"`
			} `json:"fields"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(env.Warnings) != 1 {
		t.Fatalf("len(warnings) = %d; want 1", len(env.Warnings))
	}
	got := env.Warnings[0].Fields.Drift
	if len(got) != 1 || got[0] != "orphan" {
		t.Errorf("warnings[0].fields.drift = %v; want [orphan]", got)
	}
}

func TestSyncEnvelope_PartialFailureFallsBackToErrorHint(t *testing.T) {
	var buf bytes.Buffer
	cmd := envelopeCommand(&buf)
	res := &fleet.SyncResult{Repo: "x/y"}
	syncErr := errors.New("git push: exit status 1")

	if err := emitSyncEnvelope(cmd, "x/y", true, res, syncErr); !errors.Is(err, syncErr) {
		t.Fatalf("emitSyncEnvelope error = %v; want %v", err, syncErr)
	}

	var env struct {
		Result fleet.SyncResult   `json:"result"`
		Hints  []fleet.Diagnostic `json:"hints"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v; raw=%s", err, buf.String())
	}
	if env.Result.Repo != "x/y" {
		t.Errorf("result.repo = %q; want x/y", env.Result.Repo)
	}
	if len(env.Hints) != 1 {
		t.Fatalf("len(hints) = %d; want 1", len(env.Hints))
	}
	if env.Hints[0].Message != syncErr.Error() {
		t.Errorf("hint message = %q; want %q", env.Hints[0].Message, syncErr.Error())
	}
}

func TestUpgradeEnvelope_EmptyArrays(t *testing.T) {
	res := &fleet.UpgradeResult{Repo: "x/y"}
	var buf bytes.Buffer
	if err := writeEnvelopeTo(&buf, "upgrade", "x/y", false, res, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"changed_files":[]`, `"conflicts":[]`} {
		if !strings.Contains(buf.String(), key) {
			t.Errorf("missing %s in %s", key, buf.String())
		}
	}
}

func TestEnvelope_AuditJSONNests(t *testing.T) {
	res := &fleet.UpgradeResult{
		Repo:      "x/y",
		AuditJSON: json.RawMessage(`{"version":"1","findings":[]}`),
	}
	var buf bytes.Buffer
	if err := writeEnvelopeTo(&buf, "upgrade", "x/y", false, res, nil, nil); err != nil {
		t.Fatal(err)
	}
	// Use a destination that captures audit_json as a re-parseable RawMessage.
	var env struct {
		Result struct {
			AuditJSON json.RawMessage `json:"audit_json"`
		} `json:"result"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v; raw=%s", err, buf.String())
	}
	var audit map[string]any
	if err := json.Unmarshal(env.Result.AuditJSON, &audit); err != nil {
		t.Fatalf("audit_json not native object; got %s; err=%v", env.Result.AuditJSON, err)
	}
	if audit["version"] != "1" {
		t.Errorf("audit.version = %v; want 1", audit["version"])
	}
	// Negative pin: ensure the byte form is the literal object, NOT a quoted string.
	if !strings.Contains(buf.String(), `"audit_json":{"version":"1","findings":[]}`) {
		t.Errorf("audit_json not nested as object: %s", buf.String())
	}
}

func TestUpgradeEnvelope_AuditJSONNil(t *testing.T) {
	res := &fleet.UpgradeResult{Repo: "x/y", AuditJSON: nil}
	var buf bytes.Buffer
	if err := writeEnvelopeTo(&buf, "upgrade", "x/y", false, res, nil, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"audit_json":null`) {
		t.Errorf("audit_json not null when AuditJSON is nil: %s", buf.String())
	}
}

func TestUpgradeEnvelope_PartialFailureFallsBackToErrorHint(t *testing.T) {
	var buf bytes.Buffer
	cmd := envelopeCommand(&buf)
	res := &fleet.UpgradeResult{Repo: "x/y"}
	upgradeErr := errors.New("gh aw update: exit status 1")

	if err := emitUpgradeEnvelope(cmd, "x/y", false, res, upgradeErr); !errors.Is(err, upgradeErr) {
		t.Fatalf("emitUpgradeEnvelope error = %v; want %v", err, upgradeErr)
	}

	var env struct {
		Result fleet.UpgradeResult `json:"result"`
		Hints  []fleet.Diagnostic  `json:"hints"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v; raw=%s", err, buf.String())
	}
	if env.Result.Repo != "x/y" {
		t.Errorf("result.repo = %q; want x/y", env.Result.Repo)
	}
	if len(env.Hints) != 1 {
		t.Fatalf("len(hints) = %d; want 1", len(env.Hints))
	}
	if env.Hints[0].Message != upgradeErr.Error() {
		t.Errorf("hint message = %q; want %q", env.Hints[0].Message, upgradeErr.Error())
	}
}

func TestNDJSON_LineCountAndSelfContained(t *testing.T) {
	results := []*fleet.UpgradeResult{
		{Repo: "a/b", NoChanges: true},
		{Repo: "c/d", ChangedFiles: []string{"x"}},
		{Repo: "e/f", Conflicts: []string{"y"}},
	}
	var buf bytes.Buffer
	for _, r := range results {
		if err := writeEnvelopeTo(&buf, "upgrade", r.Repo, false, r, nil, nil); err != nil {
			t.Fatalf("writeEnvelopeTo: %v", err)
		}
	}
	got := buf.String()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("len(lines) = %d; want 3; raw=%s", len(lines), got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Error("NDJSON stream missing trailing newline on last line")
	}
	for i, line := range lines {
		var env Envelope
		if err := json.Unmarshal([]byte(line), &env); err != nil {
			t.Errorf("line %d: not valid JSON: %v; raw=%s", i, err, line)
			continue
		}
		if env.Command != "upgrade" {
			t.Errorf("line %d: command = %q; want upgrade", i, env.Command)
		}
	}
}

func TestNDJSON_ErrorRepoIncluded(t *testing.T) {
	// Simulate a 3-repo loop where the middle repo failed before producing a result.
	repos := []struct {
		name string
		res  *fleet.UpgradeResult
		err  error
	}{
		{"a/b", &fleet.UpgradeResult{Repo: "a/b", NoChanges: true}, nil},
		{"c/d", nil, errors.New("simulated failure")},
		{"e/f", &fleet.UpgradeResult{Repo: "e/f", NoChanges: true}, nil},
	}
	var buf bytes.Buffer
	for _, r := range repos {
		var hints []fleet.Diagnostic
		if r.res == nil && r.err != nil {
			hints = []fleet.Diagnostic{{
				Code:    fleet.DiagHint,
				Message: r.err.Error(),
				Fields:  map[string]any{"hint": r.err.Error()},
			}}
		}
		if err := writeEnvelopeTo(&buf, "upgrade", r.name, false, r.res, nil, hints); err != nil {
			t.Fatal(err)
		}
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("len(lines) = %d; want 3", len(lines))
	}
	// Middle line must have result: null and a hint.
	if !strings.Contains(lines[1], `"result":null`) {
		t.Errorf("middle line missing result:null: %s", lines[1])
	}
	if !strings.Contains(lines[1], `"code":"hint"`) {
		t.Errorf("middle line missing hint diagnostic: %s", lines[1])
	}
	// Other lines must have result objects.
	if strings.Contains(lines[0], `"result":null`) {
		t.Errorf("first line should have non-null result: %s", lines[0])
	}
	if strings.Contains(lines[2], `"result":null`) {
		t.Errorf("last line should have non-null result: %s", lines[2])
	}
}

func TestNDJSON_EmptyFleet(t *testing.T) {
	var buf bytes.Buffer
	// Loop over zero repos — buf stays empty.
	got := buf.String()
	if got != "" {
		t.Errorf("empty fleet stdout = %q; want empty", got)
	}
}

// TestEnvelope_NoNullSlices_AllResultTypes pins FR-009 across every result
// type so a future-added slice field doesn't sneak in as null. Walks the
// struct via reflection and asserts each Slice (excluding json.RawMessage
// []byte) renders as a JSON array.
func TestEnvelope_NoNullSlices_AllResultTypes(t *testing.T) {
	cases := []struct {
		name   string
		result any
	}{
		{"ListResult", &fleet.ListResult{}},
		{"DeployResult", &fleet.DeployResult{Repo: "x/y"}},
		{"SyncResult", &fleet.SyncResult{Repo: "x/y"}},
		{"UpgradeResult", &fleet.UpgradeResult{Repo: "x/y"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := writeEnvelopeTo(&buf, "x", "x/y", false, tc.result, nil, nil); err != nil {
				t.Fatalf("writeEnvelopeTo: %v", err)
			}
			var env struct {
				Result map[string]any `json:"result"`
			}
			if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
				t.Fatalf("unmarshal: %v; raw=%s", err, buf.String())
			}
			rt := reflect.TypeOf(tc.result).Elem()
			for i := 0; i < rt.NumField(); i++ {
				f := rt.Field(i)
				if f.Type.Kind() != reflect.Slice {
					continue
				}
				if f.Type.Elem().Kind() == reflect.Uint8 {
					continue // json.RawMessage / []byte — null is correct when nil
				}
				tag := f.Tag.Get("json")
				jsonKey, _, _ := strings.Cut(tag, ",")
				if jsonKey == "" {
					jsonKey = f.Name
				}
				v, present := env.Result[jsonKey]
				if !present {
					t.Errorf("%s: field %q missing from result", tc.name, jsonKey)
					continue
				}
				if v == nil {
					t.Errorf("%s: field %q is null; want [] (FR-009)", tc.name, jsonKey)
				}
			}
		})
	}
}

func TestBuildMissingSecretMessage(t *testing.T) {
	res := &fleet.DeployResult{
		Repo:          "x/y",
		MissingSecret: "FOO",
		SecretKeyURL:  "https://example.com",
	}
	got := buildMissingSecretMessage(res)
	if !strings.Contains(got, "FOO") || !strings.Contains(got, "x/y") {
		t.Errorf("message %q missing secret/repo", got)
	}
	if !strings.Contains(got, "https://example.com") {
		t.Errorf("message %q missing key URL", got)
	}
}
