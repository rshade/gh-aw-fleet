package fleet

import (
	"context"
	"errors"
	"math"
	"strings"
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

	withModelStubs(t, nil, nil)
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

// withModelStubs owns the ghFailureIssuesAPI / ghRepoVariablesAPI seams for
// the duration of a test. Nil handlers default to offline no-ops (no issues,
// no variables) so Overview tests that only exercise other signals can stub
// the model collector out with withModelStubs(t, nil, nil).
func withModelStubs(
	t *testing.T,
	issues func(repo string) ([]failureIssue, error),
	vars func(repo string) (map[string]string, error),
) {
	t.Helper()
	if issues == nil {
		issues = func(string) ([]failureIssue, error) { return nil, nil }
	}
	if vars == nil {
		vars = func(string) (map[string]string, error) { return map[string]string{}, nil }
	}
	prevI, prevV := ghFailureIssuesAPI, ghRepoVariablesAPI
	t.Cleanup(func() { ghFailureIssuesAPI, ghRepoVariablesAPI = prevI, prevV })
	ghFailureIssuesAPI = func(_ context.Context, repo string) ([]failureIssue, error) { return issues(repo) }
	ghRepoVariablesAPI = func(_ context.Context, repo string) (map[string]string, error) { return vars(repo) }
}

func modelFailureIssues(body string) []failureIssue {
	return []failureIssue{{Number: 1, Title: "[aw] Code Simplifier failed", Body: body}}
}

func findRepoDiag(diags []Diagnostic, repo string) (Diagnostic, bool) {
	for _, d := range diags {
		if r, _ := d.Fields[fieldRepo].(string); r == repo {
			return d, true
		}
	}
	return Diagnostic{}, false
}

func TestCollectOverviewModelHealth(t *testing.T) {
	modelBody := mustReadFailureFixture(t, "model_not_supported.txt")
	unrelatedBody := mustReadFailureFixture(t, "unrelated.txt")

	repos := []string{"e/read-error", "h/healthy", "m/override", "m/unset", "u/unrelated", "v/var-error"}
	withModelStubs(t,
		func(repo string) ([]failureIssue, error) {
			switch repo {
			case "e/read-error":
				return nil, errors.New("gh api: HTTP 500 while listing issues")
			case "h/healthy":
				return nil, nil
			case "u/unrelated":
				return modelFailureIssues(unrelatedBody), nil
			default: // m/override, m/unset, v/var-error
				return modelFailureIssues(modelBody), nil
			}
		},
		func(repo string) (map[string]string, error) {
			switch repo {
			case "m/override":
				return map[string]string{varDefaultModelCopilot: "gpt-5-mini"}, nil
			case "v/var-error":
				return nil, errors.New("gh api: HTTP 403 while listing variables")
			default: // m/unset
				return map[string]string{}, nil
			}
		})

	rows, diags := collectOverviewModelHealth(context.Background(), repos)

	assertModelRow(t, rows, "m/unset", ModelSupportAffected, ghAwBakedDefaultCopilotModel)
	assertModelRow(t, rows, "m/override", ModelSupportAffected, "gpt-5-mini")
	assertModelRow(t, rows, "v/var-error", ModelSupportAffected, "")
	assertModelRow(t, rows, "e/read-error", ModelSupportUndetermined, "")
	assertModelRow(t, rows, "u/unrelated", "", "")
	assertModelRow(t, rows, "h/healthy", "", "")

	diag, ok := findRepoDiag(diags, "e/read-error")
	if !ok {
		t.Fatalf("no diagnostic for undetermined repo; diags=%#v", diags)
	}
	if diag.Code == DiagHint {
		t.Fatalf("undetermined diag Code = %q; want a typed repo-error code, not DiagHint", diag.Code)
	}
	if sig, _ := diag.Fields[fieldSignal].(string); sig != overviewSignalModel {
		t.Fatalf("undetermined diag signal = %q; want %q", sig, overviewSignalModel)
	}
	if _, hasClean := findRepoDiag(filterDiags(diags, "h/healthy"), "h/healthy"); hasClean {
		t.Fatalf("clean repo produced a diagnostic; diags=%#v", diags)
	}
}

func TestCollectOverviewModelHealthRemediationHints(t *testing.T) {
	modelBody := mustReadFailureFixture(t, "model_not_supported.txt")
	repos := []string{"a/affected", "b/affected", "c/clean", "u/undetermined"}
	withModelStubs(t,
		func(repo string) ([]failureIssue, error) {
			switch repo {
			case "a/affected", "b/affected":
				return modelFailureIssues(modelBody), nil
			case "u/undetermined":
				return nil, errors.New("gh api: HTTP 500 while listing issues")
			default:
				return nil, nil
			}
		},
		func(_ string) (map[string]string, error) { return map[string]string{}, nil })

	_, diags := collectOverviewModelHealth(context.Background(), repos)

	hintRepos := map[string]int{}
	for _, d := range diags {
		if d.Code != DiagHint {
			continue
		}
		repo, _ := d.Fields[fieldRepo].(string)
		hintRepos[repo]++
		if _, ok := d.Fields[fieldHint]; !ok {
			t.Fatalf("remediation hint for %s missing %q field: %#v", repo, fieldHint, d.Fields)
		}
		if !strings.Contains(d.Message, varDefaultModelCopilot) {
			t.Fatalf("remediation hint for %s does not name %s: %q", repo, varDefaultModelCopilot, d.Message)
		}
	}
	if hintRepos["a/affected"] != 1 || hintRepos["b/affected"] != 1 {
		t.Fatalf("affected repos = %v; want exactly one hint each", hintRepos)
	}
	if hintRepos["c/clean"] != 0 || hintRepos["u/undetermined"] != 0 {
		t.Fatalf("non-affected repos produced hints: %v", hintRepos)
	}
}

func filterDiags(diags []Diagnostic, repo string) []Diagnostic {
	out := []Diagnostic{}
	for _, d := range diags {
		if r, _ := d.Fields[fieldRepo].(string); r == repo {
			out = append(out, d)
		}
	}
	return out
}

func assertModelRow(t *testing.T, rows map[string]overviewModelHealth, repo, wantStatus, wantModel string) {
	t.Helper()
	row := rows[repo]
	if row.status != wantStatus {
		t.Fatalf("%s status = %q; want %q", repo, row.status, wantStatus)
	}
	if row.offendingModel != wantModel {
		t.Fatalf("%s offending model = %q; want %q", repo, row.offendingModel, wantModel)
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
