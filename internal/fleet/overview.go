package fleet

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// OverviewOpts holds the configuration for an overview run.
type OverviewOpts struct {
	// Mode selects the gh aw logs run window used for health, AIC, cost, and no-op data.
	Mode FetchMode
	// fetcher overrides Status' GitHub fetcher in tests; nil uses the production fetcher.
	fetcher statusFetcher
}

// OverviewResult represents the final dashboard output.
type OverviewResult struct {
	// LoadedFrom names the config file or files used to build the scoped fleet.
	LoadedFrom string `json:"loaded_from"`
	// Window is the human-readable label for the active run-log window.
	Window string `json:"window"`
	// Repos is the joined per-repo dashboard row set sorted by repo name.
	Repos []RepoOverview `json:"repos"`
	// Total is the pooled fleet aggregate rendered as the TOTAL row.
	Total OverviewTotal `json:"total"`
}

// RepoOverview is one joined row per repo.
type RepoOverview struct {
	// Repo is the canonical owner/name repository identifier.
	Repo string `json:"repo"`
	// DriftState is aligned, drifted, or errored, as computed by Status.
	DriftState string `json:"drift_state"`
	// DriftDetail carries the full status detail when the repo is not clean.
	DriftDetail *RepoStatus `json:"drift_detail,omitempty"`
	// Runs is the health-counting denominator: successes plus failures.
	Runs int `json:"runs"`
	// Failures is the count of failure, timed_out, and startup_failure runs.
	Failures int `json:"failures"`
	// NoOps is the safeoutputs/noop aggregate clamped to the success count.
	NoOps int `json:"noops"`
	// HealthRate is successes divided by Runs, or nil when Runs is zero.
	HealthRate *float64 `json:"health_rate,omitempty"`
	// AIC is the positive AI-credit sum for successful in-window runs.
	AIC *float64 `json:"aic,omitempty"`
	// Cost is AIC converted to USD via aicToUSD.
	Cost *float64 `json:"cost,omitempty"`
	// RunsAvailable reports whether the run-log fan-out succeeded for this repo.
	RunsAvailable bool `json:"runs_available"`
	// RunsError describes the run-log fan-out failure when RunsAvailable is false.
	RunsError string `json:"runs_error,omitempty"`
	// ModelSupport carries the model-support advisory when the repo is affected
	// or undetermined; nil (and omitted) when the repo has no model-support
	// problem, keeping clean rows byte-identical to prior output.
	ModelSupport *ModelSupport `json:"model_support,omitempty"`
}

// ModelSupport is the per-repo advisory reporting whether a repo's recent
// agentic runs failed because the effective Copilot model is unsupported or
// deprecated. It is additive to the RepoOverview row and mirrors the
// pointer-with-omitempty convention of DriftDetail.
type ModelSupport struct {
	// Status is ModelSupportAffected when a model_not_supported failure issue
	// is open, or ModelSupportUndetermined when the failure-issue signal could
	// not be read.
	Status string `json:"status"`
	// OffendingModels names the repo's effective Copilot model(s); the v1
	// collector resolves at most one per repo. Empty means the model could not
	// be named (report as unknown).
	OffendingModels []string `json:"offending_models,omitempty"`
}

const (
	// ModelSupportAffected marks a repo with an open model_not_supported issue.
	ModelSupportAffected = "affected"
	// ModelSupportUndetermined marks a repo whose failure-issue read failed.
	ModelSupportUndetermined = "undetermined"
)

// OverviewTotal represents the pooled fleet aggregate.
type OverviewTotal struct {
	// Runs is the pooled health-counting run total.
	Runs int `json:"runs"`
	// Failures is the pooled failed-run total.
	Failures int `json:"failures"`
	// NoOps is the pooled no-op run total.
	NoOps int `json:"noops"`
	// HealthRate is the pooled success rate, or nil when Runs is zero.
	HealthRate *float64 `json:"health_rate,omitempty"`
	// AIC is the pooled positive AI-credit sum.
	AIC *float64 `json:"aic,omitempty"`
	// Cost is AIC converted to USD via aicToUSD.
	Cost *float64 `json:"cost,omitempty"`
	// Aligned is the number of repos with aligned drift state.
	Aligned int `json:"aligned"`
	// Drifted is the number of repos with drifted drift state.
	Drifted int `json:"drifted"`
	// Errored is the number of repos with errored drift state.
	Errored int `json:"errored"`
}

