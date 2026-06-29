package fleet

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func TestOverviewClassifyConclusion(t *testing.T) {
	cases := []struct {
		conclusion string
		want       runOutcome
	}{
		{conclusion: conclusionSuccess, want: outcomeSuccess},
		{conclusion: conclusionFailure, want: outcomeFailure},
		{conclusion: conclusionTimedOut, want: outcomeFailure},
		{conclusion: conclusionStartupFailure, want: outcomeFailure},
		{conclusion: conclusionCancelled, want: outcomeIgnored},
		{conclusion: conclusionSkipped, want: outcomeIgnored},
		{conclusion: "", want: outcomeIgnored},
		{conclusion: "new_terminal_state", want: outcomeFailure},
	}
	for _, tc := range cases {
		t.Run(tc.conclusion, func(t *testing.T) {
			if got := classifyConclusion(tc.conclusion); got != tc.want {
				t.Fatalf("classifyConclusion(%q) = %v; want %v", tc.conclusion, got, tc.want)
			}
		})
	}
}

func TestOverviewReducer(t *testing.T) {
	aic := 12.5
	row := reduceOverviewHealth([]repoRunData{{
		Workflow: "Audit",
		Runs: []logsRun{
			{Conclusion: conclusionSuccess, AIC: &aic},
			{Conclusion: conclusionFailure},
			{Conclusion: conclusionTimedOut},
			{Conclusion: conclusionCancelled},
		},
	}})
	if row.runs != 3 || row.failures != 2 {
		t.Fatalf("runs/failures = %d/%d; want 3/2", row.runs, row.failures)
	}
	if row.healthRate == nil || math.Abs(*row.healthRate-(1.0/3.0)) > 0.0001 {
		t.Fatalf("health = %v; want 1/3", row.healthRate)
	}
	if row.aic == nil || *row.aic != aic {
		t.Fatalf("AIC = %v; want %v", row.aic, aic)
	}
	if row.cost == nil || *row.cost != aic*aicToUSDRate {
		t.Fatalf("Cost = %v; want %v", row.cost, aic*aicToUSDRate)
	}

	failedOnly := reduceOverviewHealth([]repoRunData{{
		Workflow: "Audit",
		Runs: []logsRun{
			{Conclusion: conclusionFailure},
			{Conclusion: conclusionStartupFailure},
		},
	}})
	if failedOnly.runs != 2 || failedOnly.failures != 2 {
		t.Fatalf("failed-only runs/failures = %d/%d; want 2/2", failedOnly.runs, failedOnly.failures)
	}
	if failedOnly.healthRate == nil || *failedOnly.healthRate != 0 {
		t.Fatalf("failed-only health = %v; want 0", failedOnly.healthRate)
	}
	if failedOnly.aic != nil || failedOnly.cost != nil {
		t.Fatalf("failed-only AIC/Cost = %v/%v; want nil/nil", failedOnly.aic, failedOnly.cost)
	}

	zero := reduceOverviewHealth([]repoRunData{{
		Workflow: "Audit",
		Runs: []logsRun{
			{Conclusion: conclusionCancelled},
			{Conclusion: conclusionSkipped},
		},
	}})
	if zero.runs != 0 || zero.healthRate != nil || zero.aic != nil || zero.cost != nil {
		t.Fatalf("zero-counting row = %+v; want runs 0 and nil health/AIC/cost", zero)
	}
}

