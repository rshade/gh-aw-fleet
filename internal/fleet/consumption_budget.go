package fleet

// ApplyBudget annotates res in place with over-budget markers when budget is
// non-nil. A row is over budget iff its AIC is non-nil and strictly greater
// than *budget; nil-AIC rows and rows equal to the ceiling are never marked.
// When budget is nil the function is a no-op, preserving byte-identical output
// for invocations without --budget.
func ApplyBudget(res *ConsumptionResult, budget *float64) {
	if res == nil || budget == nil {
		return
	}
	res.Budget = budget
	for i := range res.Groups {
		res.Groups[i].OverBudget = overBudgetPtr(res.Groups[i].AIC, *budget)
	}
	for i := range res.TopBurners {
		res.TopBurners[i].OverBudget = overBudgetPtr(res.TopBurners[i].AIC, *budget)
	}
}

func overBudgetPtr(aic *float64, budget float64) *bool {
	over := aic != nil && *aic > budget
	return &over
}
