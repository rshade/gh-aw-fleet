package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SourceKind selects which upstream surface `gh-aw-fleet consumption` reads.
type SourceKind int

const (
	// SourceLogs reads AI-credit usage from `gh aw logs --json` per agentic
	// workflow (the default; issue #103). Needs no deployed report workflow.
	SourceLogs SourceKind = iota
	// SourceArtifacts reads the legacy path: discover each repo's
	// api-consumption-report discussions, then decode the run's aw_info.json /
	// run_summary.json artifacts. Retained as a transitional fallback.
	SourceArtifacts
)

// sourceNameLogs and sourceNameArtifacts are the canonical --source CLI values.
const (
	sourceNameLogs      = "logs"
	sourceNameArtifacts = "artifacts"
)

// sourceNames maps SourceKind to its CLI vocabulary, indexed by kind.
//
//nolint:gochecknoglobals // immutable lookup table; Go has no const arrays.
var sourceNames = [...]string{
	SourceLogs:      sourceNameLogs,
	SourceArtifacts: sourceNameArtifacts,
}

// String returns the CLI-vocabulary name for the source, or "" when out of range.
func (s SourceKind) String() string {
	if int(s) < 0 || int(s) >= len(sourceNames) {
		return ""
	}
	return sourceNames[s]
}

// ParseSource returns the SourceKind for the canonical name s, or an error
// naming the valid sources when s is outside the closed set.
func ParseSource(s string) (SourceKind, error) {
	for k, name := range sourceNames {
		if name == s {
			return SourceKind(k), nil
		}
	}
	return 0, fmt.Errorf("invalid --source value %q: expected one of logs, artifacts", s)
}

// workflowRef is one agentic workflow discovered via the Actions API. Name is
// the GitHub Actions display name (e.g. "Daily Malicious Code Scan Agent") —
// the only form `gh aw logs <WORKFLOW>` accepts; the fleet's slug
// (daily-malicious-code-scan) is rejected. Path is the compiled lock file.
type workflowRef struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	State string `json:"state"`
}

// logsSummary is the subset of `gh aw logs --json` .summary consumed. TotalAIC
// is absent (nil) on an all-failure capture; GitHub does not emit a USD cost
// field under the Copilot AI-Credits model.
type logsSummary struct {
	TotalRuns           int      `json:"total_runs"`
	TotalAIC            *float64 `json:"total_aic"`
	TotalGitHubAPICalls int      `json:"total_github_api_calls"`
	TotalSafeItems      int      `json:"total_safe_items"`
}

// logsRun is the subset of `gh aw logs --json` .runs[] consumed. AIC is absent
// (nil) on failed / non-"normal" runs.
type logsRun struct {
	RunID          int64     `json:"run_id"`
	WorkflowName   string    `json:"workflow_name"`
	Conclusion     string    `json:"conclusion"`
	AIC            *float64  `json:"aic"`
	GitHubAPICalls int       `json:"github_api_calls"`
	SafeItemsCount int       `json:"safe_items_count"`
	ActionMinutes  int       `json:"action_minutes"`
	CreatedAt      time.Time `json:"created_at"`
}

// logsPayload bundles the decoded `gh aw logs --json` output for one
// (repo, workflow) call.
type logsPayload struct {
	Summary logsSummary `json:"summary"`
	Runs    []logsRun   `json:"runs"`
}

// aicToUSDRate converts AI credits to USD: one credit is $0.01 (issue #103).
const aicToUSDRate = 0.01

// logsSourceMinVersion is the minimum gh-aw release whose logs JSON carries
// the AI-credit schema consumed by SourceLogs.
const logsSourceMinVersion = CompileStrictMinVersion

// secondsPerMinute converts the logs schema's integer action minutes to the
// AvgDurationS seconds basis used by WorkflowConsumption.
const secondsPerMinute = 60

// aicToUSD derives the USD cost from an AI-credit total, honoring the
// nil-until-positive convention. Returns nil when aic is nil or non-positive.
func aicToUSD(aic *float64) *float64 {
	if aic == nil {
		return nil
	}
	usd := *aic * aicToUSDRate
	return normalizeCost(&usd)
}