type overviewHealth struct {
	runs       int
	failures   int
	noOps      int
	healthRate *float64
	aic        *float64
	cost       *float64
	available  bool
	errMessage string
}

// overviewModelHealth is the model collector's per-repo reduce result. A zero
// status means clean, so joinOverviewRow leaves RepoOverview.ModelSupport nil.
type overviewModelHealth struct {
	status         string // "", ModelSupportAffected, or ModelSupportUndetermined
	offendingModel string // "" ⇒ unknown
}

const (
	conclusionSuccess        = "success"
	conclusionFailure        = "failure"
	conclusionTimedOut       = "timed_out"
	conclusionStartupFailure = "startup_failure"
	conclusionCancelled      = "cancelled"
	conclusionSkipped        = "skipped"
)

const (
	overviewSignalDrift        = "drift"
	overviewSignalRuns         = "runs"
	overviewSignalModel        = "model"
	diagnosticSignalFieldCount = 2
)

// runOutcome is the health-counting classification of one run conclusion.
type runOutcome int

const (
	// outcomeIgnored is a non-counted conclusion: cancelled, skipped, or empty.
	outcomeIgnored runOutcome = iota
	// outcomeSuccess is a successful, health-positive run.
	outcomeSuccess
	// outcomeFailure is a failed run, or any unknown terminal state.
	outcomeFailure
)

// Overview computes the drift, health, and cost for the scoped repos and joins them into a single OverviewResult.
func Overview(ctx context.Context, cfg *Config, opts OverviewOpts) (*OverviewResult, []Diagnostic, error) {
	repos := overviewRepoNames(cfg)
	result := &OverviewResult{
		LoadedFrom: cfg.LoadedFrom,
		Window:     formatFetchMode(opts.Mode),
		Repos:      []RepoOverview{},
	}
	if len(repos) == 0 {
		return result, []Diagnostic{{
			Code:    DiagEmptyFleet,
			Message: "No repos in scope.",
		}}, nil
	}

	var (
		wg          sync.WaitGroup
		driftRows   map[string]RepoStatus
		driftDiags  []Diagnostic
		driftErr    error
		healthRows  map[string]overviewHealth
		healthDiags []Diagnostic
		healthErr   error
		modelRows   map[string]overviewModelHealth
		modelDiags  []Diagnostic
	)

	wg.Go(func() {
		driftRows, driftDiags, driftErr = collectOverviewDrift(ctx, cfg, opts.fetcher)
	})
	wg.Go(func() {
		healthRows, healthDiags, healthErr = collectOverviewHealth(ctx, repos, opts.Mode)
	})
	wg.Go(func() {
		modelRows, modelDiags = collectOverviewModelHealth(ctx, repos)
	})
	wg.Wait()

	diags := append(signalDiags(driftDiags, overviewSignalDrift), healthDiags...)
	diags = append(diags, modelDiags...)
	switch {
	case driftErr != nil:
		return result, diags, driftErr
	case healthErr != nil:
		return result, diags, healthErr
	}

	for _, repo := range repos {
		row := joinOverviewRow(repo, driftRows[repo], healthRows[repo], modelRows[repo])
		result.Repos = append(result.Repos, row)
		addOverviewTotal(&result.Total, row)
	}
	finalizeOverviewTotal(&result.Total)
	sortOverviewDiagnostics(diags)
	return result, diags, nil
}

func overviewRepoNames(cfg *Config) []string {
	repos := make([]string, 0, len(cfg.Repos))
	for repo := range cfg.Repos {
		repos = append(repos, repo)
	}
	sort.Strings(repos)
	return repos
}

func collectOverviewDrift(
	ctx context.Context, cfg *Config, fetcher statusFetcher,
) (map[string]RepoStatus, []Diagnostic, error) {
	res, diags, err := Status(ctx, cfg, StatusOpts{fetcher: fetcher})
	rows := map[string]RepoStatus{}
	if res != nil {
		for _, row := range res.Repos {
			rows[row.Repo] = row
		}
	}
	return rows, diags, err
}

