package fleet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"time"
)

// forecastPayload is the top-level document from `gh aw forecast --json` for one repo.
type forecastPayload struct {
	AsOf      time.Time          `json:"as_of"`
	Period    string             `json:"period"`
	Workflows []forecastWorkflow `json:"workflows"`
}

// forecastWorkflow is one workflow row inside a forecastPayload.
type forecastWorkflow struct {
	WorkflowID          string      `json:"workflow_id"`
	SampledRuns         int         `json:"sampled_runs"`
	WeeklyProjectedAIC  float64     `json:"weekly_projected_aic"`
	MonthlyProjectedAIC float64     `json:"monthly_projected_aic"`
	WeeklyMonteCarlo    *monteCarlo `json:"weekly_monte_carlo"`
	MonthlyMonteCarlo   *monteCarlo `json:"monthly_monte_carlo"`
}

// monteCarlo is the advisory confidence band for a workflow forecast.
// Absent (nil) when sampled_runs == 0 (cold start).
type monteCarlo struct {
	P10        float64 `json:"p10_projected_aic"`
	P50        float64 `json:"p50_projected_aic"`
	P90        float64 `json:"p90_projected_aic"`
	IsReliable bool    `json:"is_reliable"`
}

// pick returns the point estimate and Monte Carlo band for the requested period.
// Keeps the week/month branch in one place.
func (wf forecastWorkflow) pick(period Period) (float64, *monteCarlo) {
	if period == PeriodMonth {
		return wf.MonthlyProjectedAIC, wf.MonthlyMonteCarlo
	}
	return wf.WeeklyProjectedAIC, wf.WeeklyMonteCarlo
}

// ForecastGroupBy is the closed set of --by axes accepted by AggregateForecast.
// It is a separate enum from GroupByKind because forecast adds a tier axis
// and lacks the workflow axis (Decision 6).
type ForecastGroupBy int

const (
	// ForecastByRepo groups forecasts by "owner/name".
	ForecastByRepo ForecastGroupBy = iota
	// ForecastByProfile groups by profile name.
	ForecastByProfile
	// ForecastByCostCenter groups by RepoSpec.CostCenter.
	ForecastByCostCenter
	// ForecastByTier groups by profile tier.
	ForecastByTier
)

//nolint:gochecknoglobals // immutable lookup table; Go has no const arrays.
var forecastGroupByNames = [...]string{
	ForecastByRepo:       axisRepo,
	ForecastByProfile:    axisProfile,
	ForecastByCostCenter: axisCostCenter,
	ForecastByTier:       "tier",
}

// String returns the CLI-vocabulary name for the axis, or "" when out of range.
func (g ForecastGroupBy) String() string {
	if int(g) < 0 || int(g) >= len(forecastGroupByNames) {
		return ""
	}
	return forecastGroupByNames[g]
}

// ParseForecastGroupBy returns the ForecastGroupBy for the canonical name s,
// or an error naming the valid axes when s is outside the closed set.
func ParseForecastGroupBy(s string) (ForecastGroupBy, error) {
	for k, name := range forecastGroupByNames {
		if name == s {
			return ForecastGroupBy(k), nil
		}
	}
	return 0, fmt.Errorf("invalid --by value %q: expected one of repo, profile, cost-center, tier", s)
}

// Period selects the projection horizon for AggregateForecast.
type Period int

const (
	// PeriodWeek is a 7-day projection.
	PeriodWeek Period = iota
	// PeriodMonth is a 30-day projection.
	PeriodMonth
)

//nolint:gochecknoglobals // immutable lookup table; Go has no const arrays.
var periodNames = [...]string{
	PeriodWeek:  "week",
	PeriodMonth: "month",
}

// String returns the CLI-vocabulary name for the period.
func (p Period) String() string {
	if int(p) < 0 || int(p) >= len(periodNames) {
		return ""
	}
	return periodNames[p]
}

// daysPerMonth is the projection window for the month period passed to `gh aw forecast --days`.
const daysPerMonth = 30

// daysPerWeek is the projection window for the week period passed to `gh aw forecast --days`.
const daysPerWeek = 7

// Days returns the --days argument value for the upstream `gh aw forecast` call.
func (p Period) Days() int {
	if p == PeriodMonth {
		return daysPerMonth
	}
	return daysPerWeek
}

// ParsePeriod returns the Period for the canonical name s, or an error when
// s is outside the closed set (week|month).
func ParsePeriod(s string) (Period, error) {
	for k, name := range periodNames {
		if name == s {
			return Period(k), nil
		}
	}
	return 0, fmt.Errorf("invalid --period value %q: expected one of week, month", s)
}

// forecastMinVersion is the minimum gh-aw release whose `forecast --json`
// command produces the weekly/monthly field schema consumed here.
const forecastMinVersion = CompileStrictMinVersion

