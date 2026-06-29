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