func collectOverviewHealth(
	ctx context.Context, repos []string, mode FetchMode,
) (map[string]overviewHealth, []Diagnostic, error) {
	if err := ensureLogsSourceGhAwVersion(ctx); err != nil {
		return nil, nil, err
	}

	now := timeNowUTC()
	rows := make(map[string]overviewHealth, len(repos))
	var diags []Diagnostic
	for _, repo := range repos {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return rows, diags, ctxErr
		}
		runData, repoDiags := collectRepoRuns(ctx, repo, mode, now)
		fanoutMsg, fanoutFailed := runFanoutFailure(repoDiags)
		runDiags := overviewRunDiagnostics(repo, repoDiags)
		if fanoutFailed && len(runData) == 0 {
			row := overviewHealth{available: false, errMessage: fanoutMsg}
			rows[repo] = row
			diags = append(diags, runDiags...)
			continue
		}
		rows[repo] = reduceOverviewHealth(runData)
		diags = append(diags, runDiags...)
	}
	return rows, diags, nil
}

func runFanoutFailure(diags []Diagnostic) (string, bool) {
	for _, d := range diags {
		if isRunFanoutFailureMessage(d.Message) {
			return d.Message, true
		}
	}
	return "", false
}

func isRunFanoutFailureMessage(msg string) bool {
	return strings.Contains(msg, "Workflow discovery failed") ||
		strings.Contains(msg, "`gh aw logs` failed")
}

func overviewRunDiagnostics(repo string, diags []Diagnostic) []Diagnostic {
	out := make([]Diagnostic, 0, len(diags))
	seen := map[string]bool{}
	for _, d := range diags {
		if isRunFanoutFailureMessage(d.Message) {
			d = buildRunsDiagnostic(repo, d.Message)
		} else {
			d = withDiagnosticSignal(d, repo, overviewSignalRuns)
		}
		key := d.Code + "\x00" + d.Message
		if seen[key] {
			continue
		}
		out = append(out, d)
		seen[key] = true
	}
	return out
}

func buildRunsDiagnostic(repo, msg string) Diagnostic {
	hints := CollectHintDiagnostics(msg)
	if len(hints) > 0 && hints[0].Code != DiagHTTP404 {
		return withDiagnosticSignal(hints[0], repo, overviewSignalRuns)
	}
	return withDiagnosticSignal(buildRepoErrorDiag(repo, msg), repo, overviewSignalRuns)
}

func withDiagnosticSignal(d Diagnostic, repo, signal string) Diagnostic {
	fields := make(map[string]any, len(d.Fields)+diagnosticSignalFieldCount)
	for key, value := range d.Fields {
		fields[key] = value
	}
	fields[fieldRepo] = repo
	fields[fieldSignal] = signal
	d.Fields = fields
	return d
}

// collectOverviewModelHealth reads each repo's open agentic-workflows failure
// issues and flags those whose recent runs failed on an unsupported/deprecated
// Copilot model, naming the effective model for flagged repos. It never returns
// an error: a failed failure-issue read degrades that repo to undetermined with
// a signal-tagged diagnostic, mirroring the runs-unavailable degradation.
func collectOverviewModelHealth(
	ctx context.Context, repos []string,
) (map[string]overviewModelHealth, []Diagnostic) {
	rows := make(map[string]overviewModelHealth, len(repos))
	var diags []Diagnostic
	for _, repo := range repos {
		if ctx.Err() != nil {
			return rows, diags
		}
		issues, err := ghFailureIssuesAPI(ctx, repo)
		if err != nil {
			rows[repo] = overviewModelHealth{status: ModelSupportUndetermined}
			msg := fmt.Sprintf("model-support signal unavailable for %s: %v", repo, err)
			diags = append(diags, withDiagnosticSignal(buildRepoErrorDiag(repo, msg), repo, overviewSignalModel))
			continue
		}
		if !anyModelFailure(issues) {
			continue
		}
		rows[repo] = resolveOverviewModel(ctx, repo)
		diags = append(diags, *newSoftDiagnostic(repo, modelSupportRemediationHint))
	}
	return rows, diags
}

// anyModelFailure reports whether any open failure issue flags a
// model-not-supported failure.
func anyModelFailure(issues []failureIssue) bool {
	for _, issue := range issues {
		if parseModelFailure(issue.Body) {
			return true
		}
	}
	return false
}

// resolveOverviewModel builds the affected reduce result for a flagged repo,
// naming its effective Copilot model. A variable-read error leaves the model
// empty (unknown) but never drops the flag (FR-002).
func resolveOverviewModel(ctx context.Context, repo string) overviewModelHealth {
	health := overviewModelHealth{status: ModelSupportAffected}
	vars, err := ghRepoVariablesAPI(ctx, repo)
	if err != nil {
		return health
	}
	health.offendingModel = resolveEffectiveModel(vars)
	return health
}