// ghForecastAPI fans out `gh aw forecast --json --repo <repo> --days <N>` for
// one repo. It captures stdout even on non-zero exit: if stdout decodes as a
// valid forecastPayload, callers receive the partial payload plus the wrapped
// error (FR-013 partial handling). Package-level seam so tests run offline.
//
//nolint:gochecknoglobals // test-injection seam for `gh aw forecast --json`
var ghForecastAPI = func(ctx context.Context, repo string, period Period) (forecastPayload, error) {
	cmd := exec.CommandContext(ctx, "gh", "aw", "forecast", "--json",
		"--repo", repo, "--days", strconv.Itoa(period.Days()))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if stdout.Len() == 0 {
		if runErr != nil {
			return forecastPayload{}, ghErr(runErr)
		}
		return forecastPayload{}, nil
	}
	var payload forecastPayload
	if decErr := json.Unmarshal(stdout.Bytes(), &payload); decErr != nil {
		if runErr != nil {
			return forecastPayload{}, ghErr(runErr)
		}
		return forecastPayload{}, fmt.Errorf("decode gh aw forecast --json for %q: %w", repo, decErr)
	}
	return payload, runErr
}

// ensureForecastGhAwVersion rejects gh-aw releases that predate the
// forecast --json command. Without this gate older CLIs return a non-zero
// exit before producing any output, causing all repos to hard-fail-skip.
func ensureForecastGhAwVersion(ctx context.Context) error {
	detected, err := ghAwVersion(ctx)
	if err != nil {
		return fmt.Errorf("gh aw --version probe failed for forecast: %w. "+
			"Install gh-aw %s or newer to use the forecast subcommand",
			err, forecastMinVersion)
	}
	if detected == "" {
		return fmt.Errorf("gh aw --version did not report a vMAJOR.MINOR.PATCH token; "+
			"forecast requires gh-aw %s or newer",
			forecastMinVersion)
	}
	cmp, cmpErr := compareVersionTokens(detected, forecastMinVersion)
	if cmpErr != nil {
		return fmt.Errorf("compare gh-aw version for forecast: %w", cmpErr)
	}
	if cmp < 0 {
		return fmt.Errorf("gh aw is too old for forecast: detected %s, minimum %s required. "+
			"Install with `gh extension install github/gh-aw --pin %s`",
			detected, forecastMinVersion, forecastMinVersion)
	}
	return nil
}

// ForecastGroup is one aggregated row in a ForecastResult.
// Band fields (AICP10/50/90) are nil for an all-cold group (FR-005).
// The omitempty tag MUST NOT be on band fields; nil marshals as JSON null
// to distinguish "no history" from zero-spend.
type ForecastGroup struct {
	Key              string   `json:"key"`
	ProjectedAIC     float64  `json:"projected_aic"`
	ProjectedCostUSD *float64 `json:"projected_cost_usd,omitempty"`
	AICP10           *float64 `json:"aic_p10"`
	AICP50           *float64 `json:"aic_p50"`
	AICP90           *float64 `json:"aic_p90"`
	SampledRuns      int      `json:"sampled_runs"`
	Cold             bool     `json:"cold"`
	WorkflowCount    int      `json:"workflow_count"`
}

// ForecastResult is the payload placed in Envelope.Result for
// `gh-aw-fleet forecast --output json`.
type ForecastResult struct {
	LoadedFrom string          `json:"loaded_from"`
	Period     string          `json:"period"`
	GroupBy    string          `json:"group_by"`
	Groups     []ForecastGroup `json:"groups"`
}

// addForecastToGroups folds one repo's forecast payload into the groups map
// according to the chosen grouping axis (by). For each workflow in the payload,
// it determines the group key(s) based on the axis and accumulates the
// projection values and band statistics.
func addForecastToGroups(
	groups map[string]*ForecastGroup,
	cfg *Config,
	repo string,
	by ForecastGroupBy,
	payload forecastPayload,
	period Period,
) []Diagnostic {
	spec := cfg.Repos[repo]
	var diags []Diagnostic

	for _, wf := range payload.Workflows {
		point, band := wf.pick(period)
		keys := forecastKeys(cfg, repo, spec, by)
		for _, key := range keys {
			addPointToGroup(groups, key, point, band, wf.SampledRuns)
			if band != nil && !band.IsReliable {
				diags = append(diags, newForecastLowConfidenceDiagnostic(repo, key, period, wf.WorkflowID))
			}
		}
	}
	return diags
}

// forecastKeys returns the group key(s) for a repo based on the grouping axis.
func forecastKeys(cfg *Config, repo string, spec RepoSpec, by ForecastGroupBy) []string {
	switch by {
	case ForecastByRepo:
		return []string{repo}
	case ForecastByProfile:
		if len(spec.Profiles) == 0 {
			return nil
		}
		return spec.Profiles
	case ForecastByCostCenter:
		key := spec.CostCenter
		if key == "" {
			key = unsetCostCenter
		}
		return []string{key}
	case ForecastByTier:
		if len(spec.Profiles) == 0 {
			return []string{unsetCostCenter}
		}
		tiers := make(map[string]bool)
		for _, p := range spec.Profiles {
			tier := cfg.Profiles[p].Tier
			if tier == "" {
				tier = unsetCostCenter
			}
			tiers[tier] = true
		}
		keys := make([]string, 0, len(tiers))
		for tier := range tiers {
			keys = append(keys, tier)
		}
		return keys
	}
	return nil
}

