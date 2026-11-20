package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
)

type overviewStub func(
	context.Context,
	*fleet.Config,
	fleet.OverviewOpts,
) (*fleet.OverviewResult, []fleet.Diagnostic, error)

func TestOverviewCmdText(t *testing.T) {
	res := overviewCmdResult()
	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	if err := renderOverviewText(cmd, res); err != nil {
		t.Fatalf("renderOverviewText: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		columnHeaderRepo, "DRIFT", "RUNS", "FAIL", "NOOP", "HEALTH", "AIC", "COST",
		"------------------------------------------------------------------------",
		"TOTAL", "69", "11", "40", "84%", "165.50", "$1.65",
		"acme/api", "9", "9", "0", "0%", "-", "acme/ghost", "errored",
		"acme/widgets (drifted):", "runs: 18 (0 failed, 4 no-op)",
		"acme/api (unhealthy):",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestOverviewCmdTextEmptyFleetOmitsTotal(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	res := &fleet.OverviewResult{
		LoadedFrom: "fleet.json",
		Window:     "trailing-7d",
		Repos:      []fleet.RepoOverview{},
	}
	if err := renderOverviewText(cmd, res); err != nil {
		t.Fatalf("renderOverviewText: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "REPO\tDRIFT\tRUNS\tFAIL\tNOOP\tHEALTH\tAIC\tCOST") &&
		!strings.Contains(got, "REPO  DRIFT") {
		t.Fatalf("output missing header:\n%s", got)
	}
	for _, unwanted := range []string{
		"------------------------------------------------------------------------",
		"TOTAL",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("empty-fleet output contains %q:\n%s", unwanted, got)
		}
	}
}

func renderOverviewRowDetail(t *testing.T, row fleet.RepoOverview) string {
	t.Helper()
	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	printOverviewDetail(cmd, row)
	return out.String()
}

func renderOverviewRows(t *testing.T, rows ...fleet.RepoOverview) string {
	t.Helper()
	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	res := &fleet.OverviewResult{LoadedFrom: "fleet.json", Window: "trailing-7d", Repos: rows}
	if err := renderOverviewText(cmd, res); err != nil {
		t.Fatalf("renderOverviewText: %v", err)
	}
	return out.String()
}

func TestOverviewCmdModelSupportText(t *testing.T) {
	affected := fleet.RepoOverview{
		Repo:          "rshade/finfocus",
		DriftState:    fleet.DriftStateAligned,
		Runs:          4,
		Failures:      4,
		HealthRate:    f64ptr(0),
		RunsAvailable: true,
		ModelSupport:  &fleet.ModelSupport{Status: "affected", OffendingModels: []string{"claude-sonnet-4.6"}},
	}
	if got := renderOverviewRowDetail(t, affected); !strings.Contains(got,
		"model-support: FAILING — offending model(s): claude-sonnet-4.6") {
		t.Fatalf("affected detail missing model line:\n%s", got)
	}

	unknown := affected
	unknown.ModelSupport = &fleet.ModelSupport{Status: "affected"}
	if got := renderOverviewRowDetail(t, unknown); !strings.Contains(got,
		"model-support: FAILING — offending model(s): unknown") {
		t.Fatalf("affected-unknown detail missing unknown line:\n%s", got)
	}

	// Renderer-only defensive path: the v1 collector names at most one model,
	// but the wire field is a list, so multiple values comma-join.
	multi := affected
	multi.ModelSupport = &fleet.ModelSupport{
		Status:          "affected",
		OffendingModels: []string{"claude-sonnet-4.6", "gpt-5-mini"},
	}
	if got := renderOverviewRowDetail(t, multi); !strings.Contains(got,
		"model-support: FAILING — offending model(s): claude-sonnet-4.6, gpt-5-mini") {
		t.Fatalf("multi-model detail missing comma-joined models:\n%s", got)
	}

	// FR-003: an affected status always forces a detail block, even on an
	// otherwise drift-aligned, healthy repo — the open failure issue can
	// outlive the failing run window, so health gating alone could drop the
	// model-support annotation.
	recovered := fleet.RepoOverview{
		Repo:          "acme/recovered",
		DriftState:    fleet.DriftStateAligned,
		Runs:          5,
		Failures:      0,
		HealthRate:    f64ptr(1),
		RunsAvailable: true,
		ModelSupport:  &fleet.ModelSupport{Status: "affected", OffendingModels: []string{"gpt-5-mini"}},
	}
	recoveredOut := renderOverviewRows(t, recovered)
	if !strings.Contains(recoveredOut, "acme/recovered (") ||
		!strings.Contains(recoveredOut, "model-support: FAILING — offending model(s): gpt-5-mini") {
		t.Fatalf("affected-but-healthy repo lost its detail block:\n%s", recoveredOut)
	}

	undetermined := fleet.RepoOverview{
		Repo:          "acme/widgets",
		DriftState:    fleet.DriftStateDrifted,
		DriftDetail:   &fleet.RepoStatus{Repo: "acme/widgets", DriftState: fleet.DriftStateDrifted},
		RunsAvailable: true,
		ModelSupport:  &fleet.ModelSupport{Status: "undetermined"},
	}
	if got := renderOverviewRowDetail(t, undetermined); !strings.Contains(got,
		"model-support: undetermined — model-failure signal unavailable") {
		t.Fatalf("undetermined detail missing undetermined line:\n%s", got)
	}

	// FR-011: an undetermined status never forces a detail block on an otherwise
	// clean, drift-aligned, healthy repo.
	quiet := fleet.RepoOverview{
		Repo:          "acme/quiet",
		DriftState:    fleet.DriftStateAligned,
		Runs:          5,
		Failures:      0,
		HealthRate:    f64ptr(1),
		RunsAvailable: true,
		ModelSupport:  &fleet.ModelSupport{Status: "undetermined"},
	}
	got := renderOverviewRows(t, quiet)
	if strings.Contains(got, "model-support:") || strings.Contains(got, "acme/quiet (") {
		t.Fatalf("undetermined status forced a detail block on a clean repo:\n%s", got)
	}

	// SC-003: a clean repo with no ModelSupport renders no detail block at all.
	clean := fleet.RepoOverview{
		Repo:          "acme/api",
		DriftState:    fleet.DriftStateAligned,
		Runs:          9,
		Failures:      0,
		HealthRate:    f64ptr(1),
		RunsAvailable: true,
	}
	cleanOut := renderOverviewRows(t, clean)
	if strings.Contains(cleanOut, "model-support:") || strings.Contains(cleanOut, "acme/api (") {
		t.Fatalf("clean repo rendered a detail block:\n%s", cleanOut)
	}
}

func TestOverviewCmdFlags(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantKind   fleet.FetchKind
		wantDays   int
		wantWindow string
	}{
		{
			name:       "default trailing seven days",
			args:       []string{"overview"},
			wantKind:   fleet.FetchTrailing,
			wantDays:   7,
			wantWindow: "trailing-7d",
		},
		{
			name:       "explicit trailing",
			args:       []string{"overview", "--trailing", "14d"},
			wantKind:   fleet.FetchTrailing,
			wantDays:   14,
			wantWindow: "trailing-14d",
		},
		{
			name:       "since",
			args:       []string{"overview", "--since", "2026-06-01"},
			wantKind:   fleet.FetchSince,
			wantWindow: "since-2026-06-01",
		},
		{
			name:       flagLatest,
			args:       []string{"overview", "--" + flagLatest},
			wantKind:   fleet.FetchLatest,
			wantWindow: flagLatest,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeOverviewFleetConfig(t, []string{"a/one"})
			var gotMode fleet.FetchMode
			stub := func(_ context.Context, cfg *fleet.Config, opts fleet.OverviewOpts) (*fleet.OverviewResult, []fleet.Diagnostic, error) {
				gotMode = opts.Mode
				return &fleet.OverviewResult{
					LoadedFrom: cfg.LoadedFrom,
					Window:     tc.wantWindow,
					Repos:      []fleet.RepoOverview{},
				}, nil, nil
			}
			withOverviewStub(t, stub)

			stdout, stderr, err := executeOverviewRoot(append([]string{"--dir", dir}, tc.args...)...)
			if err != nil {
				t.Fatalf("Execute: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
			}
			if gotMode.Kind != tc.wantKind {
				t.Fatalf("mode.Kind = %v; want %v", gotMode.Kind, tc.wantKind)
			}
			if tc.wantDays != 0 && gotMode.Days != tc.wantDays {
				t.Fatalf("mode.Days = %d; want %d", gotMode.Days, tc.wantDays)
			}
			if !strings.Contains(stderr, "overview · window: "+tc.wantWindow) {
				t.Fatalf("stderr = %q; want window %q", stderr, tc.wantWindow)
			}
		})
	}

	stdout, stderr, err := executeOverviewRoot("overview", "--"+flagLatest, "--trailing", "7d")
	if err == nil {
		t.Fatalf("Execute: expected mutual-exclusion error; stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(err.Error(), "none of the others can be") {
		t.Fatalf("error = %q; want Cobra mutual-exclusion message", err.Error())
	}
}

func TestOverviewCmdScope(t *testing.T) {
	dir := writeOverviewFleetConfig(t, []string{"a/one", "b/two"})
	var scopedRepos []string
	withOverviewStub(t, recordScopedOverviewRepos(&scopedRepos))

	stdout, stderr, err := executeOverviewRoot("--dir", dir, "overview", "b/two")
	if err != nil {
		t.Fatalf("Execute: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if len(scopedRepos) != 1 || scopedRepos[0] != "b/two" {
		t.Fatalf("scoped repos = %v; want [b/two]", scopedRepos)
	}

	called := false
	withOverviewStub(t, markOverviewCalled(&called))
	_, _, err = executeOverviewRoot("--dir", dir, "overview", "missing/repo")
	if err == nil {
		t.Fatal("Execute: expected unknown repo error")
	}
	if called {
		t.Fatal("runOverview called for unknown repo; want fail-fast before fetch")
	}
	if !strings.Contains(err.Error(), "missing/repo") {
		t.Fatalf("error = %q; want unknown repo name", err.Error())
	}
}

func recordScopedOverviewRepos(scopedRepos *[]string) overviewStub {
	return func(
		_ context.Context,
		cfg *fleet.Config,
		_ fleet.OverviewOpts,
	) (*fleet.OverviewResult, []fleet.Diagnostic, error) {
		for repo := range cfg.Repos {
			*scopedRepos = append(*scopedRepos, repo)
		}
		return &fleet.OverviewResult{LoadedFrom: cfg.LoadedFrom, Window: "trailing-7d"}, nil, nil
	}
}

func markOverviewCalled(called *bool) overviewStub {
	return func(
		context.Context,
		*fleet.Config,
		fleet.OverviewOpts,
	) (*fleet.OverviewResult, []fleet.Diagnostic, error) {
		*called = true
		return nil, nil, nil
	}
}

func TestOverviewCmdExitCode(t *testing.T) {
	cases := []struct {
		name    string
		total   fleet.OverviewTotal
		wantErr bool
	}{
		{name: "aligned with failing runs", total: fleet.OverviewTotal{Aligned: 1, Runs: 3, Failures: 2}},
		{name: "drifted", total: fleet.OverviewTotal{Drifted: 1}, wantErr: true},
		{name: "errored", total: fleet.OverviewTotal{Errored: 1}, wantErr: true},
		{name: "aligned all passing", total: fleet.OverviewTotal{Aligned: 1, Runs: 3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := overviewExitCode(&fleet.OverviewResult{Total: tc.total})
			if tc.wantErr {
				if err == nil {
					t.Fatal("overviewExitCode returned nil; want drift error")
				}
				if code, ok := ExitCodeForError(err); !ok || code != 1 {
					t.Fatalf("exit code = %d/%v; want 1/true", code, ok)
				}
				if !SuppressErrorLog(err) {
					t.Fatalf("expected overview drift error to suppress fatal logging")
				}
				return
			}
			if err != nil {
				t.Fatalf("overviewExitCode returned %v; want nil", err)
			}
		})
	}
}

func TestOverviewCmdJSON(t *testing.T) {
	dir := writeOverviewFleetConfig(t, []string{"acme/api"})
	withOverviewStub(t, overviewJSONStub)

	stdout, stderr, err := executeOverviewRoot("--output", "json", "--dir", dir, "overview")
	if err != nil {
		t.Fatalf("Execute: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var env Envelope
	if decErr := json.Unmarshal([]byte(stdout), &env); decErr != nil {
		t.Fatalf("decode envelope: %v\nstdout=%s", decErr, stdout)
	}
	if env.SchemaVersion != SchemaVersion || env.Command != commandOverview || env.Apply {
		t.Fatalf("envelope header = version %d command %q apply %v", env.SchemaVersion, env.Command, env.Apply)
	}
	if len(env.Hints) != 1 || env.Hints[0].Code != fleet.DiagRepoInaccessible {
		t.Fatalf("hints = %#v; want repo_inaccessible", env.Hints)
	}
	raw := map[string]any{}
	if decErr := json.Unmarshal([]byte(stdout), &raw); decErr != nil {
		t.Fatalf("decode raw: %v", decErr)
	}
	result := raw["result"].(map[string]any)
	repos := result["repos"].([]any)
	repo := repos[0].(map[string]any)
	if _, ok := repo["aic"]; ok {
		t.Fatalf("repo aic key present for failed-only row: %#v", repo)
	}
	if _, ok := repo["cost"]; ok {
		t.Fatalf("repo cost key present for failed-only row: %#v", repo)
	}
}

func TestOverviewCmdJSONModelSupport(t *testing.T) {
	dir := writeOverviewFleetConfig(t, []string{"a/affected", "b/unknown", "c/undetermined", "d/clean"})
	withOverviewStub(t, func(
		_ context.Context,
		cfg *fleet.Config,
		_ fleet.OverviewOpts,
	) (*fleet.OverviewResult, []fleet.Diagnostic, error) {
		return &fleet.OverviewResult{
			LoadedFrom: cfg.LoadedFrom,
			Window:     "trailing-7d",
			Repos: []fleet.RepoOverview{
				{
					Repo: "a/affected", DriftState: fleet.DriftStateAligned, RunsAvailable: true,
					ModelSupport: &fleet.ModelSupport{
						Status:          "affected",
						OffendingModels: []string{"claude-sonnet-4.6"},
					},
				},
				{
					Repo: "b/unknown", DriftState: fleet.DriftStateAligned, RunsAvailable: true,
					ModelSupport: &fleet.ModelSupport{Status: "affected"},
				},
				{
					Repo: "c/undetermined", DriftState: fleet.DriftStateAligned, RunsAvailable: true,
					ModelSupport: &fleet.ModelSupport{Status: "undetermined"},
				},
				{
					Repo:          "d/clean",
					DriftState:    fleet.DriftStateAligned,
					Runs:          5,
					HealthRate:    f64ptr(1),
					RunsAvailable: true,
				},
			},
			Total: fleet.OverviewTotal{Aligned: 4, Runs: 5, HealthRate: f64ptr(1)},
		}, nil, nil
	})

	stdout, stderr, err := executeOverviewRoot("--output", "json", "--dir", dir, "overview")
	if err != nil {
		t.Fatalf("Execute: %v\nstderr=%s", err, stderr)
	}

	var env Envelope
	if decErr := json.Unmarshal([]byte(stdout), &env); decErr != nil {
		t.Fatalf("decode envelope: %v\nstdout=%s", decErr, stdout)
	}
	if env.SchemaVersion != SchemaVersion || SchemaVersion != 1 {
		t.Fatalf("schema_version = %d (const %d); want 1 (unchanged, FR-006)", env.SchemaVersion, SchemaVersion)
	}

	raw := map[string]any{}
	if decErr := json.Unmarshal([]byte(stdout), &raw); decErr != nil {
		t.Fatalf("decode raw: %v", decErr)
	}
	result := raw["result"].(map[string]any)
	byRepo := map[string]map[string]any{}
	for _, r := range result["repos"].([]any) {
		m := r.(map[string]any)
		byRepo[m["repo"].(string)] = m
	}

	affected := byRepo["a/affected"]["model_support"].(map[string]any)
	if affected["status"] != "affected" {
		t.Fatalf("a/affected status = %v; want affected", affected["status"])
	}
	models := affected["offending_models"].([]any)
	if len(models) != 1 || models[0] != "claude-sonnet-4.6" {
		t.Fatalf("a/affected offending_models = %v; want [claude-sonnet-4.6]", affected["offending_models"])
	}

	unknown := byRepo["b/unknown"]["model_support"].(map[string]any)
	if unknown["status"] != "affected" {
		t.Fatalf("b/unknown status = %v; want affected", unknown["status"])
	}
	if _, ok := unknown["offending_models"]; ok {
		t.Fatalf("b/unknown offending_models present; want omitted: %#v", unknown)
	}

	undetermined := byRepo["c/undetermined"]["model_support"].(map[string]any)
	if undetermined["status"] != "undetermined" {
		t.Fatalf("c/undetermined status = %v; want undetermined", undetermined["status"])
	}
	if _, ok := undetermined["offending_models"]; ok {
		t.Fatalf("c/undetermined offending_models present; want omitted: %#v", undetermined)
	}

	if _, ok := byRepo["d/clean"]["model_support"]; ok {
		t.Fatalf("d/clean carries model_support; want omitted entirely (SC-003): %#v", byRepo["d/clean"])
	}
	if _, ok := result["total"].(map[string]any)["model_support"]; ok {
		t.Fatalf("total carries model_support; want no pooled model counter (FR-007)")
	}
}

func TestOverviewCmdJSONSetupFailurePreservesTypedDiagnostic(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code string
	}{
		{
			name: "gh_aw_too_old",
			err: &fleet.DiagnosticError{
				Code:    fleet.DiagGhAwTooOld,
				Message: "Local `gh aw` version is too old for logs source (detected v0.77.5; minimum v0.79.2).",
				Fields: map[string]any{
					"detected_version": "v0.77.5",
					"minimum_version":  "v0.79.2",
				},
				Cause: errors.New("gh aw is too old for logs source"),
			},
			code: fleet.DiagGhAwTooOld,
		},
		{
			name: "gh_aw_missing",
			err: &fleet.DiagnosticError{
				Code:    fleet.DiagGhAwMissing,
				Message: "gh aw --version probe failed for logs source.",
				Fields:  map[string]any{"minimum_version": "v0.79.2"},
				Cause:   errors.New(`exec: "gh": executable file not found in $PATH`),
			},
			code: fleet.DiagGhAwMissing,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeOverviewFleetConfig(t, []string{"acme/api"})
			withOverviewStub(t, func(
				context.Context,
				*fleet.Config,
				fleet.OverviewOpts,
			) (*fleet.OverviewResult, []fleet.Diagnostic, error) {
				return nil, nil, tc.err
			})

			stdout, stderr, err := executeOverviewRoot("--output", "json", "--dir", dir, "overview")
			if err == nil {
				t.Fatalf("Execute: nil error; want setup failure\nstdout=%s\nstderr=%s", stdout, stderr)
			}
			var env Envelope
			if decErr := json.Unmarshal([]byte(stdout), &env); decErr != nil {
				t.Fatalf("decode envelope: %v\nstdout=%s", decErr, stdout)
			}
			if env.Result != nil {
				t.Fatalf("result = %#v; want nil", env.Result)
			}
			if len(env.Hints) != 1 || env.Hints[0].Code != tc.code {
				t.Fatalf("hints = %#v; want one %s diagnostic", env.Hints, tc.code)
			}
			if env.Hints[0].Code == fleet.DiagHint {
				t.Fatalf("hint code = %q; want typed setup diagnostic", env.Hints[0].Code)
			}
		})
	}
}

func TestSplitOverviewDiagsRoutesBillingRunFailuresToHints(t *testing.T) {
	diags := []fleet.Diagnostic{
		{
			Code:    fleet.DiagBillingQuotaExceeded,
			Message: "billing cap reached",
			Fields:  map[string]any{"repo": "acme/api", "signal": "runs"},
		},
		{
			Code:    fleet.DiagDriftDetected,
			Message: "drift",
		},
	}

	warnings, hints := splitOverviewDiags(diags)
	if len(hints) != 1 || hints[0].Code != fleet.DiagBillingQuotaExceeded {
		t.Fatalf("hints = %#v; want billing_quota_exceeded routed as a hint", hints)
	}
	if len(warnings) != 1 || warnings[0].Code != fleet.DiagDriftDetected {
		t.Fatalf("warnings = %#v; want drift_detected left as a warning", warnings)
	}
}

func TestSplitOverviewDiagsRoutesModelRemediationToHints(t *testing.T) {
	msg := "agentic runs are failing on an unsupported/deprecated Copilot model. " +
		"Set the GH_AW_DEFAULT_MODEL_COPILOT Actions variable on the repo."
	diags := []fleet.Diagnostic{
		{
			Code:    fleet.DiagHint,
			Message: msg,
			Fields:  map[string]any{"repo": "rshade/finfocus", diagnosticFieldHint: msg},
		},
		{
			Code:    fleet.DiagRepoInaccessible,
			Message: "model-support signal unavailable for acme/api",
			Fields:  map[string]any{"repo": "acme/api", "signal": "model"},
		},
	}

	warnings, hints := splitOverviewDiags(diags)
	if len(hints) != 2 {
		t.Fatalf("hints = %#v; want the remediation and the undetermined read-error both routed to hints", hints)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v; want none", warnings)
	}
	if hints[0].Fields["repo"] != "acme/api" || hints[1].Fields["repo"] != "rshade/finfocus" {
		t.Fatalf("hints not sorted by repo: %#v", hints)
	}
	if !strings.Contains(hints[1].Message, "GH_AW_DEFAULT_MODEL_COPILOT") {
		t.Fatalf("remediation hint does not name the fix variable: %q", hints[1].Message)
	}
}

func TestSplitOverviewDiagsHealthyFleetEmitsNoModelHint(t *testing.T) {
	// A healthy fleet's overview produces no model diagnostics at all (FR-008).
	warnings, hints := splitOverviewDiags(nil)
	if len(warnings) != 0 || len(hints) != 0 {
		t.Fatalf("healthy fleet split = warnings %d hints %d; want 0/0", len(warnings), len(hints))
	}
}

func overviewJSONStub(
	_ context.Context,
	cfg *fleet.Config,
	_ fleet.OverviewOpts,
) (*fleet.OverviewResult, []fleet.Diagnostic, error) {
	return &fleet.OverviewResult{
			LoadedFrom: cfg.LoadedFrom,
			Window:     "trailing-7d",
			Repos: []fleet.RepoOverview{{
				Repo:          "acme/api",
				DriftState:    fleet.DriftStateAligned,
				Runs:          9,
				Failures:      9,
				NoOps:         0,
				HealthRate:    f64ptr(0),
				RunsAvailable: true,
			}},
			Total: fleet.OverviewTotal{
				Aligned:    1,
				Runs:       9,
				Failures:   9,
				NoOps:      0,
				HealthRate: f64ptr(0),
			},
		},
		[]fleet.Diagnostic{{
			Code:    fleet.DiagRepoInaccessible,
			Message: "runs unavailable",
			Fields:  map[string]any{"repo": "acme/ghost", "signal": "runs"},
		}}, nil
}

func overviewCmdResult() *fleet.OverviewResult {
	return &fleet.OverviewResult{
		LoadedFrom: "fleet.json",
		Window:     "trailing-7d",
		Repos: []fleet.RepoOverview{
			{
				Repo:          "rshade/finfocus",
				DriftState:    fleet.DriftStateAligned,
				Runs:          42,
				Failures:      2,
				NoOps:         36,
				HealthRate:    f64ptr(0.952),
				AIC:           f64ptr(118.40),
				Cost:          f64ptr(1.18),
				RunsAvailable: true,
			},
			{
				Repo:          "acme/api",
				DriftState:    fleet.DriftStateAligned,
				Runs:          9,
				Failures:      9,
				NoOps:         0,
				HealthRate:    f64ptr(0),
				RunsAvailable: true,
			},
			{
				Repo:       "acme/widgets",
				DriftState: fleet.DriftStateDrifted,
				DriftDetail: &fleet.RepoStatus{
					Repo:       "acme/widgets",
					DriftState: fleet.DriftStateDrifted,
					Drifted: []fleet.WorkflowDrift{{
						Name:       "ci-doctor",
						DesiredRef: "v0.79.2",
						ActualRef:  "v0.78.0",
					}},
				},
				Runs:          18,
				Failures:      0,
				NoOps:         4,
				HealthRate:    f64ptr(1),
				AIC:           f64ptr(47.10),
				Cost:          f64ptr(0.47),
				RunsAvailable: true,
			},
			{
				Repo:       "acme/ghost",
				DriftState: fleet.DriftStateErrored,
				DriftDetail: &fleet.RepoStatus{
					Repo:         "acme/ghost",
					DriftState:   fleet.DriftStateErrored,
					ErrorMessage: "HTTP 404",
				},
				RunsAvailable: false,
				RunsError:     "HTTP 404",
			},
		},
		Total: fleet.OverviewTotal{
			Runs:       69,
			Failures:   11,
			NoOps:      40,
			HealthRate: f64ptr(0.841),
			AIC:        f64ptr(165.50),
			Cost:       f64ptr(1.65),
			Aligned:    2,
			Drifted:    1,
			Errored:    1,
		},
	}
}

func withOverviewStub(
	t *testing.T,
	stub overviewStub,
) {
	t.Helper()
	prev := runOverview
	t.Cleanup(func() { runOverview = prev })
	runOverview = stub
}

func executeOverviewRoot(args ...string) (string, string, error) {
	root := NewRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func writeOverviewFleetConfig(t *testing.T, repos []string) string {
	t.Helper()
	dir := t.TempDir()
	var b strings.Builder
	b.WriteString(`{"version":1,"profiles":{"default":{"sources":{},"workflows":[]}},"repos":{`)
	for i, repo := range repos {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Quote(repo))
		b.WriteString(`:{"profiles":["default"]}`)
	}
	b.WriteString("}}\n")
	if err := os.WriteFile(filepath.Join(dir, "fleet.json"), []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write fleet.json: %v", err)
	}
	return dir
}