// ensureLogsSourceGhAwVersion rejects gh-aw releases that predate the logs JSON
// AI-credit schema. Without this gate older CLIs decode successfully while
// leaving AIC/COST nil, which looks like valid empty spend.
func ensureLogsSourceGhAwVersion(ctx context.Context) error {
	detected, err := ghAwVersion(ctx)
	if err != nil {
		return fmt.Errorf("gh aw --version probe failed for --source logs: %w. "+
			"Install gh-aw %s or newer, or rerun with --source artifacts",
			err, logsSourceMinVersion)
	}
	if detected == "" {
		return fmt.Errorf("gh aw --version did not report a vMAJOR.MINOR.PATCH token; "+
			"--source logs requires gh-aw %s or newer. Rerun with --source artifacts to use the legacy path",
			logsSourceMinVersion)
	}
	cmp, cmpErr := compareVersionTokens(detected, logsSourceMinVersion)
	if cmpErr != nil {
		return fmt.Errorf("compare gh-aw version for --source logs: %w", cmpErr)
	}
	if cmp < 0 {
		return fmt.Errorf("gh aw is too old for --source logs: detected %s, minimum %s required. "+
			"Install with `gh extension install github/gh-aw --pin %s`, or rerun with --source artifacts",
			detected, logsSourceMinVersion, logsSourceMinVersion)
	}
	return nil
}

// compareVersionTokens compares vMAJOR.MINOR.PATCH tokens. Returns -1, 0, or 1
// when a is less than, equal to, or greater than b.
func compareVersionTokens(a, b string) (int, error) {
	ap, err := versionParts(a)
	if err != nil {
		return 0, err
	}
	bp, err := versionParts(b)
	if err != nil {
		return 0, err
	}
	for i := range ap {
		switch {
		case ap[i] < bp[i]:
			return -1, nil
		case ap[i] > bp[i]:
			return 1, nil
		}
	}
	return 0, nil
}

// versionParts parses a vMAJOR.MINOR.PATCH token into numeric components.
func versionParts(v string) ([3]int, error) {
	var out [3]int
	if !ghAwVersionRE.MatchString(v) || ghAwVersionRE.FindString(v) != v {
		return out, fmt.Errorf("invalid version token %q", v)
	}
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, fmt.Errorf("invalid version token %q: %w", v, err)
		}
		out[i] = n
	}
	return out, nil
}

// ghWorkflowsAPI lists a repo's agentic workflows (those compiled to a
// .lock.yml) via the Actions API. The returned Name is the display name
// `gh aw logs` matches on. Package-level seam so tests run offline.
//
//nolint:gochecknoglobals // test-injection seam for the Actions workflows API
var ghWorkflowsAPI = func(ctx context.Context, repo string) ([]workflowRef, error) {
	path := fmt.Sprintf("repos/%s/actions/workflows?per_page=100", repo)
	cmd := exec.CommandContext(ctx, "gh", "api", path)
	out, err := runLoggedOutput(cmd, "gh", "api", map[string]string{fieldPath: path})
	if err != nil {
		return nil, ghErr(err)
	}
	var payload struct {
		Workflows []workflowRef `json:"workflows"`
	}
	if decErr := json.Unmarshal(out, &payload); decErr != nil {
		return nil, fmt.Errorf("decode gh api workflows list: %w", decErr)
	}
	agentic := make([]workflowRef, 0, len(payload.Workflows))
	for _, w := range payload.Workflows {
		if strings.HasSuffix(w.Path, ".lock.yml") {
			agentic = append(agentic, w)
		}
	}
	return agentic, nil
}