// classifyConclusion maps a gh aw logs conclusion to the overview health taxonomy.
func classifyConclusion(c string) runOutcome {
	switch c {
	case conclusionSuccess:
		return outcomeSuccess
	case conclusionFailure, conclusionTimedOut, conclusionStartupFailure:
		return outcomeFailure
	case conclusionCancelled, conclusionSkipped, "":
		return outcomeIgnored
	default:
		return outcomeFailure
	}
}

func reduceOverviewHealth(workflows []repoRunData) overviewHealth {
	var (
		successes int
		failures  int
		noopRaw   int
		aicSum    float64
		aicAny    bool
	)
	for _, wf := range workflows {
		noopRaw += noopCount(logsPayload{MCPToolUsage: wf.MCPToolUsage})
		for _, run := range wf.Runs {
			switch classifyConclusion(run.Conclusion) {
			case outcomeSuccess:
				successes++
			case outcomeFailure:
				failures++
			case outcomeIgnored:
				continue
			}
			if run.AIC != nil && *run.AIC > 0 {
				aicSum += *run.AIC
				aicAny = true
			}
		}
	}

	runs := successes + failures
	row := overviewHealth{
		runs:      runs,
		failures:  failures,
		noOps:     clampNoOps(noopRaw, successes),
		available: true,
	}
	if runs > 0 {
		rate := float64(successes) / float64(runs)
		row.healthRate = &rate
	}
	if aicAny {
		aic := aicSum
		row.aic = &aic
		row.cost = aicToUSD(&aic)
	}
	return row
}

func clampNoOps(raw, successes int) int {
	if raw < 0 {
		return 0
	}
	if raw > successes {
		return successes
	}
	return raw
}

func joinOverviewRow(repo string, drift RepoStatus, health overviewHealth, model overviewModelHealth) RepoOverview {
	row := RepoOverview{
		Repo:          repo,
		DriftState:    drift.DriftState,
		Runs:          health.runs,
		Failures:      health.failures,
		NoOps:         health.noOps,
		HealthRate:    health.healthRate,
		AIC:           health.aic,
		Cost:          health.cost,
		RunsAvailable: health.available,
		RunsError:     health.errMessage,
	}
	if drift.DriftState != DriftStateAligned {
		d := drift
		row.DriftDetail = &d
	}
	if model.status != "" {
		ms := &ModelSupport{Status: model.status}
		if model.offendingModel != "" {
			ms.OffendingModels = []string{model.offendingModel}
		}
		row.ModelSupport = ms
	}
	return row
}

func addOverviewTotal(total *OverviewTotal, row RepoOverview) {
	switch row.DriftState {
	case DriftStateAligned:
		total.Aligned++
	case DriftStateDrifted:
		total.Drifted++
	default:
		total.Errored++
	}
	if !row.RunsAvailable {
		return
	}
	total.Runs += row.Runs
	total.Failures += row.Failures
	total.NoOps += row.NoOps
	if row.AIC != nil {
		if total.AIC == nil {
			v := 0.0
			total.AIC = &v
		}
		*total.AIC += *row.AIC
	}
}

func finalizeOverviewTotal(total *OverviewTotal) {
	if total.Runs > 0 {
		successes := total.Runs - total.Failures
		rate := float64(successes) / float64(total.Runs)
		total.HealthRate = &rate
	}
	total.Cost = aicToUSD(total.AIC)
}

func signalDiags(diags []Diagnostic, signal string) []Diagnostic {
	out := make([]Diagnostic, 0, len(diags))
	for _, d := range diags {
		if _, ok := d.Fields[fieldRepo]; ok {
			d.Fields[fieldSignal] = signal
		}
		out = append(out, d)
	}
	return out
}

func sortOverviewDiagnostics(diags []Diagnostic) {
	sort.SliceStable(diags, func(i, j int) bool {
		ri, _ := diags[i].Fields[fieldRepo].(string)
		rj, _ := diags[j].Fields[fieldRepo].(string)
		if ri == rj {
			return diags[i].Code < diags[j].Code
		}
		return ri < rj
	})
}

func timeNowUTC() time.Time { return time.Now().UTC() }
