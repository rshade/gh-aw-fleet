package security

import (
	"context"
	"os"

	"github.com/rshade/gh-aw-fleet/internal/fleet/frontmatter"
)

// costScanner evaluates each workflow's YAML frontmatter for high-frequency
// triggers (push, check_run, check_suite), medium-frequency reactive
// triggers (pull_request, issues, issue_comment), and recurring schedule
// triggers that lack a skip-if-match or skip-if-no-match pre-activation
// guard. Pre-activation guards let the gh-aw runtime cancel a workflow
// cheaply — before incurring Copilot-credit or Actions-minute cost — when a
// condition is not met (e.g. no relevant files changed). Without a guard,
// every matching event unconditionally burns credits. All rules are
// advisory-only (never blocking). The costScanner silently skips files with
// unparseable frontmatter; the structural scanner already emits a parse-error
// INFO finding for those.
type costScanner struct {
	rules []rule
}

// newCostScanner constructs the cost trigger-risk scanner. Rule evaluation
// functions are closures over no mutable state, so a single costScanner
// instance is safe for concurrent use.
func newCostScanner() *costScanner {
	return &costScanner{rules: costRules()}
}

// Scan walks <cloneDir>/.github/workflows/*.md, parses each workflow's YAML
// frontmatter, and runs every cost rule against it. Returns nil when the
// workflows directory does not exist.
func (s *costScanner) Scan(_ context.Context, cloneDir string) []Finding {
	var out []Finding
	for _, w := range walkWorkflows(cloneDir, ".md") {
		out = append(out, s.scanFile(w)...)
	}
	return out
}

func (s *costScanner) scanFile(w walkEntry) []Finding {
	content, err := os.ReadFile(w.Full)
	if err != nil {
		return nil
	}
	fmText, _ := frontmatter.Split(string(content))
	fm, parseErr := frontmatter.Parse(fmText)
	if parseErr != nil {
		// Silently skip files with malformed frontmatter — the structural
		// scanner already emits a parse-error INFO finding for them.
		return nil
	}
	var out []Finding
	for _, r := range s.rules {
		for _, h := range r.Eval(fm) {
			out = append(out, hitToFinding(r, h, w.Rel))
		}
	}
	return out
}

// highFrequencyTriggerSet is the set of trigger names that drive the
// highest Copilot-credit burn per the gh-aw cost-management docs. These
// fire on every commit (push) or every CI status update (check_run,
// check_suite), potentially dozens of times per PR on active repos.
//
//nolint:gochecknoglobals // immutable lookup set
var highFrequencyTriggerSet = map[string]bool{
	"push":        true,
	"check_run":   true,
	"check_suite": true,
}

// reactiveTriggerSet covers the medium-cost event-driven triggers that fire
// more frequently than workflow_dispatch but less than the high tier. Each PR
// typically generates multiple pull_request events; issues and issue_comment
// fire on every comment.
//
//nolint:gochecknoglobals // immutable lookup set
var reactiveTriggerSet = map[string]bool{
	"pull_request":        true,
	"pull_request_target": true,
	"issues":              true,
	"issue_comment":       true,
}

// scheduledTriggerSet covers recurring timer-driven workflows. Schedules are
// lower-frequency than push/check events, but still burn credits on idle repos
// unless bounded by a pre-activation guard.
//
//nolint:gochecknoglobals // immutable lookup set
var scheduledTriggerSet = map[string]bool{
	"schedule": true,
}

// costRules returns the cost-risk lint rules. Each rule is independent:
// a workflow with push, pull_request, and schedule triggers and no skip guard
// fires every applicable rule simultaneously.
func costRules() []rule {
	return []rule{
		highFrequencyTriggerRule(),
		reactiveNoSkipGuardRule(),
		scheduledNoSkipGuardRule(),
	}
}

// noSkipGuardRule builds a cost rule that fires when the workflow declares any
// trigger in set but lacks a skip-if-match or skip-if-no-match guard. All
// cost rules share this Eval shape; only the trigger set, severity, and
// human-readable strings differ.
func noSkipGuardRule(id string, sev Severity, msg, remedy string, set map[string]bool) rule {
	return rule{
		ID: id, Severity: sev, Message: msg, Remedy: remedy,
		Eval: func(fm map[string]any) []ruleHit {
			if !hasTriggerInSet(fm["on"], set) {
				return nil
			}
			if hasSkipGuard(fm) {
				return nil
			}
			return []ruleHit{{Line: 0}}
		},
	}
}

