package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// loadForecastFixture reads a fixture JSON file and unmarshals it to a forecastPayload.
func loadForecastFixture(t *testing.T, name string) forecastPayload {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "forecast", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var p forecastPayload
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	return p
}

// TestAggregateForecastHappyPath covers the single-workflow happy path
// with a warm (non-zero sampled_runs) workflow.
func TestAggregateForecastHappyPath(t *testing.T) {
	prevF, prevV := ghForecastAPI, ghAwVersion
	t.Cleanup(func() { ghForecastAPI, ghAwVersion = prevF, prevV })

	singlePayload := loadForecastFixture(t, "forecast_single_workflow.json")
	ghForecastAPI = func(_ context.Context, repo string, period Period) (forecastPayload, error) {
		if repo == "test/repo" {
			return singlePayload, nil
		}
		return forecastPayload{}, nil
	}
	ghAwVersion = func(_ context.Context) (string, error) {
		return CompileStrictMinVersion, nil
	}

	cfg := &Config{
		LoadedFrom: "test",
		Repos: map[string]RepoSpec{
			"test/repo": {
				Profiles: []string{"default"},
			},
		},
		Profiles: map[string]Profile{
			"default": {Tier: "standard"},
		},
	}

	res, diags, err := AggregateForecast(context.Background(), cfg, PeriodWeek, ForecastByRepo)
	if err != nil {
		t.Fatalf("AggregateForecast: %v", err)
	}

	if len(res.Groups) != 1 {
		t.Errorf("len(Groups) = %d; want 1", len(res.Groups))
	}
	if len(diags) != 0 {
		t.Errorf("len(diags) = %d; want 0", len(diags))
	}

	g := res.Groups[0]
	if g.Key != "test/repo" {
		t.Errorf("Key = %q; want \"test/repo\"", g.Key)
	}
	if g.Cold {
		t.Errorf("Cold = true; want false (sampled_runs=1)")
	}
	if g.SampledRuns != 1 {
		t.Errorf("SampledRuns = %d; want 1", g.SampledRuns)
	}
	if g.WorkflowCount != 1 {
		t.Errorf("WorkflowCount = %d; want 1", g.WorkflowCount)
	}

	// Check projected AIC (weekly from fixture is 32.082)
	const expectedWeeklyAIC = 32.082
	if g.ProjectedAIC < expectedWeeklyAIC-0.01 || g.ProjectedAIC > expectedWeeklyAIC+0.01 {
		t.Errorf("ProjectedAIC = %.3f; want ~%.3f", g.ProjectedAIC, expectedWeeklyAIC)
	}

	// Check that P50 is non-nil (from weekly_monte_carlo in fixture)
	if g.AICP50 == nil {
		t.Errorf("AICP50 = nil; want non-nil")
	}
}

// TestAggregateForecastColdStart covers a fleet with only cold-start workflows
// (sampled_runs=0 across all workflows).
func TestAggregateForecastColdStart(t *testing.T) {
	prevF, prevV := ghForecastAPI, ghAwVersion
	t.Cleanup(func() { ghForecastAPI, ghAwVersion = prevF, prevV })

	coldPayload := loadForecastFixture(t, "forecast_cold_start.json")
	ghForecastAPI = func(_ context.Context, repo string, period Period) (forecastPayload, error) {
		if repo == "test/repo" {
			return coldPayload, nil
		}
		return forecastPayload{}, nil
	}
	ghAwVersion = func(_ context.Context) (string, error) {
		return CompileStrictMinVersion, nil
	}

	cfg := &Config{
		LoadedFrom: "test",
		Repos: map[string]RepoSpec{
			"test/repo": {
				Profiles: []string{"default"},
			},
		},
		Profiles: map[string]Profile{
			"default": {},
		},
	}

	res, diags, err := AggregateForecast(context.Background(), cfg, PeriodMonth, ForecastByRepo)
	if err != nil {
		t.Fatalf("AggregateForecast: %v", err)
	}

	if len(res.Groups) != 1 {
		t.Errorf("len(Groups) = %d; want 1", len(res.Groups))
	}

	g := res.Groups[0]
	if !g.Cold {
		t.Errorf("Cold = false; want true (all sampled_runs=0)")
	}
	if g.AICP10 != nil || g.AICP50 != nil || g.AICP90 != nil {
		t.Errorf("Band fields should be nil for all-cold group, got P10=%v P50=%v P90=%v",
			g.AICP10, g.AICP50, g.AICP90)
	}

	// Check for all-cold diagnostic
	foundAllColdDiag := false
	for _, d := range diags {
		if d.Code == "hint" && len(d.Message) > 0 {
			// The diagnostic should mention cold-started
			foundAllColdDiag = true
			break
		}
	}
	if !foundAllColdDiag {
		t.Errorf("Expected all-cold fleet diagnostic, got %d diags", len(diags))
	}
}