// ghLogsAPI fetches `gh aw logs --json` for one (repo, workflow), scoped to the
// fetch-mode window. It runs in a throwaway temp dir because `gh aw logs`
// writes downloaded artifacts under ./.github/aw/logs. Package-level seam so
// tests run offline.
//
//nolint:gochecknoglobals // test-injection seam for `gh aw logs --json`
var ghLogsAPI = func(ctx context.Context, repo, workflow string, mode FetchMode) (logsPayload, error) {
	dir, err := os.MkdirTemp("", "gh-aw-fleet-logs-")
	if err != nil {
		return logsPayload{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	args := []string{"aw", logsToken, "--json", "--repo", repo}
	args = append(args, logsWindowArgs(mode)...)
	args = append(args, workflow)
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	out, err := runLoggedOutput(cmd, "gh", "aw logs", map[string]string{fieldRepo: repo})
	if err != nil {
		return logsPayload{}, ghErr(err)
	}
	var payload logsPayload
	if decErr := json.Unmarshal(out, &payload); decErr != nil {
		return logsPayload{}, fmt.Errorf("decode gh aw logs --json for %q: %w", workflow, decErr)
	}
	return payload, nil
}

// logsWindowArgs builds the `gh aw logs` run-window flags for the fetch mode.
// `gh aw logs` has no trailing/since flags; it uses --start-date (absolute or
// relative like -7d) plus -c (max runs). The client re-filters on created_at
// (filterRunsByWindow) so behavior is deterministic and fixture-testable.
func logsWindowArgs(mode FetchMode) []string {
	switch mode.Kind {
	case FetchTrailing:
		return []string{"--start-date", fmt.Sprintf("-%dd", mode.Days), "-c", "1000"}
	case FetchSince:
		return []string{"--start-date", mode.Since.Format("2006-01-02"), "-c", "1000"}
	case FetchLatest:
		return []string{"-c", "5"}
	}
	return []string{"-c", "5"}
}

// filterRunsByWindow keeps only the runs that fall inside the fetch-mode window.
// FetchLatest keeps the single newest run; the dated modes keep every run on or
// after the window start.
func filterRunsByWindow(runs []logsRun, mode FetchMode, now time.Time) []logsRun {
	switch mode.Kind {
	case FetchLatest:
		if len(runs) == 0 {
			return nil
		}
		newest := runs[0]
		for _, r := range runs[1:] {
			if r.CreatedAt.After(newest.CreatedAt) {
				newest = r
			}
		}
		return []logsRun{newest}
	case FetchTrailing:
		return runsOnOrAfter(runs, now.Add(-time.Duration(mode.Days)*24*time.Hour))
	case FetchSince:
		return runsOnOrAfter(runs, mode.Since)
	}
	return runs
}

// runsOnOrAfter returns the runs created at or after cutoff.
func runsOnOrAfter(runs []logsRun, cutoff time.Time) []logsRun {
	out := make([]logsRun, 0, len(runs))
	for _, r := range runs {
		if !r.CreatedAt.Before(cutoff) {
			out = append(out, r)
		}
	}
	return out
}

// summarizeRuns folds a workflow's in-window runs into one WorkflowConsumption
// row. AIC sums the runs that carry it (failed runs omit aic) and is nil when
// none do. AvgDurationS is derived from action minutes (the only numeric
// duration in the logs schema).
func summarizeRuns(workflow string, runs []logsRun) WorkflowConsumption {
	var (
		aicSum     float64
		aicAny     bool
		apiCalls   int
		actionMins int
		newest     time.Time
	)
	for _, r := range runs {
		apiCalls += r.GitHubAPICalls
		actionMins += r.ActionMinutes
		if r.AIC != nil {
			aicSum += *r.AIC
			aicAny = true
		}
		if r.CreatedAt.After(newest) {
			newest = r.CreatedAt
		}
	}
	wc := WorkflowConsumption{Workflow: workflow, Runs: len(runs), APICalls: apiCalls}
	if len(runs) > 0 {
		wc.AvgDurationS = float64(actionMins) / float64(len(runs)) * secondsPerMinute
	}
	if aicAny {
		aic := aicSum
		wc.AIC = &aic
		wc.Cost = aicToUSD(&aic)
	}
	return wc
}

// safeItemsForRuns derives SAFE_WRITES from the same in-window run set used for
// API calls and AIC. When no client-side filtering happened, keep the CLI's
// summary total so older payloads that omit per-run safe_items_count still
// report full-window totals.
func safeItemsForRuns(payload logsPayload, runs []logsRun) int {
	if len(runs) == len(payload.Runs) {
		return payload.Summary.TotalSafeItems
	}
	var total int
	for _, r := range runs {
		total += r.SafeItemsCount
	}
	return total
}

// logSourceToReports builds one repo-level ConsumptionReport from `gh aw logs
// --json`, fanning out across the repo's agentic workflows (the slug→display
// resolution `gh aw logs` requires). Mirrors collectRepoReports' return shape so
// the aggregation layer downstream is source-agnostic. The api-consumption-report
// workflow need not be deployed (issue #103).
func logSourceToReports(
	ctx context.Context,
	repo string,
	mode FetchMode,
	now time.Time,
) ([]*ConsumptionReport, []Diagnostic) {
	var diags []Diagnostic
	workflows, err := ghWorkflowsAPI(ctx, repo)
	if err != nil {
		diags = append(diags, *newSoftDiagnostic(repo, fmt.Sprintf("Workflow discovery failed for %s: %v", repo, err)))
		return nil, diags
	}
	if len(workflows) == 0 {
		diags = append(diags, *newSoftDiagnostic(repo, fmt.Sprintf(
			"No agentic workflows (.lock.yml) found on %s — nothing to roll up.", repo)))
		return nil, diags
	}
	sortWorkflowRefs(workflows)

	var (
		perWF       []WorkflowConsumption
		totalCalls  int
		totalSafe   int
		aicSum      float64
		aicAny      bool
		newest      time.Time
		newestRunID int64
	)
	for _, wf := range workflows {
		payload, callErr := ghLogsAPI(ctx, repo, wf.Name, mode)
		if callErr != nil {
			diags = append(diags, *newSoftDiagnostic(repo, fmt.Sprintf(
				"`gh aw logs` failed for %s / %q: %v", repo, wf.Name, callErr)))
			continue
		}
		runs := filterRunsByWindow(payload.Runs, mode, now)
		if len(runs) == 0 {
			continue
		}
		row := summarizeRuns(wf.Name, runs)
		perWF = append(perWF, row)
		totalCalls += row.APICalls
		totalSafe += safeItemsForRuns(payload, runs)
		if row.AIC != nil {
			aicSum += *row.AIC
			aicAny = true
		}
		for _, r := range runs {
			if r.CreatedAt.After(newest) {
				newest, newestRunID = r.CreatedAt, r.RunID
			}
		}
	}

	if len(perWF) == 0 {
		diags = append(diags, *newSoftDiagnostic(repo, fmt.Sprintf(
			"No in-window agentic runs with AI-credit data on %s — runs may have failed, "+
				"fallen outside the window, or had artifacts expire.", repo)))
		return nil, diags
	}

	if newest.IsZero() {
		newest = now
	}
	report := &ConsumptionReport{
		Repo:            repo,
		Date:            newest,
		RunID:           newestRunID,
		GitHubAPICalls:  totalCalls,
		SafeOutputCalls: totalSafe,
		PerWorkflow:     perWF,
	}
	if aicAny {
		aic := aicSum
		report.AIC = &aic
		report.Cost = aicToUSD(&aic)
	}
	return []*ConsumptionReport{report}, diags
}

// collectReportsForSource dispatches one repo's report collection to the active
// data source, keeping AggregateConsumption agnostic to which path runs.
func collectReportsForSource(
	ctx context.Context,
	repo string,
	mode FetchMode,
	source SourceKind,
	now time.Time,
) ([]*ConsumptionReport, []Diagnostic) {
	switch source {
	case SourceLogs:
		return logSourceToReports(ctx, repo, mode, now)
	case SourceArtifacts:
		return collectRepoReports(ctx, repo, mode, now)
	}
	return nil, nil
}

// sourceEmptyDataHint returns the empty-data diagnostic appropriate to the
// active source when every group lacks its spend signal (AIC for logs, USD Cost
// for artifacts), or nil when data is present or no groups exist.
func sourceEmptyDataHint(source SourceKind, groups []ConsumptionGroup) *Diagnostic {
	if len(groups) == 0 {
		return nil
	}
	switch source {
	case SourceLogs:
		if allAICNil(groups) {
			d := nilAICDiag()
			return &d
		}
	case SourceArtifacts:
		if allCostNil(groups) {
			d := nilCostDiag()
			return &d
		}
	}
	return nil
}

// allAICNil reports whether every group's rolled-up AIC is nil — the signal
// that the logs source found no AI-credit data fleet-wide (see nilAICDiag).
func allAICNil(groups []ConsumptionGroup) bool {
	for i := range groups {
		if groups[i].AIC != nil {
			return false
		}
	}
	return true
}

// nilAICDiag explains an empty AIC/COST column under the logs source: every
// in-window run either failed (no aic), fell outside the window, or had its
// artifacts expire. Distinct from nilCostDiag, which covers the artifact
// source's structural lack of a USD field.
func nilAICDiag() Diagnostic {
	msg := "No AI-credit data found. Every in-window agentic run failed (failed runs " +
		"carry no aic), fell outside the window, or had its run artifacts expire. " +
		"Widen the window with --trailing/--since, or confirm the repos have recent " +
		"successful agentic runs."
	return Diagnostic{Code: DiagHint, Message: msg, Fields: map[string]any{fieldHint: msg}}
}

// sortWorkflowRefs orders workflows by display name for deterministic fan-out.
func sortWorkflowRefs(refs []workflowRef) {
	sort.Slice(refs, func(i, j int) bool { return refs[i].Name < refs[j].Name })
}