func TestOverviewJoin(t *testing.T) {
	cfg := overviewTestConfig([]string{
		"a/aligned",
		"b/drifted",
		"c/drift-error",
		"d/runs-limited",
	})
	fetcher := newRecordingFetcher()
	fetcher.listings = map[string][]string{
		"a/aligned":      {"audit.md"},
		"b/drifted":      {"audit.md"},
		"d/runs-limited": {"audit.md"},
	}
	fetcher.bodies = map[string]map[string]string{
		"a/aligned": {
			"audit.md": frontmatterDoc("source: githubnext/agentics/audit@v1.0"),
		},
		"b/drifted": {
			"audit.md": frontmatterDoc("source: githubnext/agentics/audit@v0.9"),
		},
		"d/runs-limited": {
			"audit.md": frontmatterDoc("source: githubnext/agentics/audit@v1.0"),
		},
	}
	fetcher.listErr = map[string]error{
		"c/drift-error": errors.New("HTTP 404"),
	}

	withLogsStubs(t,
		func(repo string) ([]workflowRef, error) {
			if repo == "d/runs-limited" {
				return nil, errors.New("gh api: API rate limit exceeded")
			}
			return []workflowRef{{Name: "Audit", Path: ".github/workflows/audit.lock.yml"}}, nil
		},
		func(repo, _ string, _ FetchMode) (logsPayload, error) {
			switch repo {
			case "b/drifted":
				return logsPayload{Runs: []logsRun{{Conclusion: conclusionFailure, CreatedAt: overviewTestTime()}}}, nil
			default:
				aic := 5.0
				return logsPayload{Runs: []logsRun{{
					Conclusion: conclusionSuccess,
					AIC:        &aic,
					CreatedAt:  overviewTestTime(),
				}}}, nil
			}
		})

	res, diags, err := Overview(context.Background(), cfg, OverviewOpts{
		Mode:    FetchMode{Kind: FetchTrailing, Days: 1000},
		fetcher: fetcher,
	})
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if len(res.Repos) != 4 {
		t.Fatalf("len(repos) = %d; want 4", len(res.Repos))
	}
	byRepo := map[string]RepoOverview{}
	for _, row := range res.Repos {
		byRepo[row.Repo] = row
	}
	if byRepo["c/drift-error"].DriftState != DriftStateErrored || byRepo["c/drift-error"].Runs != 1 {
		t.Fatalf("drift-error row = %+v; want errored drift with fetched health", byRepo["c/drift-error"])
	}
	if byRepo["d/runs-limited"].DriftState != DriftStateAligned || byRepo["d/runs-limited"].RunsAvailable {
		t.Fatalf("runs-limited row = %+v; want aligned drift with unavailable runs", byRepo["d/runs-limited"])
	}
	if res.Total.Aligned != 2 || res.Total.Drifted != 1 || res.Total.Errored != 1 {
		t.Fatalf("total drift counts = %+v; want aligned=2 drifted=1 errored=1", res.Total)
	}
	if res.Total.Runs != 3 || res.Total.Failures != 1 {
		t.Fatalf("total runs/failures = %d/%d; want 3/1", res.Total.Runs, res.Total.Failures)
	}
	if !hasSignalDiag(diags, DiagRateLimited, "d/runs-limited", "runs") {
		t.Fatalf("diags = %#v; want rate_limited signal=runs for d/runs-limited", diags)
	}
	if !hasSignalDiag(diags, DiagRepoInaccessible, "c/drift-error", "drift") {
		t.Fatalf("diags = %#v; want repo_inaccessible signal=drift for c/drift-error", diags)
	}
}

func TestOverviewJoinEmptyFleet(t *testing.T) {
	cfg := &Config{LoadedFrom: "fleet.json", Repos: map[string]RepoSpec{}}
	res, diags, err := Overview(context.Background(), cfg, OverviewOpts{Mode: FetchMode{Kind: FetchTrailing, Days: 7}})
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if len(res.Repos) != 0 {
		t.Fatalf("len(repos) = %d; want 0", len(res.Repos))
	}
	if len(diags) != 1 || diags[0].Code != DiagEmptyFleet {
		t.Fatalf("diags = %#v; want one empty_fleet diagnostic", diags)
	}
}

func TestOverviewNoOp(t *testing.T) {
	mixedRuns := make([]logsRun, 0, 42)
	for range 40 {
		aic := 1.0
		mixedRuns = append(mixedRuns, logsRun{Conclusion: conclusionSuccess, AIC: &aic})
	}
	for range 2 {
		mixedRuns = append(mixedRuns, logsRun{Conclusion: conclusionFailure})
	}
	mixed := reduceOverviewHealth([]repoRunData{{
		Workflow: "Audit",
		Runs:     mixedRuns,
		MCPToolUsage: mcpToolUsage{Summary: []mcpToolSummary{{
			ServerName: mcpServerSafeOutputs,
			ToolName:   mcpToolNoop,
			CallCount:  36,
		}}},
	}})
	if mixed.runs != 42 || mixed.failures != 2 || mixed.noOps != 36 {
		t.Fatalf("mixed row = %+v; want runs=42 failures=2 noops=36", mixed)
	}
	if mixed.healthRate == nil || math.Round(*mixed.healthRate*100) != 95 {
		t.Fatalf("mixed health = %v; want rounded 95%%", mixed.healthRate)
	}
	if mixed.cost == nil {
		t.Fatalf("mixed cost nil; want populated")
	}

	allNoop := reduceOverviewHealth([]repoRunData{{
		Workflow: "Audit",
		Runs: []logsRun{
			{Conclusion: conclusionSuccess, AIC: f64(2)},
			{Conclusion: conclusionSuccess, AIC: f64(3)},
		},
		MCPToolUsage: mcpToolUsage{Summary: []mcpToolSummary{{
			ServerName: mcpServerSafeOutputs,
			ToolName:   mcpToolNoop,
			CallCount:  2,
		}}},
	}})
	if allNoop.runs != allNoop.noOps || allNoop.failures != 0 || allNoop.healthRate == nil || *allNoop.healthRate != 1 {
		t.Fatalf("all-noop row = %+v; want runs==noops, failures=0, health=1", allNoop)
	}

	clamped := reduceOverviewHealth([]repoRunData{{
		Workflow: "Audit",
		Runs: []logsRun{
			{Conclusion: conclusionSuccess},
			{Conclusion: conclusionSuccess},
		},
		MCPToolUsage: mcpToolUsage{Summary: []mcpToolSummary{{
			ServerName: mcpServerSafeOutputs,
			ToolName:   mcpToolNoop,
			CallCount:  99,
		}}},
	}})
	if clamped.noOps != 2 {
		t.Fatalf("clamped noops = %d; want 2", clamped.noOps)
	}
}