// TestAggrecastForecastHardFail covers the case where ghForecastAPI returns
// an error with zero workflows in the payload (hard fail, skipped repo).
func TestAggrecastForecastHardFail(t *testing.T) {
	prevF, prevV := ghForecastAPI, ghAwVersion
	t.Cleanup(func() { ghForecastAPI, ghAwVersion = prevF, prevV })

	ghForecastAPI = func(_ context.Context, repo string, period Period) (forecastPayload, error) {
		return forecastPayload{}, errors.New("simulate error")
	}
	ghAwVersion = func(_ context.Context) (string, error) {
		return CompileStrictMinVersion, nil
	}

	cfg := &Config{
		LoadedFrom: "test",
		Repos: map[string]RepoSpec{
			"test/repo": {
				Profiles: []string{"default"},
			},
		},
		Profiles: map[string]Profile{
			"default": {},
		},
	}

	res, diags, err := AggregateForecast(context.Background(), cfg, PeriodWeek, ForecastByRepo)
	if err != nil {
		t.Fatalf("AggregateForecast: %v", err)
	}

	// Result should be returned with zero groups
	if len(res.Groups) != 0 {
		t.Errorf("len(Groups) = %d; want 0", len(res.Groups))
	}

	// A diagnostic should be emitted for the skipped repo
	if len(diags) == 0 {
		t.Errorf("Expected hard-fail diagnostic, got 0 diags")
	}
}

// TestForecastPeriodWeek ensures the week period sums weekly_projected_aic.
func TestForecastPeriodWeek(t *testing.T) {
	prevF, prevV := ghForecastAPI, ghAwVersion
	t.Cleanup(func() { ghForecastAPI, ghAwVersion = prevF, prevV })

	singlePayload := loadForecastFixture(t, "forecast_single_workflow.json")
	ghForecastAPI = func(_ context.Context, repo string, period Period) (forecastPayload, error) {
		if repo == "test/repo" && period == PeriodWeek {
			return singlePayload, nil
		}
		return forecastPayload{}, nil
	}
	ghAwVersion = func(_ context.Context) (string, error) {
		return CompileStrictMinVersion, nil
	}

	cfg := &Config{
		LoadedFrom: "test",
		Repos: map[string]RepoSpec{
			"test/repo": {
				Profiles: []string{"default"},
			},
		},
		Profiles: map[string]Profile{
			"default": {},
		},
	}

	res, _, err := AggregateForecast(context.Background(), cfg, PeriodWeek, ForecastByRepo)
	if err != nil {
		t.Fatalf("AggregateForecast: %v", err)
	}

	// Weekly AIC from fixture is 32.082
	const expectedAIC = 32.082
	if len(res.Groups) != 1 {
		t.Fatalf("len(Groups) = %d; want 1", len(res.Groups))
	}
	g := res.Groups[0]
	if g.ProjectedAIC < expectedAIC-0.01 || g.ProjectedAIC > expectedAIC+0.01 {
		t.Errorf("ProjectedAIC = %.3f; want ~%.3f", g.ProjectedAIC, expectedAIC)
	}
}

// TestForecastPeriodMonth ensures the month period sums monthly_projected_aic.
func TestForecastPeriodMonth(t *testing.T) {
	prevF, prevV := ghForecastAPI, ghAwVersion
	t.Cleanup(func() { ghForecastAPI, ghAwVersion = prevF, prevV })

	singlePayload := loadForecastFixture(t, "forecast_single_workflow.json")
	ghForecastAPI = func(_ context.Context, repo string, period Period) (forecastPayload, error) {
		if repo == "test/repo" && period == PeriodMonth {
			return singlePayload, nil
		}
		return forecastPayload{}, nil
	}
	ghAwVersion = func(_ context.Context) (string, error) {
		return CompileStrictMinVersion, nil
	}

	cfg := &Config{
		LoadedFrom: "test",
		Repos: map[string]RepoSpec{
			"test/repo": {
				Profiles: []string{"default"},
			},
		},
		Profiles: map[string]Profile{
			"default": {},
		},
	}

	res, _, err := AggregateForecast(context.Background(), cfg, PeriodMonth, ForecastByRepo)
	if err != nil {
		t.Fatalf("AggregateForecast: %v", err)
	}

	// Monthly AIC from fixture is 137.494
	const expectedAIC = 137.494
	if len(res.Groups) != 1 {
		t.Fatalf("len(Groups) = %d; want 1", len(res.Groups))
	}
	g := res.Groups[0]
	if g.ProjectedAIC < expectedAIC-0.01 || g.ProjectedAIC > expectedAIC+0.01 {
		t.Errorf("ProjectedAIC = %.3f; want ~%.3f", g.ProjectedAIC, expectedAIC)
	}
}