// highFrequencyTriggerRule fires MEDIUM when the workflow declares a push,
// check_run, or check_suite trigger without a skip-if-match or
// skip-if-no-match pre-activation guard. These are the dominant cost drivers
// named in the gh-aw cost-management reference: they fire on every commit or
// CI event without a guard to short-circuit the credit burn.
func highFrequencyTriggerRule() rule {
	return noSkipGuardRule(
		ruleIDCostHighFrequencyTrigger,
		SeverityMedium,
		"workflow uses a high-frequency trigger (push / check_run / check_suite) without a skip-if-match or skip-if-no-match guard",
		"Add skip-if-match or skip-if-no-match to the workflow frontmatter. The pre-activation job exits early when the guard matches, saving Copilot credits and Actions minutes before any AI call is made.",
		highFrequencyTriggerSet,
	)
}

// reactiveNoSkipGuardRule fires LOW when the workflow declares a medium-cost
// reactive trigger (pull_request, pull_request_target, issues, or
// issue_comment) without a skip-if-match or skip-if-no-match guard. Adding a
// guard lets the pre-activation job cancel cheaply when no relevant activity
// has occurred (e.g. the PR touches only docs).
func reactiveNoSkipGuardRule() rule {
	return noSkipGuardRule(
		ruleIDCostReactiveNoSkipGuard,
		SeverityLow,
		"reactive workflow (pull_request / pull_request_target / issues / issue_comment) lacks a skip-if-match or skip-if-no-match guard",
		"Add skip-if-match or skip-if-no-match to the workflow frontmatter. The pre-activation job exits early when the guard matches, saving Copilot credits and Actions minutes before any AI call is made.",
		reactiveTriggerSet,
	)
}

// scheduledNoSkipGuardRule fires LOW when the workflow declares a schedule
// trigger without a skip-if-match or skip-if-no-match guard. Scheduled
// workflows can burn credits on quiet repos because the timer fires regardless
// of whether there is useful work to perform.
func scheduledNoSkipGuardRule() rule {
	return noSkipGuardRule(
		ruleIDCostScheduledNoSkipGuard,
		SeverityLow,
		"scheduled workflow lacks a skip-if-match or skip-if-no-match guard",
		"Add skip-if-match or skip-if-no-match under the workflow's on: block so scheduled runs can exit before any AI call when there is no useful work to perform.",
		scheduledTriggerSet,
	)
}

// hasTriggerInSet reports whether the `on:` value contains any trigger name
// present in set. Handles the three YAML forms GitHub Actions accepts: a bare
// string, a YAML sequence of strings, and a mapping whose keys are trigger
// names.
func hasTriggerInSet(v any, set map[string]bool) bool {
	switch t := v.(type) {
	case string:
		return set[t]
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok && set[s] {
				return true
			}
		}
	case map[string]any:
		for k := range t {
			if set[k] {
				return true
			}
		}
	}
	return false
}

// hasSkipGuard reports whether the frontmatter contains a substantive
// skip-if-match or skip-if-no-match guard either under the on: block (the
// gh-aw syntax) or at the top level (legacy/test fixture fallback). A key that
// is present but nil, an empty string, or an empty sequence does not constitute
// an active guard (the gh-aw runtime would treat it as a no-op).
func hasSkipGuard(fm map[string]any) bool {
	return hasSubstantiveSkipGuard(fm, "skip-if-match") || hasSubstantiveSkipGuard(fm, "skip-if-no-match")
}

func hasSubstantiveSkipGuard(fm map[string]any, key string) bool {
	if on, ok := fm["on"].(map[string]any); ok && isSubstantiveSkipKey(on[key]) {
		return true
	}
	return isSubstantiveSkipKey(fm[key])
}

// isSubstantiveSkipKey returns true when v is a non-nil, non-empty guard
// value. Unknown types (e.g. a future structured form) are treated as present.
func isSubstantiveSkipKey(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case string:
		return t != ""
	case []any:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	}
	return true
}
