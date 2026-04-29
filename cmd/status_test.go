package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
)

func newStatusTestCmd(stdout, stderr *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{Use: "status"}
	cmd.Flags().StringP("output", "o", "json", "")
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd
}

func TestEmitStatusEnvelope_Success(t *testing.T) {
	res := &fleet.StatusResult{
		Repos: []fleet.RepoStatus{
			{Repo: "rshade/aligned", DriftState: "aligned"},
			{
				Repo:       "rshade/drifted",
				DriftState: "drifted",
				Drifted: []fleet.WorkflowDrift{
					{Name: "audit", DesiredRef: "v1.1", ActualRef: "v1.0"},
				},
			},
		},
	}

	var stdout, stderr bytes.Buffer
	cmd := newStatusTestCmd(&stdout, &stderr)
	err := emitStatusEnvelope(cmd, "", res, nil)
	if !errors.Is(err, errStatusDrift) {
		t.Fatalf("emitStatusEnvelope returned %v; want errStatusDrift (drift triggers exit 1)", err)
	}

	var env Envelope
	if uerr := json.Unmarshal(stdout.Bytes(), &env); uerr != nil {
		t.Fatalf("unmarshal envelope: %v", uerr)
	}
	if env.SchemaVersion != 1 {
		t.Errorf("schema_version = %d; want 1", env.SchemaVersion)
	}
	if env.Command != "status" {
		t.Errorf("command = %q; want status", env.Command)
	}
	if env.Apply {
		t.Errorf("apply = true; want false")
	}
	if env.Repo != "" {
		t.Errorf("repo = %q; want empty", env.Repo)
	}
	if env.Result == nil {
		t.Fatal("result is null; want StatusResult")
	}
	if env.Warnings == nil {
		t.Error("warnings serialized as null; want []")
	}
	if env.Hints == nil {
		t.Error("hints serialized as null; want []")
	}
}

func TestEmitStatusEnvelope_AllAligned_NoError(t *testing.T) {
	res := &fleet.StatusResult{
		Repos: []fleet.RepoStatus{
			{Repo: "rshade/aligned", DriftState: "aligned"},
		},
	}
	var stdout, stderr bytes.Buffer
	cmd := newStatusTestCmd(&stdout, &stderr)
	err := emitStatusEnvelope(cmd, "", res, nil)
	if err != nil {
		t.Fatalf("emitStatusEnvelope(aligned-only) returned %v; want nil", err)
	}
}

