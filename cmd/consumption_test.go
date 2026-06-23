package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
)

func renderConsumptionTextFixture(t *testing.T, by fleet.GroupByKind, res *fleet.ConsumptionResult) string {
	t.Helper()
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := renderConsumptionText(cmd, by, res); err != nil {
		t.Fatalf("renderConsumptionText: %v", err)
	}
	return out.String()
}

func writeEmptyFleetConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	content := `{"version":1,"profiles":{},"repos":{}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "fleet.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("write fleet.json: %v", err)
	}
	return dir
}

func withAggregateConsumptionStub(
	t *testing.T,
	stub func(context.Context, *fleet.Config, fleet.FetchMode, fleet.GroupByKind, fleet.SourceKind) (*fleet.ConsumptionResult, []fleet.Diagnostic, error),
) {
	t.Helper()
	prev := aggregateConsumption
	t.Cleanup(func() { aggregateConsumption = prev })
	aggregateConsumption = stub
}

// TestConsumption_MutualExclusion covers FR-004: cobra rejects combinations
// of --latest / --trailing / --since with a "mutually exclusive" message.
func TestConsumption_MutualExclusion(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"latest+trailing", []string{"consumption", "--latest", "--trailing", "7d"}},
		{"latest+since", []string{"consumption", "--latest", "--since", "2026-04-01"}},
		{"trailing+since", []string{"consumption", "--trailing", "7d", "--since", "2026-04-01"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := NewRootCmd()
			var out, errBuf bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errBuf)
			root.SetArgs(tc.args)
			err := root.Execute()
			if err == nil {
				t.Fatalf("Execute: expected error; got nil. stdout=%q stderr=%q", out.String(), errBuf.String())
			}
			// Cobra's MarkFlagsMutuallyExclusive message names the group with all three flag names.
			if !strings.Contains(err.Error(), "none of the others can be") {
				t.Errorf("error %q does not signal mutual exclusion", err.Error())
			}
			for _, name := range []string{"latest", "trailing", "since"} {
				if !strings.Contains(err.Error(), name) {
					t.Errorf("error %q does not name flag %q", err.Error(), name)
				}
			}
		})
	}
}

// TestConsumption_InvalidByFlag covers FR-005: --by rejects values outside
// the closed set with a message naming all four valid axes.
func TestConsumption_InvalidByFlag(t *testing.T) {
	root := NewRootCmd()
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"consumption", "--by", "tier"})
	err := root.Execute()
	if err == nil {
		t.Fatalf("Execute: expected error; got nil")
	}
	msg := err.Error()
	for _, want := range []string{"repo", "profile", "cost-center", "workflow"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not name valid axis %q", msg, want)
		}
	}
}

// TestConsumption_InvalidTrailing covers the --trailing parser's error path.
func TestConsumption_InvalidTrailing(t *testing.T) {
	root := NewRootCmd()
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"consumption", "--trailing", "7h"})
	err := root.Execute()
	if err == nil {
		t.Fatalf("Execute: expected error; got nil")
	}
	if !strings.Contains(err.Error(), "Nd") {
		t.Errorf("error %q does not name the accepted form (Nd)", err.Error())
	}
}

// TestConsumption_InvalidSince covers the --since parser's error path.
func TestConsumption_InvalidSince(t *testing.T) {
	root := NewRootCmd()
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"consumption", "--since", "not-a-date"})
	err := root.Execute()
	if err == nil {
		t.Fatalf("Execute: expected error; got nil")
	}
	if !strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Errorf("error %q does not name the accepted form (YYYY-MM-DD)", err.Error())
	}
}

func TestRenderConsumptionText_BudgetColumn(t *testing.T) {
	budget := 50.0
	over, under := true, false
	res := &fleet.ConsumptionResult{
		Budget: &budget,
		Groups: []fleet.ConsumptionGroup{
			{
				Key: "rshade/over", GitHubAPICalls: 10, SafeOutputCalls: 1, AIC: f64ptr(50.01),
				Cost: f64ptr(0.50), ReportCount: 1, OverBudget: &over,
			},
			{
				Key: "rshade/under", GitHubAPICalls: 5, SafeOutputCalls: 0, AIC: f64ptr(50),
				Cost: f64ptr(0.49), ReportCount: 1, OverBudget: &under,
			},
		},
	}

	out := renderConsumptionTextFixture(t, fleet.GroupByRepo, res)

	if !strings.Contains(out, "REPORTS  OVER") {
		t.Fatalf("output missing OVER header:\n%s", out)
	}
	if !strings.Contains(out, "rshade/over") || !strings.Contains(out, "!") {
		t.Fatalf("output missing over-budget marker:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "rshade/under") && strings.Contains(line, "!") {
			t.Fatalf("under-budget row unexpectedly marked:\n%s", out)
		}
	}

	noBudget := *res
	noBudget.Budget = nil
	for i := range noBudget.Groups {
		noBudget.Groups[i].OverBudget = nil
	}
	noBudgetOut := renderConsumptionTextFixture(t, fleet.GroupByRepo, &noBudget)
	if strings.Contains(noBudgetOut, "OVER") || strings.Contains(noBudgetOut, "!") {
		t.Fatalf("no-budget output contains budget annotation:\n%s", noBudgetOut)
	}
}

func TestConsumption_BudgetFlagValidation(t *testing.T) {
	t.Run("negative budget rejected before config loading", func(t *testing.T) {
		root := NewRootCmd()
		var out, errBuf bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errBuf)
		root.SetArgs([]string{"--dir", t.TempDir(), "consumption", "--budget", "-1"})

		err := root.Execute()

		if err == nil {
			t.Fatal("Execute: expected negative budget error; got nil")
		}
		if !strings.Contains(err.Error(), "--budget") || !strings.Contains(err.Error(), "non-negative") {
			t.Fatalf("error %q does not describe non-negative --budget", err.Error())
		}
	})

	t.Run("zero budget accepted", func(t *testing.T) {
		root := NewRootCmd()
		var out, errBuf bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errBuf)
		root.SetArgs([]string{
			"--dir", writeEmptyFleetConfig(t),
			"consumption", "--source", "artifacts", "--budget", "0",
		})

		if err := root.Execute(); err != nil {
			t.Fatalf("Execute: %v\nstdout=%s\nstderr=%s", err, out.String(), errBuf.String())
		}
	})

	t.Run("non-numeric budget rejected by cobra", func(t *testing.T) {
		root := NewRootCmd()
		var out, errBuf bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errBuf)
		root.SetArgs([]string{"consumption", "--budget", "abc"})

		err := root.Execute()

		if err == nil {
			t.Fatal("Execute: expected parse error; got nil")
		}
		if !strings.Contains(err.Error(), `invalid argument "abc"`) {
			t.Fatalf("error %q does not report invalid float", err.Error())
		}
	})
}

func TestConsumption_BudgetBreachesExitZero(t *testing.T) {
	cases := []struct {
		name   string
		budget string
	}{
		{name: "zero breaches", budget: "200"},
		{name: "some breaches", budget: "50"},
		{name: "all breaches", budget: "0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := NewRootCmd()
			withAggregateConsumptionStub(
				t,
				func(
					_ context.Context,
					_ *fleet.Config,
					_ fleet.FetchMode,
					_ fleet.GroupByKind,
					_ fleet.SourceKind,
				) (*fleet.ConsumptionResult, []fleet.Diagnostic, error) {
					return &fleet.ConsumptionResult{
						LoadedFrom: "fleet.json",
						FetchMode:  "latest",
						GroupBy:    "repo",
						Source:     "artifacts",
						Groups: []fleet.ConsumptionGroup{
							{Key: "a/one", AIC: f64ptr(25), ReportCount: 1},
							{Key: "b/two", AIC: f64ptr(100), ReportCount: 1},
						},
					}, nil, nil
				},
			)
			var out, errBuf bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errBuf)
			root.SetArgs([]string{
				"--dir", writeEmptyFleetConfig(t),
				"consumption", "--source", "artifacts", "--budget", tc.budget,
			})

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute: %v\nstdout=%s\nstderr=%s", err, out.String(), errBuf.String())
			}
		})
	}
}

func TestRenderConsumptionText_TopBurnersBudgetColumn(t *testing.T) {
	budget := 50.0
	over, under := true, false
	res := &fleet.ConsumptionResult{
		Budget: &budget,
		TopBurners: []fleet.WorkflowConsumption{
			{
				Workflow: "top-over", Runs: 3, APICalls: 30,
				AvgDurationS: 12.5, AIC: f64ptr(60),
				Cost: f64ptr(0.60), OverBudget: &over,
			},
			{
				Workflow: "top-under", Runs: 1, APICalls: 10,
				AvgDurationS: 3.5, AIC: f64ptr(40),
				Cost: f64ptr(0.40), OverBudget: &under,
			},
		},
	}

	out := renderConsumptionTextFixture(t, fleet.GroupByRepo, res)

	if !strings.Contains(out, "AVG_DURATION  AIC    COST   OVER") {
		t.Fatalf("top-burners output missing OVER header:\n%s", out)
	}
	if !strings.Contains(out, "top-over") || !strings.Contains(out, "!") {
		t.Fatalf("top-burners output missing marker:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "top-under") && strings.Contains(line, "!") {
			t.Fatalf("under-budget top burner unexpectedly marked:\n%s", out)
		}
	}
}

func TestConsumption_JSONNegativeBudgetEnvelope(t *testing.T) {
	root := NewRootCmd()
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"--output", "json", "--dir", t.TempDir(), "consumption", "--budget", "-1"})

	err := root.Execute()

	if err == nil {
		t.Fatal("Execute: expected negative budget error; got nil")
	}
	var env Envelope
	if decodeErr := json.Unmarshal(out.Bytes(), &env); decodeErr != nil {
		t.Fatalf("decode envelope: %v\nstdout=%s", decodeErr, out.String())
	}
	if env.SchemaVersion != SchemaVersion {
		t.Fatalf("schema_version = %d; want %d", env.SchemaVersion, SchemaVersion)
	}
	if env.Result != nil {
		t.Fatalf("result = %#v; want nil", env.Result)
	}
	if len(env.Hints) == 0 || !strings.Contains(env.Hints[0].Message, "--budget") {
		t.Fatalf("hints = %+v; want --budget failure hint", env.Hints)
	}
}

func TestConsumption_SchemaVersionUnchangedForBudget(t *testing.T) {
	root := NewRootCmd()
	withAggregateConsumptionStub(
		t,
		func(
			_ context.Context,
			_ *fleet.Config,
			_ fleet.FetchMode,
			_ fleet.GroupByKind,
			_ fleet.SourceKind,
		) (*fleet.ConsumptionResult, []fleet.Diagnostic, error) {
			return &fleet.ConsumptionResult{
				LoadedFrom: "fleet.json",
				FetchMode:  "latest",
				GroupBy:    "repo",
				Source:     "artifacts",
				Groups: []fleet.ConsumptionGroup{
					{Key: "a/one", AIC: f64ptr(25), ReportCount: 1},
				},
			}, nil, nil
		},
	)
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{
		"--output", "json", "--dir", writeEmptyFleetConfig(t),
		"consumption", "--source", "artifacts", "--budget", "1",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v\nstdout=%s\nstderr=%s", err, out.String(), errBuf.String())
	}
	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\nstdout=%s", err, out.String())
	}
	if env.SchemaVersion != SchemaVersion {
		t.Fatalf("schema_version = %d; want %d", env.SchemaVersion, SchemaVersion)
	}
}

func f64ptr(v float64) *float64 { return &v }
