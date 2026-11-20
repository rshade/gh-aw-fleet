package fleet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os/exec"
	"strings"
)

// failure-issue marker tokens. gh-aw files an open agentic-workflows issue per
// failing workflow whose trailing HTML comment carries these machine-readable
// fields; overview matches on them to detect model-not-supported failures.
const (
	// failureIssueMarker gates a body as a gh-aw failure issue (vs the sibling
	// "[aw] No-Op Runs" issue, which shares the label but lacks this marker).
	failureIssueMarker = "gh-aw-failure-issue: true"
	// failureCategoriesKey precedes the pipe-delimited failure-category list.
	failureCategoriesKey = "failure_categories:"
	// modelNotSupportedToken is the substring flagging an unsupported/deprecated
	// model; matches both model_not_supported and model_not_supported_error.
	modelNotSupportedToken = "model_not_supported" // #nosec G101 -- failure-category marker token, not a credential.
)

// Copilot model-resolution Actions-variable names — gh-aw's COPILOT_MODEL
// fallback chain (GH_AW_MODEL_AGENT_COPILOT || GH_AW_DEFAULT_MODEL_COPILOT ||
// baked default).
const (
	varAgentModelCopilot   = "GH_AW_MODEL_AGENT_COPILOT"
	varDefaultModelCopilot = "GH_AW_DEFAULT_MODEL_COPILOT"
)

// modelSupportRemediationHint returns the one-per-affected-repo remediation
// surfaced through the hints channel. It names controllingVar, the variable
// whose value is the failing model: GH_AW_MODEL_AGENT_COPILOT outranks
// GH_AW_DEFAULT_MODEL_COPILOT, so advising the default alone would change
// nothing when the agent variable is set. An empty controllingVar means the
// variables could not be read, so the hint spells out the precedence instead.
// A repo-level value always wins over an organization one, so "on the repo" is
// correct whichever level the controlling variable came from.
func modelSupportRemediationHint(controllingVar string) string {
	const (
		prefix = "agentic runs are failing on an unsupported/deprecated Copilot model. "
		target = " Actions variable on the repo to a model your Copilot subscription supports" +
			" (no recompile required)."
	)
	switch controllingVar {
	case varAgentModelCopilot:
		return prefix + "Set the " + varAgentModelCopilot + target + " It takes precedence over " +
			varDefaultModelCopilot + ", so changing only that variable has no effect."
	case varDefaultModelCopilot:
		return prefix + "Set the " + varDefaultModelCopilot + target
	default:
		return prefix + "Set the " + varDefaultModelCopilot + target + " If " + varAgentModelCopilot +
			" is set on the repo or its organization, it takes precedence: change it instead."
	}
}

// ghAwBakedDefaultCopilotModel is the model gh-aw compiles into COPILOT_MODEL
// when neither GH_AW_MODEL_AGENT_COPILOT nor GH_AW_DEFAULT_MODEL_COPILOT is set.
//
// MAINTENANCE TOUCH-POINT: this value MUST track the fleet's github/gh-aw source
// pin (fleet.json). gh-aw changes its baked default across releases — it dropped
// gpt-5-mini in v0.81.5 — so re-verify this constant on every github/gh-aw pin
// bump. Verified against the current pin; the compiled expression is
// `COPILOT_MODEL: ${{ vars.GH_AW_MODEL_AGENT_COPILOT || vars.GH_AW_DEFAULT_MODEL_COPILOT || 'claude-sonnet-4.6' }}`.
const ghAwBakedDefaultCopilotModel = "claude-sonnet-4.6"

// failureIssue is one open agentic-workflows failure issue for a repo, decoded
// from `gh issue list --json number,title,body`.
type failureIssue struct {
	// Number is the GitHub issue number.
	Number int `json:"number"`
	// Title is the issue title (e.g. "[aw] Code Simplifier failed").
	Title string `json:"title"`
	// Body carries the trailing gh-aw failure marker parsed by parseModelFailure.
	Body string `json:"body"`
}