// TestForecastGroupByProfile groups by profile name and fans out multi-profile repos.
func TestForecastGroupByProfile(t *testing.T) {
	prevF, prevV := ghForecastAPI, ghAwVersion
	t.Cleanup(func() { ghForecastAPI, ghAwVersion = prevF, prevV })

	singlePayload := loadForecastFixture(t, "forecast_single_workflow.json")
	ghForecastAPI = func(_ context.Context, repo string, period Period) (forecastPayload, error) {
		if repo == "test/repo" {
			return singlePayload, nil
		}
		return forecastPayload{}, nil
	}
	ghAwVersion = func(_ context.Context) (string, error) {
		return CompileStrictMinVersion, nil
	}

	cfg := &Config{
		LoadedFrom: "test",
		Repos: map[string]RepoSpec{
			"test/repo": {
				Profiles: []string{"default", "standard"},
			},
		},
		Profiles: map[string]Profile{
			"default":  {},
			"standard": {},
		},
	}

	res, _, err := AggregateForecast(context.Background(), cfg, PeriodWeek, ForecastByProfile)
	if err != nil {
		t.Fatalf("AggregateForecast: %v", err)
	}

	// Multi-profile repo should contribute to both groups
	if len(res.Groups) != 2 {
		t.Errorf("len(Groups) = %d; want 2", len(res.Groups))
	}

	// Both groups should have the same projection value
	const expectedAIC = 32.082
	for _, g := range res.Groups {
		if g.ProjectedAIC < expectedAIC-0.01 || g.ProjectedAIC > expectedAIC+0.01 {
			t.Errorf("Group %q ProjectedAIC = %.3f; want ~%.3f", g.Key, g.ProjectedAIC, expectedAIC)
		}
	}
}

// TestForecastGroupByCostCenter groups by cost-center and uses "<unset>" for empty.
func TestForecastGroupByCostCenter(t *testing.T) {
	prevF, prevV := ghForecastAPI, ghAwVersion
	t.Cleanup(func() { ghForecastAPI, ghAwVersion = prevF, prevV })

	singlePayload := loadForecastFixture(t, "forecast_single_workflow.json")
	ghForecastAPI = func(_ context.Context, repo string, period Period) (forecastPayload, error) {
		if repo == "test/repo" {
			return singlePayload, nil
		}
		return forecastPayload{}, nil
	}
	ghAwVersion = func(_ context.Context) (string, error) {
		return CompileStrictMinVersion, nil
	}

	cfg := &Config{
		LoadedFrom: "test",
		Repos: map[string]RepoSpec{
			"test/repo": {
				Profiles: []string{"default"},
				// No CostCenter set
			},
		},
		Profiles: map[string]Profile{
			"default": {},
		},
	}

	res, _, err := AggregateForecast(context.Background(), cfg, PeriodWeek, ForecastByCostCenter)
	if err != nil {
		t.Fatalf("AggregateForecast: %v", err)
	}

	if len(res.Groups) != 1 {
		t.Errorf("len(Groups) = %d; want 1", len(res.Groups))
	}

	g := res.Groups[0]
	if g.Key != unsetCostCenter {
		t.Errorf("Key = %q; want %q", g.Key, unsetCostCenter)
	}
}

// TestForecastGroupByTier groups by tier from profile definitions.
func TestForecastGroupByTier(t *testing.T) {
	prevF, prevV := ghForecastAPI, ghAwVersion
	t.Cleanup(func() { ghForecastAPI, ghAwVersion = prevF, prevV })

	singlePayload := loadForecastFixture(t, "forecast_single_workflow.json")
	ghForecastAPI = func(_ context.Context, repo string, period Period) (forecastPayload, error) {
		if repo == "test/repo" {
			return singlePayload, nil
		}
		return forecastPayload{}, nil
	}
	ghAwVersion = func(_ context.Context) (string, error) {
		return CompileStrictMinVersion, nil
	}

	cfg := &Config{
		LoadedFrom: "test",
		Repos: map[string]RepoSpec{
			"test/repo": {
				Profiles: []string{"standard"},
			},
		},
		Profiles: map[string]Profile{
			"standard": {Tier: "standard"},
		},
	}

	res, _, err := AggregateForecast(context.Background(), cfg, PeriodWeek, ForecastByTier)
	if err != nil {
		t.Fatalf("AggregateForecast: %v", err)
	}

	if len(res.Groups) != 1 {
		t.Errorf("len(Groups) = %d; want 1", len(res.Groups))
	}

	g := res.Groups[0]
	if g.Key != "standard" {
		t.Errorf("Key = %q; want \"standard\"", g.Key)
	}
}