func TestOverviewPartialFanoutKeepsData(t *testing.T) {
	withLogsStubs(t,
		func(_ string) ([]workflowRef, error) {
			return []workflowRef{
				{Name: "Audit", Path: ".github/workflows/audit.lock.yml"},
				{Name: "Scan", Path: ".github/workflows/scan.lock.yml"},
			}, nil
		},
		func(_, workflow string, _ FetchMode) (logsPayload, error) {
			if workflow == "Scan" {
				return logsPayload{}, errors.New("gh api: API rate limit exceeded")
			}
			aic := 5.0
			return logsPayload{Runs: []logsRun{{
				Conclusion: conclusionSuccess,
				AIC:        &aic,
				CreatedAt:  overviewTestTime(),
			}}}, nil
		})

	rows, diags, err := collectOverviewHealth(
		context.Background(),
		[]string{"p/partial"},
		FetchMode{Kind: FetchTrailing, Days: 1000},
	)
	if err != nil {
		t.Fatalf("collectOverviewHealth: %v", err)
	}
	row := rows["p/partial"]
	if !row.available {
		t.Fatalf("row = %+v; want available with the surviving workflow's data", row)
	}
	if row.runs != 1 || row.failures != 0 {
		t.Fatalf("row runs/failures = %d/%d; want 1/0 from the successful workflow", row.runs, row.failures)
	}
	if row.aic == nil || *row.aic != 5.0 {
		t.Fatalf("row AIC = %v; want 5.0 from the successful workflow", row.aic)
	}
	if !hasSignalDiag(diags, DiagRateLimited, "p/partial", "runs") {
		t.Fatalf("diags = %#v; want rate_limited signal=runs for the failed workflow", diags)
	}
}

func TestOverviewRunFanoutFailuresPreserveTypedDiagnostics(t *testing.T) {
	cases := []struct {
		name        string
		repo        string
		workflowErr error
		logsErr     error
		wantCode    string
	}{
		{
			name:        "discovery billing failure",
			repo:        "p/billing",
			workflowErr: errors.New("gh api: HTTP 402: spending limit reached"),
			wantCode:    DiagBillingQuotaExceeded,
		},
		{
			name:     "logs network failure",
			repo:     "p/network",
			logsErr:  errors.New("gh api: Could not resolve host: api.github.com"),
			wantCode: DiagNetworkUnreachable,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withLogsStubs(t,
				func(_ string) ([]workflowRef, error) {
					if tc.workflowErr != nil {
						return nil, tc.workflowErr
					}
					return []workflowRef{{Name: "Audit", Path: ".github/workflows/audit.lock.yml"}}, nil
				},
				func(_, _ string, _ FetchMode) (logsPayload, error) {
					return logsPayload{}, tc.logsErr
				})

			rows, diags, err := collectOverviewHealth(
				context.Background(),
				[]string{tc.repo},
				FetchMode{Kind: FetchTrailing, Days: 1000},
			)
			if err != nil {
				t.Fatalf("collectOverviewHealth: %v", err)
			}
			row := rows[tc.repo]
			if row.available || row.errMessage == "" {
				t.Fatalf("row = %+v; want unavailable runs with original error detail", row)
			}
			if !hasSignalDiag(diags, tc.wantCode, tc.repo, "runs") {
				t.Fatalf("diags = %#v; want %s signal=runs for %s", diags, tc.wantCode, tc.repo)
			}
			if tc.wantCode != DiagRepoInaccessible && hasSignalDiag(diags, DiagRepoInaccessible, tc.repo, "runs") {
				t.Fatalf("diags = %#v; typed run failure was collapsed to repo_inaccessible", diags)
			}
		})
	}
}

func overviewTestConfig(repos []string) *Config {
	cfg := &Config{
		LoadedFrom: "fleet.json",
		Profiles: map[string]Profile{
			"default": {
				Sources: map[string]SourcePin{
					"githubnext/agentics": {Ref: "v1.0"},
				},
				Workflows: []ProfileWorkflow{{Name: "audit", Source: "githubnext/agentics"}},
			},
		},
		Repos: map[string]RepoSpec{},
	}
	for _, repo := range repos {
		cfg.Repos[repo] = RepoSpec{Profiles: []string{"default"}}
	}
	return cfg
}

func overviewTestTime() time.Time {
	return time.Now().UTC().Add(-24 * time.Hour)
}

func hasSignalDiag(diags []Diagnostic, code, repo, signal string) bool {
	for _, diag := range diags {
		if diag.Code == code &&
			diag.Fields[fieldRepo] == repo &&
			diag.Fields["signal"] == signal {
			return true
		}
	}
	return false
}

func f64(v float64) *float64 { return &v }