// ghFailureIssuesAPI lists a repo's open agentic-workflows failure issues.
// Package-level seam so tests run offline, mirroring ghWorkflowsAPI/ghLogsAPI.
//
//nolint:gochecknoglobals // test-injection seam for `gh issue list`
var ghFailureIssuesAPI = func(ctx context.Context, repo string) ([]failureIssue, error) {
	args := []string{
		"issue", "list", flagRepo, repo,
		"--label", "agentic-workflows", "--state", "open",
		flagJSON, "number,title,body", "--limit", "100",
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	out, err := runLoggedOutput(cmd, "gh", "issue list", map[string]string{fieldRepo: repo})
	if err != nil {
		return nil, ghErr(err)
	}
	var issues []failureIssue
	if decErr := json.Unmarshal(out, &issues); decErr != nil {
		return nil, fmt.Errorf("decode gh issue list for %q: %w", repo, decErr)
	}
	return issues, nil
}

// actionsVariable is one Actions variable as returned by `gh variable list`
// and the organization-variables REST endpoint.
type actionsVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ghRepoVariablesAPI returns a repo's own (repository-level) Actions variables
// as name→value. Package-level seam; called only for flagged repos to minimize
// I/O.
//
//nolint:gochecknoglobals // test-injection seam for `gh variable list`
var ghRepoVariablesAPI = func(ctx context.Context, repo string) (map[string]string, error) {
	args := []string{"variable", "list", flagRepo, repo, flagJSON, "name,value"}
	cmd := exec.CommandContext(ctx, "gh", args...)
	out, err := runLoggedOutput(cmd, "gh", "variable list", map[string]string{fieldRepo: repo})
	if err != nil {
		return nil, ghErr(err)
	}
	var entries []actionsVariable
	if decErr := json.Unmarshal(out, &entries); decErr != nil {
		return nil, fmt.Errorf("decode gh variable list for %q: %w", repo, decErr)
	}
	return variablesByName(entries), nil
}

// ghOrgVariablesAPI returns the organization Actions variables shared with a
// repo as name→value. A user-owned repo has no organization, which the API
// reports as HTTP 422; that is an empty set, not an error. Package-level seam;
// called only for flagged repos.
//
//nolint:gochecknoglobals // test-injection seam for `gh api .../organization-variables`
var ghOrgVariablesAPI = func(ctx context.Context, repo string) (map[string]string, error) {
	path := fmt.Sprintf("repos/%s/actions/organization-variables", repo)
	cmd := exec.CommandContext(ctx, "gh", "api", "--paginate", path, "--jq", ".variables[] | {name, value}")
	out, err := runLoggedOutput(cmd, "gh", "api organization-variables", map[string]string{fieldRepo: repo})
	if err != nil {
		if wrapped := ghErr(err); !isGitHubValidationFailed(wrapped) {
			return nil, wrapped
		}
		return map[string]string{}, nil
	}
	var entries []actionsVariable
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var e actionsVariable
		if decErr := dec.Decode(&e); errors.Is(decErr, io.EOF) {
			break
		} else if decErr != nil {
			return nil, fmt.Errorf("decode organization variables for %q: %w", repo, decErr)
		}
		entries = append(entries, e)
	}
	return variablesByName(entries), nil
}

func isGitHubValidationFailed(err error) bool {
	// Tracks gh 2.x stderr formatting as wrapped by ghErr, like isGitHubNotFound.
	return err != nil && strings.Contains(err.Error(), "HTTP 422")
}

func variablesByName(entries []actionsVariable) map[string]string {
	vars := make(map[string]string, len(entries))
	for _, e := range entries {
		vars[e.Name] = e.Value
	}
	return vars
}

// mergeActionsVariables returns the variables a repo's Actions runs see:
// repository values override organization values of the same name, as GitHub
// resolves the vars context. Environment-level variables are out of scope
// because compiled gh-aw lock files declare no environment.
func mergeActionsVariables(org, repo map[string]string) map[string]string {
	merged := make(map[string]string, len(org)+len(repo))
	maps.Copy(merged, org)
	maps.Copy(merged, repo)
	return merged
}

// parseModelFailure reports whether a failure-issue body flags an
// unsupported/deprecated Copilot model. It is true iff the body carries the
// gh-aw failure marker AND its failure_categories value (from the key up to the
// next ',' or '-->') contains the model_not_supported token.
func parseModelFailure(body string) bool {
	if !strings.Contains(body, failureIssueMarker) {
		return false
	}
	idx := strings.Index(body, failureCategoriesKey)
	if idx < 0 {
		return false
	}
	value := categoriesValue(body[idx+len(failureCategoriesKey):])
	return strings.Contains(value, modelNotSupportedToken)
}

// categoriesValue trims a failure_categories value to the text before the next
// ',' (next marker field) or '-->' (marker close), whichever comes first.
func categoriesValue(s string) string {
	end := len(s)
	if i := strings.Index(s, ","); i >= 0 {
		end = i
	}
	if i := strings.Index(s, "-->"); i >= 0 && i < end {
		end = i
	}
	return s[:end]
}

// resolveEffectiveModel resolves a repo's effective Copilot model from its
// merged Actions variables using gh-aw's fallback chain:
// GH_AW_MODEL_AGENT_COPILOT, then GH_AW_DEFAULT_MODEL_COPILOT, then the baked
// default. Empty values are skipped so an explicitly-blank variable does not
// mask the next fallback. It returns the model and the controlling variable,
// the one an operator must set to change that model; the baked default is
// overridden through GH_AW_DEFAULT_MODEL_COPILOT.
func resolveEffectiveModel(vars map[string]string) (string, string) {
	if v := vars[varAgentModelCopilot]; v != "" {
		return v, varAgentModelCopilot
	}
	if v := vars[varDefaultModelCopilot]; v != "" {
		return v, varDefaultModelCopilot
	}
	return ghAwBakedDefaultCopilotModel, varDefaultModelCopilot
}