func TestEmitStatusEnvelope_EmptySlicesSerializeAsArrays(t *testing.T) {
	// FR-015: nil drift slices must serialize as `[]`, never `null`.
	res := &fleet.StatusResult{
		Repos: []fleet.RepoStatus{
			{Repo: "rshade/aligned", DriftState: "aligned"},
		},
	}
	var stdout, stderr bytes.Buffer
	cmd := newStatusTestCmd(&stdout, &stderr)
	if err := emitStatusEnvelope(cmd, "", res, nil); err != nil {
		t.Fatalf("emitStatusEnvelope: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	repos, _ := raw["result"].(map[string]any)["repos"].([]any)
	if len(repos) != 1 {
		t.Fatalf("len(result.repos) = %d; want 1", len(repos))
	}
	r0 := repos[0].(map[string]any)
	for _, key := range []string{"missing", "extra", "drifted", "unpinned"} {
		v, ok := r0[key]
		if !ok {
			t.Errorf("repo[0].%s missing from JSON", key)
			continue
		}
		if v == nil {
			t.Errorf("repo[0].%s serialized as null; want []", key)
		}
		if _, isArray := v.([]any); !isArray {
			t.Errorf("repo[0].%s is %T; want array", key, v)
		}
	}
}

func TestEmitStatusEnvelope_ErroredRepoSurfacesHint(t *testing.T) {
	res := &fleet.StatusResult{
		Repos: []fleet.RepoStatus{
			{
				Repo:         "rshade/private",
				DriftState:   "errored",
				ErrorMessage: "HTTP 404",
			},
		},
	}
	diags := []fleet.Diagnostic{
		{
			Code:    fleet.DiagRepoInaccessible,
			Message: "HTTP 404",
			Fields:  map[string]any{"repo": "rshade/private"},
		},
	}
	var stdout, stderr bytes.Buffer
	cmd := newStatusTestCmd(&stdout, &stderr)
	err := emitStatusEnvelope(cmd, "", res, diags)
	if !errors.Is(err, errStatusDrift) {
		t.Fatalf("emitStatusEnvelope returned %v; want errStatusDrift", err)
	}
	var env Envelope
	if uerr := json.Unmarshal(stdout.Bytes(), &env); uerr != nil {
		t.Fatalf("unmarshal envelope: %v", uerr)
	}
	if len(env.Hints) != 1 {
		t.Fatalf("len(hints) = %d; want 1", len(env.Hints))
	}
	if env.Hints[0].Code != fleet.DiagRepoInaccessible {
		t.Errorf("hint.code = %q; want %q", env.Hints[0].Code, fleet.DiagRepoInaccessible)
	}
	if env.Hints[0].Fields["repo"] != "rshade/private" {
		t.Errorf("hint.fields.repo = %v; want rshade/private", env.Hints[0].Fields["repo"])
	}
}

func TestEmitStatusEnvelope_EmptyFleetWarning(t *testing.T) {
	res := &fleet.StatusResult{Repos: []fleet.RepoStatus{}}
	diags := []fleet.Diagnostic{
		{Code: fleet.DiagEmptyFleet, Message: "fleet config declares zero repos"},
	}
	var stdout, stderr bytes.Buffer
	cmd := newStatusTestCmd(&stdout, &stderr)
	if err := emitStatusEnvelope(cmd, "", res, diags); err != nil {
		t.Fatalf("emitStatusEnvelope: %v", err)
	}
	var env Envelope
	if uerr := json.Unmarshal(stdout.Bytes(), &env); uerr != nil {
		t.Fatalf("unmarshal: %v", uerr)
	}
	if len(env.Warnings) != 1 {
		t.Fatalf("len(warnings) = %d; want 1", len(env.Warnings))
	}
	if env.Warnings[0].Code != fleet.DiagEmptyFleet {
		t.Errorf("warning.code = %q; want empty_fleet", env.Warnings[0].Code)
	}
}

func TestPreResultFailureEnvelope_Status(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := newStatusTestCmd(&stdout, &stderr)
	err := preResultFailureEnvelope(cmd, "status", "some/unknown", false,
		errors.New(`repo "some/unknown" is not declared in fleet config`))
	if err == nil {
		t.Fatal("preResultFailureEnvelope returned nil; want propagated err")
	}

	var raw map[string]any
	if uerr := json.Unmarshal(stdout.Bytes(), &raw); uerr != nil {
		t.Fatalf("unmarshal: %v", uerr)
	}
	if raw["result"] != nil {
		t.Errorf("result = %v; want null", raw["result"])
	}
	if raw["repo"] != "some/unknown" {
		t.Errorf("repo = %v; want some/unknown", raw["repo"])
	}
	hints, _ := raw["hints"].([]any)
	if len(hints) != 1 {
		t.Fatalf("len(hints) = %d; want 1", len(hints))
	}
}

// TestStatusTextOutputUnaffectedByJSONHelpers (C5 / FR-019): rendering text
// twice — once after the JSON helper has been invoked and discarded, once
// without — must produce byte-identical stdout. The two modes must be disjoint.
func TestStatusTextOutputUnaffectedByJSONHelpers(t *testing.T) {
	mkRes := func() *fleet.StatusResult {
		return &fleet.StatusResult{
			Repos: []fleet.RepoStatus{
				{
					Repo:       "rshade/drifted",
					DriftState: "drifted",
					Missing:    []string{"audit"},
					Drifted: []fleet.WorkflowDrift{
						{Name: "x", DesiredRef: "v1.1", ActualRef: "v1.0"},
					},
				},
			},
		}
	}

	captureText := func() string {
		var stdout, stderr bytes.Buffer
		cmd := newStatusTestCmd(&stdout, &stderr)
		printStatus(cmd, mkRes(), nil)
		return stdout.String()
	}

	first := captureText()

	// Run the JSON helper and discard.
	{
		var jsonStdout, jsonStderr bytes.Buffer
		cmd := newStatusTestCmd(&jsonStdout, &jsonStderr)
		_ = emitStatusEnvelope(cmd, "", mkRes(), nil)
	}

	second := captureText()

	if first != second {
		t.Errorf("text-mode output changed after JSON helper invocation\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestSplitStatusDiags(t *testing.T) {
	diags := []fleet.Diagnostic{
		{Code: fleet.DiagEmptyFleet},
		{Code: fleet.DiagRepoInaccessible, Fields: map[string]any{"repo": "z"}},
		{Code: fleet.DiagRateLimited, Fields: map[string]any{"repo": "a"}},
		{Code: fleet.DiagNetworkUnreachable, Fields: map[string]any{"repo": "m"}},
	}
	warnings, hints := splitStatusDiags(diags)
	if len(warnings) != 1 || warnings[0].Code != fleet.DiagEmptyFleet {
		t.Errorf("warnings = %#v; want [empty_fleet]", warnings)
	}
	if len(hints) != 3 {
		t.Fatalf("len(hints) = %d; want 3", len(hints))
	}
	gotRepos := []string{
		hints[0].Fields["repo"].(string),
		hints[1].Fields["repo"].(string),
		hints[2].Fields["repo"].(string),
	}
	wantRepos := []string{"a", "m", "z"}
	if !reflect.DeepEqual(gotRepos, wantRepos) {
		t.Errorf("hints not sorted by repo: got %v want %v", gotRepos, wantRepos)
	}
}

func TestStatusExitCode(t *testing.T) {
	cases := []struct {
		name    string
		res     *fleet.StatusResult
		wantErr bool
	}{
		{"nil", nil, false},
		{"empty", &fleet.StatusResult{Repos: []fleet.RepoStatus{}}, false},
		{
			"all-aligned",
			&fleet.StatusResult{Repos: []fleet.RepoStatus{
				{Repo: "a", DriftState: "aligned"},
				{Repo: "b", DriftState: "aligned"},
			}},
			false,
		},
		{
			"one-drifted",
			&fleet.StatusResult{Repos: []fleet.RepoStatus{
				{Repo: "a", DriftState: "aligned"},
				{Repo: "b", DriftState: "drifted"},
			}},
			true,
		},
		{
			"one-errored",
			&fleet.StatusResult{Repos: []fleet.RepoStatus{
				{Repo: "a", DriftState: "errored"},
			}},
			true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := statusExitCode(tc.res)
			if tc.wantErr && err == nil {
				t.Errorf("expected non-nil err")
			}
			if tc.wantErr && !SuppressErrorLog(err) {
				t.Errorf("expected status drift error to suppress fatal logging")
			}
			if tc.wantErr {
				code, ok := ExitCodeForError(err)
				if !ok {
					t.Errorf("expected status drift error to carry an exit code")
				}
				if code != 1 {
					t.Errorf("exit code = %d; want 1", code)
				}
			}
			if !tc.wantErr && err != nil {
				t.Errorf("expected nil err; got %v", err)
			}
		})
	}
}
