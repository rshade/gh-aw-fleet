package fleet

import (
	"encoding/json"
	"strings"
	"testing"
)

func budgetTestResult(groups []ConsumptionGroup, topBurners []WorkflowConsumption) *ConsumptionResult {
	return &ConsumptionResult{
		LoadedFrom: "fleet.local.json",
		FetchMode:  "latest",
		GroupBy:    "repo",
		Source:     "logs",
		Groups:     groups,
		TopBurners: topBurners,
	}
}

func assertBoolPtr(t *testing.T, got *bool, want bool) {
	t.Helper()
	if got == nil {
		t.Fatalf("bool pointer is nil; want %v", want)
	}
	wantPtr := boolPtr(want)
	if *got != *wantPtr {
		t.Fatalf("bool pointer = %v; want %v", *got, *wantPtr)
	}
}

func TestApplyBudget_StrictThreshold(t *testing.T) {
	budget := 5.0
	res := budgetTestResult([]ConsumptionGroup{
		{Key: "above", AIC: fptr(5.01)},
		{Key: "below", AIC: fptr(4.99)},
		{Key: "equal", AIC: fptr(5.0)},
		{Key: "nil"},
		{Key: "zero", AIC: fptr(0)},
	}, nil)

	ApplyBudget(res, &budget)

	if res.Budget == nil || *res.Budget != budget {
		t.Fatalf("Budget = %v; want %v", res.Budget, budget)
	}
	assertBoolPtr(t, res.Groups[0].OverBudget, true)
	assertBoolPtr(t, res.Groups[1].OverBudget, false)
	assertBoolPtr(t, res.Groups[2].OverBudget, false)
	assertBoolPtr(t, res.Groups[3].OverBudget, false)
	assertBoolPtr(t, res.Groups[4].OverBudget, false)

	zeroBudget := 0.0
	zeroRes := budgetTestResult([]ConsumptionGroup{
		{Key: "positive", AIC: fptr(0.01)},
		{Key: "zero", AIC: fptr(0)},
	}, nil)
	ApplyBudget(zeroRes, &zeroBudget)
	assertBoolPtr(t, zeroRes.Groups[0].OverBudget, true)
	assertBoolPtr(t, zeroRes.Groups[1].OverBudget, false)
}

func TestApplyBudget_AllGroupingAxes(t *testing.T) {
	for _, groupBy := range []string{"repo", "profile", "cost-center", "workflow"} {
		t.Run(groupBy, func(t *testing.T) {
			budget := 10.0
			res := budgetTestResult([]ConsumptionGroup{
				{Key: groupBy + "-over", AIC: fptr(10.5)},
				{Key: groupBy + "-under", AIC: fptr(9.5)},
			}, nil)
			res.GroupBy = groupBy

			ApplyBudget(res, &budget)

			assertBoolPtr(t, res.Groups[0].OverBudget, true)
			assertBoolPtr(t, res.Groups[1].OverBudget, false)
		})
	}
}

func TestApplyBudget_TopBurners(t *testing.T) {
	budget := 50.0
	res := budgetTestResult(nil, []WorkflowConsumption{
		{Workflow: "over", AIC: fptr(50.01)},
		{Workflow: "equal", AIC: fptr(50)},
		{Workflow: "below", AIC: fptr(49.99)},
		{Workflow: "nil"},
	})

	ApplyBudget(res, &budget)

	assertBoolPtr(t, res.TopBurners[0].OverBudget, true)
	assertBoolPtr(t, res.TopBurners[1].OverBudget, false)
	assertBoolPtr(t, res.TopBurners[2].OverBudget, false)
	assertBoolPtr(t, res.TopBurners[3].OverBudget, false)
}

func TestConsumptionResult_JSONBudgetFields(t *testing.T) {
	noBudget := budgetTestResult(
		[]ConsumptionGroup{{Key: "repo", AIC: fptr(1)}},
		[]WorkflowConsumption{{Workflow: "wf", AIC: fptr(1)}},
	)
	rawNoBudget, err := json.Marshal(noBudget)
	if err != nil {
		t.Fatalf("Marshal(no budget): %v", err)
	}
	sNoBudget := string(rawNoBudget)
	for _, forbidden := range []string{`"budget"`, `"over_budget"`} {
		if strings.Contains(sNoBudget, forbidden) {
			t.Fatalf("no-budget JSON contains %s; want omitted: %s", forbidden, sNoBudget)
		}
	}

	budget := 5.0
	withBudget := budgetTestResult(
		[]ConsumptionGroup{
			{Key: "over", AIC: fptr(5.01)},
			{Key: "under", AIC: fptr(4.99)},
		},
		[]WorkflowConsumption{
			{Workflow: "top-over", AIC: fptr(7)},
			{Workflow: "top-under", AIC: fptr(3)},
		},
	)
	ApplyBudget(withBudget, &budget)

	rawWithBudget, err := json.Marshal(withBudget)
	if err != nil {
		t.Fatalf("Marshal(with budget): %v", err)
	}
	sWithBudget := string(rawWithBudget)
	for _, want := range []string{
		`"budget":5`,
		`"key":"over"`,
		`"over_budget":true`,
		`"key":"under"`,
		`"over_budget":false`,
		`"workflow":"top-over"`,
		`"workflow":"top-under"`,
	} {
		if !strings.Contains(sWithBudget, want) {
			t.Fatalf("budget JSON missing %s: %s", want, sWithBudget)
		}
	}
}