func newForecastLowConfidenceDiagnostic(repo, group string, period Period, workflowID string) Diagnostic {
	msg := fmt.Sprintf("Forecast Monte Carlo band for %s workflow %q in group %q is low-confidence; "+
		"treat P10/P50/P90 as advisory until more runs are sampled.", repo, workflowID, group)
	return Diagnostic{
		Code:    DiagForecastLowConfidence,
		Message: msg,
		Fields: map[string]any{
			fieldGroup:    group,
			fieldPeriod:   period.String(),
			fieldRepo:     repo,
			fieldWorkflow: workflowID,
		},
	}
}

// addPointToGroup adds a single workflow's projection to a group, folding in
// both the point estimate and the Monte Carlo band (if non-nil).
func addPointToGroup(
	groups map[string]*ForecastGroup,
	key string,
	point float64,
	band *monteCarlo,
	sampledRuns int,
) {
	g, ok := groups[key]
	if !ok {
		g = &ForecastGroup{Key: key, Cold: true}
		groups[key] = g
	}

	g.ProjectedAIC += point
	g.SampledRuns += sampledRuns
	g.WorkflowCount++

	// Update Cold flag: becomes false when we see any workflow with sampled_runs > 0
	if sampledRuns > 0 {
		g.Cold = false
	}

	// Accumulate band: initialize on first non-nil, add on subsequent non-nil,
	// skip nil bands but don't reset the flag.
	if band != nil {
		if g.AICP10 == nil {
			p10, p50, p90 := band.P10, band.P50, band.P90
			g.AICP10 = &p10
			g.AICP50 = &p50
			g.AICP90 = &p90
		} else {
			*g.AICP10 += band.P10
			*g.AICP50 += band.P50
			*g.AICP90 += band.P90
		}
	}
}

// materializeForecastGroups converts the map to a sorted slice and computes
// ProjectedCostUSD from ProjectedAIC.
func materializeForecastGroups(groups map[string]*ForecastGroup) []ForecastGroup {
	out := make([]ForecastGroup, 0, len(groups))
	for _, g := range groups {
		// Compute projected_cost_usd from projected_aic
		if g.ProjectedAIC > 0 {
			aic := g.ProjectedAIC
			g.ProjectedCostUSD = aicToUSD(&aic)
		}
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// AggregateForecast walks every repo in cfg in sorted order, calls
// `gh aw forecast --json` for each, and assembles a ForecastResult grouped
// by the requested axis.
func AggregateForecast(
	ctx context.Context,
	cfg *Config,
	period Period,
	by ForecastGroupBy,
) (*ForecastResult, []Diagnostic, error) {
	if err := ensureForecastGhAwVersion(ctx); err != nil {
		return nil, nil, err
	}

	repoNames := make([]string, 0, len(cfg.Repos))
	for r := range cfg.Repos {
		repoNames = append(repoNames, r)
	}
	sort.Strings(repoNames)

	groups := map[string]*ForecastGroup{}
	var diags []Diagnostic
	successfulForecasts := 0

	for _, repo := range repoNames {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, diags, ctxErr
		}

		payload, err := ghForecastAPI(ctx, repo, period)
		if err != nil && len(payload.Workflows) == 0 {
			// Hard fail: error and no partial output
			diags = append(diags, *newSoftDiagnostic(repo,
				fmt.Sprintf("`gh aw forecast` failed for %s: %v. Skipping this repo.", repo, err)))
			continue
		}
		successfulForecasts++
		if err != nil && len(payload.Workflows) > 0 {
			// Partial output: warning but still aggregate
			diags = append(diags, *newSoftDiagnostic(repo,
				fmt.Sprintf("Forecast for %s is partial — `gh aw forecast` exited non-zero but produced a decodable partial document. Projections for this repo reflect only the workflows present in the output.", repo)))
		}

		diags = append(diags, addForecastToGroups(groups, cfg, repo, by, payload, period)...)
	}

	result := &ForecastResult{
		LoadedFrom: cfg.LoadedFrom,
		Period:     period.String(),
		GroupBy:    by.String(),
		Groups:     materializeForecastGroups(groups),
	}

	if len(repoNames) > 0 && successfulForecasts == 0 {
		return result, diags, errors.New(
			"gh aw forecast failed for every repo; no decodable forecast payloads were produced",
		)
	}

	// Emit all-cold fleet diagnostic if every group is cold
	allCold := true
	for _, g := range result.Groups {
		if !g.Cold {
			allCold = false
			break
		}
	}
	if allCold && len(result.Groups) > 0 {
		msg := "Every repo in the fleet is cold-started (sampled_runs=0 for all workflows). " +
			"Projections are zero until agentic workflows accumulate run history. " +
			"Re-run after one billing period."
		diags = append(diags, Diagnostic{Code: DiagHint, Message: msg, Fields: map[string]any{fieldHint: msg}})
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, diags, ctxErr
	}
	return result, diags, nil
}
