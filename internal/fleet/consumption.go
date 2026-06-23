package fleet

// The consumption-rollup layer aggregates per-repo api-consumption-report
// output across the fleet.
//
// Discovery: `gh api repos/{owner}/{repo}/discussions --paginate`, filtered
// to category.slug=="audits" plus the stable tracker-id HTML-comment marker.
// Data: `gh api repos/{owner}/{repo}/actions/runs/{run_id}/artifacts` plus
// the per-artifact `/zip` endpoint; the zip carries aw_info.json and
// run_summary.json. Both layers go through package-level injection seams
// (ghDiscussionsAPI, ghRunArtifactAPI) so tests run offline.

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Layer 1 — discovery (unexported)
// ---------------------------------------------------------------------------

// reportRef is one row of the discovery layer's index. It identifies a
// consumption discussion, the workflow run it links to, and the temporal-
// filter metadata used by shouldIncludeReport.
type reportRef struct {
	Repo       string    // "owner/name" tracked in fleet config
	RunID      int64     // captured from the actions/runs/{id}/agentic_workflow link
	Date       time.Time // parsed from the discussion title (YYYY-MM-DD)
	Expires    time.Time // parsed from <!-- gh-aw-expires: ISO --> marker
	InProgress bool      // true when body contains "🔄 in-progress"
	URL        string    // discussion html_url, for diagnostic-warning copy
}

// FetchKind is the sum-type discriminant for FetchMode.
type FetchKind int

const (
	// FetchLatest selects the most-recent valid report per repo.
	FetchLatest FetchKind = iota
	// FetchTrailing selects all reports within the trailing Nd window.
	FetchTrailing
	// FetchSince selects all reports on or after Since.
	FetchSince
)

// discussionJSON is the subset of fields consumed from `gh api .../discussions`.
// Fields not listed (user, reactions, comments, ...) are present in the raw
// response but ignored.
type discussionJSON struct {
	Number   int    `json:"number"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	HTMLURL  string `json:"html_url"`
	Category struct {
		Slug string `json:"slug"`
	} `json:"category"`
}

// awInfoPayload is the subset of aw_info.json consumed by the consumption
// layer. See contracts/run-artifact-payload.md §"Payload shape consumed".
type awInfoPayload struct {
	GithubRateLimitUsage struct {
		CoreConsumed int `json:"core_consumed"`
	} `json:"github_rate_limit_usage"`
	SafeOutputs struct {
		TotalCalls int `json:"total_calls"`
	} `json:"safe_outputs"`
	// Cost as *float64 distinguishes absent (nil) from present-and-zero.
	Cost *float64 `json:"cost,omitempty"`
}

// runSummaryPayload is the subset of run_summary.json consumed by the
// consumption layer. See contracts/run-artifact-payload.md.
type runSummaryPayload struct {
	Workflows []struct {
		Name               string   `json:"name"`
		Runs               int      `json:"runs"`
		APICalls           int      `json:"api_calls"`
		AvgDurationSeconds float64  `json:"avg_duration_seconds"`
		Cost               *float64 `json:"cost,omitempty"`
	} `json:"workflows"`
}

// artifactPayload bundles the two decoded JSON payloads returned by the
// ghRunArtifactAPI seam. Tests substitute fixture-loaded payloads here.
type artifactPayload struct {
	AWInfo     awInfoPayload
	RunSummary runSummaryPayload
}

// ---------------------------------------------------------------------------
// Layer 2 — FetchMode (CLI-built)
// ---------------------------------------------------------------------------

// FetchMode is the one-of selector populated from the mutually-exclusive
// --latest / --trailing / --since flags. The CLI layer validates exactly one
// is set and constructs this; helpers downstream assume the invariant.
type FetchMode struct {
	// Kind is which arm is active.
	Kind FetchKind
	// Days is populated when Kind == FetchTrailing; > 0.
	Days int
	// Since is populated when Kind == FetchSince; UTC midnight of the input date.
	Since time.Time
}

// GroupByKind is the closed set of --by axes accepted by AggregateConsumption.
// The CLI layer parses the user-facing string into one of these values via
// ParseGroupBy; helpers downstream switch on the kind exhaustively.
type GroupByKind int

const (
	// GroupByRepo groups rolled totals by "owner/name".
	GroupByRepo GroupByKind = iota
	// GroupByProfile groups by profile name; multi-profile repos contribute
	// additively (FR-014).
	GroupByProfile
	// GroupByCostCenter groups by RepoSpec.CostCenter; the unset value folds
	// into the "<unset>" bucket (FR-015).
	GroupByCostCenter
	// GroupByWorkflow groups by workflow name pulled from per-workflow run
	// summaries.
	GroupByWorkflow
)

// Axis vocabulary for --by. Defined as named constants (rather than inline
// literals in groupByNames) so the CLI-axis "repo" stays semantically
// decoupled from the fieldRepo structured-logging key, which happens to
// share the same value; the goconst linter folds inline literals into
// fieldRepo otherwise.
const (
	axisRepo       = "repo"
	axisProfile    = "profile"
	axisCostCenter = "cost-center"
	axisWorkflow   = "workflow"
)

// groupByNames is the canonical CLI-vocabulary mapping for GroupByKind,
// indexed by kind. String and ParseGroupBy both read from this single source
// of truth so each axis literal appears exactly once in the package.
//
//nolint:gochecknoglobals // immutable lookup table; Go has no const arrays.
var groupByNames = [...]string{
	GroupByRepo:       axisRepo,
	GroupByProfile:    axisProfile,
	GroupByCostCenter: axisCostCenter,
	GroupByWorkflow:   axisWorkflow,
}

// String returns the CLI-vocabulary name for the kind, or "" when k is out
// of range. Surfaced on ConsumptionResult.GroupBy in the JSON envelope.
func (k GroupByKind) String() string {
	if int(k) < 0 || int(k) >= len(groupByNames) {
		return ""
	}
	return groupByNames[k]
}

// ParseGroupBy returns the GroupByKind for the canonical name s, or an
// error naming the valid axes when s is outside the closed set (FR-005).
func ParseGroupBy(s string) (GroupByKind, error) {
	for k, name := range groupByNames {
		if name == s {
			return GroupByKind(k), nil
		}
	}
	return 0, fmt.Errorf(
		"invalid --by value %q: expected one of repo, profile, cost-center, workflow", s,
	)
}

// ---------------------------------------------------------------------------
// Layer 3 — Reports (parsed) and Result (aggregated)
// ---------------------------------------------------------------------------

// ConsumptionReport is one repo-day's worth of attribution data. Built from
// a discovered reportRef plus the workflow-run artifacts it points to.
// Profile and CostCenter are joined from fleet config at aggregation time,
// not at fetch time — a freshly-parsed report carries empty Profile and
// CostCenter strings until AggregateConsumption populates them.
type ConsumptionReport struct {
	Repo            string    `json:"repo"`
	Date            time.Time `json:"date"`
	RunID           int64     `json:"run_id"`
	GitHubAPICalls  int       `json:"github_api_calls"`
	SafeOutputCalls int       `json:"safe_output_calls"`
	// AIC is the AI-credit spend under the Copilot model (logs source only;
	// nil under the artifact source, which has no AIC field). USD Cost is
	// derived as AIC * 0.01.
	AIC         *float64              `json:"aic,omitempty"`
	Cost        *float64              `json:"cost,omitempty"`
	PerWorkflow []WorkflowConsumption `json:"per_workflow"`
	Profile     string                `json:"profile,omitempty"`
	CostCenter  string                `json:"cost_center,omitempty"`
}

// WorkflowConsumption is one row in the per-workflow breakdown table. Used
// both as the unit inside ConsumptionReport.PerWorkflow and as the standalone
// row type emitted into ConsumptionResult.TopBurners.
type WorkflowConsumption struct {
	Workflow     string   `json:"workflow"`
	Runs         int      `json:"runs"`
	APICalls     int      `json:"api_calls"`
	AvgDurationS float64  `json:"avg_duration_s"`
	AIC          *float64 `json:"aic,omitempty"`
	Cost         *float64 `json:"cost,omitempty"`
	// OverBudget is present when a budget ceiling was supplied and reports
	// whether this workflow row strictly exceeded that ceiling.
	OverBudget *bool `json:"over_budget,omitempty"`
}

// ConsumptionGroup is one aggregated row in the consumption rollup. The Key
// field's meaning depends on the GroupBy axis on the parent result:
//   - GroupBy == "repo":         Key is "owner/name"
//   - GroupBy == "profile":      Key is the profile name (e.g., "standard")
//   - GroupBy == "cost-center":  Key is the cost-center value, or "<unset>"
//   - GroupBy == "workflow":     Key is the workflow name
//
// SafeOutputCalls is meaningful only for the repo / profile / cost-center
// axes — the run-summary artifact carries no per-workflow safe-output count,
// so workflow rows always report zero. See addWorkflowToGroup for context.
type ConsumptionGroup struct {
	Key             string   `json:"key"`
	GitHubAPICalls  int      `json:"github_api_calls"`
	SafeOutputCalls int      `json:"safe_output_calls"`
	AIC             *float64 `json:"aic,omitempty"`
	Cost            *float64 `json:"cost,omitempty"`
	ReportCount     int      `json:"report_count"`
	// OverBudget is present when a budget ceiling was supplied and reports
	// whether this group row strictly exceeded that ceiling.
	OverBudget *bool `json:"over_budget,omitempty"`
}

// ConsumptionResult is the JSON envelope payload for `gh-aw-fleet
// consumption`. Slice fields are normalized to non-nil empty slices by
// initSlices (cmd/output.go) so JSON marshaling renders them as [].
type ConsumptionResult struct {
	LoadedFrom string `json:"loaded_from"`
	FetchMode  string `json:"fetch_mode"`
	GroupBy    string `json:"group_by"`
	Source     string `json:"source,omitempty"`
	// Budget is the optional AIC ceiling supplied by the operator.
	Budget     *float64              `json:"budget,omitempty"`
	Groups     []ConsumptionGroup    `json:"groups"`
	TopBurners []WorkflowConsumption `json:"top_burners"`
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// trailingRe matches the accepted `Nd` shape for --trailing. Days are the
// only supported unit (research.md Decision 4).
var trailingRe = regexp.MustCompile(`^(\d+)d$`)

// runIDRe captures the workflow run ID from the standard agentic-workflow link.
var runIDRe = regexp.MustCompile(`/actions/runs/(\d+)/agentic_workflow`)

// expiresRe captures the ISO timestamp from the gh-aw-expires marker.
var expiresRe = regexp.MustCompile(`<!-- gh-aw-expires:\s*([^\s]+)\s*-->`)

// dateRe matches the first YYYY-MM-DD substring in a discussion title.
var dateRe = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

// ParseTrailing parses a `Nd` string into the integer day count. Any other
// shape returns an error naming the accepted form. Exported for the cmd
// layer to call from --trailing flag parsing.
func ParseTrailing(s string) (int, error) {
	m := trailingRe.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("--trailing value %q invalid: expected Nd (e.g. 7d)", s)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("--trailing value %q invalid: expected positive Nd (e.g. 7d)", s)
	}
	return n, nil
}

// normalizeCost applies the nil-on-non-positive rule (research.md Decision 6).
// Absent (nil), zero, and negative values all collapse to nil so downstream
// jq filters can `select(.cost != null)` cleanly.
func normalizeCost(raw *float64) *float64 {
	if raw == nil || *raw <= 0 {
		return nil
	}
	return raw
}

// ---------------------------------------------------------------------------
// Discovery seam + implementation
// ---------------------------------------------------------------------------

//nolint:gochecknoglobals // test-injection seam for gh api discussion list
var ghDiscussionsAPI = func(ctx context.Context, repo string) ([]discussionJSON, error) {
	path := fmt.Sprintf("repos/%s/discussions", repo)
	cmd := exec.CommandContext(ctx, "gh", "api", "--paginate",
		"-H", "Accept: application/vnd.github+json", path)
	out, err := runLoggedOutput(cmd, "gh", "api", map[string]string{fieldPath: path})
	if err != nil {
		return nil, ghErr(err)
	}
	// gh api --paginate emits one JSON array per page, concatenated without a
	// merging wrapper (gh api --help: "Each page is a separate" document).
	// A single json.Unmarshal only decodes the first page, so we stream every
	// document with json.Decoder and flatten.
	dec := json.NewDecoder(bytes.NewReader(out))
	var payload []discussionJSON
	for dec.More() {
		var page []discussionJSON
		if decodeErr := dec.Decode(&page); decodeErr != nil {
			return nil, fmt.Errorf("decode gh api discussions: %w", decodeErr)
		}
		payload = append(payload, page...)
	}
	return payload, nil
}

const consumptionTrackerMarker = "<!-- gh-aw-tracker-id: api-consumption-report-daily -->"

// discoverReports queries the repo's discussions and returns one reportRef
// per surviving record (audits-category, tracker-marker present, mandatory
// markers parseable). Returns diagnostics for malformed records — soft
// failure, the rollup continues.
//
// The returned slice is sorted by Date descending so FetchLatest can take
// element 0 without re-sorting.
func discoverReports(ctx context.Context, repo string) ([]reportRef, []Diagnostic, error) {
	raw, err := ghDiscussionsAPI(ctx, repo)
	if err != nil {
		return nil, nil, err
	}
	var (
		refs  []reportRef
		diags []Diagnostic
	)
	for _, d := range raw {
		if d.Category.Slug != "audits" {
			continue
		}
		if !strings.Contains(d.Body, consumptionTrackerMarker) {
			continue
		}
		ref, diag, ok := parseDiscussion(repo, d)
		if diag != nil {
			diags = append(diags, *diag)
		}
		if !ok {
			continue
		}
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].Date.After(refs[j].Date)
	})
	return refs, diags, nil
}

// parseDiscussion extracts the four body markers from a candidate
// discussion. Returns ok=false (with a soft-failure diagnostic) when a
// mandatory marker is missing or unparseable.
func parseDiscussion(repo string, d discussionJSON) (reportRef, *Diagnostic, bool) {
	runM := runIDRe.FindStringSubmatch(d.Body)
	if runM == nil {
		return reportRef{}, newSoftDiagnostic(
			repo,
			fmt.Sprintf("Discussion #%d on %s contains no actions/runs/{id}/agentic_workflow link — skipping",
				d.Number, repo),
		), false
	}
	runID, err := strconv.ParseInt(runM[1], 10, 64)
	if err != nil {
		return reportRef{}, newSoftDiagnostic(
			repo,
			fmt.Sprintf("Discussion #%d on %s has malformed run-ID %q — skipping",
				d.Number, repo, runM[1]),
		), false
	}
	dateToken := dateRe.FindString(d.Title)
	if dateToken == "" {
		return reportRef{}, newSoftDiagnostic(
			repo,
			fmt.Sprintf("Discussion #%d on %s title %q does not contain a YYYY-MM-DD date — skipping",
				d.Number, repo, d.Title),
		), false
	}
	date, err := time.Parse("2006-01-02", dateToken)
	if err != nil {
		return reportRef{}, newSoftDiagnostic(
			repo,
			fmt.Sprintf("Discussion #%d on %s date %q unparseable — skipping",
				d.Number, repo, dateToken),
		), false
	}
	ref := reportRef{
		Repo:       repo,
		RunID:      runID,
		Date:       date,
		InProgress: strings.Contains(d.Body, "🔄 in-progress"),
		URL:        d.HTMLURL,
	}
	expM := expiresRe.FindStringSubmatch(d.Body)
	if expM == nil {
		msg := fmt.Sprintf(
			"Discussion #%d on %s contains no <!-- gh-aw-expires: ISO --> marker — "+
				"treating as never-expiring (validity cannot be determined)",
			d.Number, repo,
		)
		return ref, newSoftDiagnostic(repo, msg), true
	}
	exp, err := time.Parse(time.RFC3339, expM[1])
	if err != nil {
		msg := fmt.Sprintf(
			"Discussion #%d on %s expires marker %q unparseable — treating as never-expiring",
			d.Number, repo, expM[1],
		)
		return ref, newSoftDiagnostic(repo, msg), true
	}
	ref.Expires = exp
	return ref, nil, true
}

// newSoftDiagnostic constructs a DiagHint Diagnostic with the canonical
// Fields shape: hint and repo. Used by parseDiscussion's many soft-failure
// paths so they don't repeat the struct literal four times.
func newSoftDiagnostic(repo, msg string) *Diagnostic {
	return &Diagnostic{
		Code:    DiagHint,
		Message: msg,
		Fields:  map[string]any{fieldHint: msg, fieldRepo: repo},
	}
}

// ---------------------------------------------------------------------------
// Artifact-fetch seam + implementation
// ---------------------------------------------------------------------------

// artifactRef is one entry of a run's artifact listing (the subset consumed:
// numeric id and name). Decoded from `gh api .../actions/runs/{id}/artifacts`.
type artifactRef struct {
	// ID is the artifact's numeric identifier used to build the /zip path.
	ID int64 `json:"id"`
	// Name is the GitHub Actions artifact name (e.g. "activation").
	Name string `json:"name"`
}

// awInfoArtifactNames lists the artifact names that carry aw_info.json, most
// preferred first. gh-aw v5+ bundles aw_info.json inside the multi-file
// "activation" artifact; pre-v5 used a single-file "aw-info" artifact. The
// "aw_info" underscore form is kept last as a defensive fallback. Verified
// against a captured live run artifact (rshade/finfocus run 27241899611).
//
//nolint:gochecknoglobals // immutable name-precedence table; Go has no const slices.
var awInfoArtifactNames = []string{artifactNameActivation, artifactNameAWInfo, "aw_info"}

// artifactNameActivation is the gh-aw v5+ multi-file artifact that bundles
// aw_info.json (see awInfoArtifactNames).
const artifactNameActivation = "activation"

// artifactNameAWInfo is the pre-v5 single-file artifact that carried
// aw_info.json (see awInfoArtifactNames).
const artifactNameAWInfo = "aw-info"

// runSummaryArtifactNames lists the artifact names that may carry
// run_summary.json, most preferred first. Standard agentic runs do not upload
// one (run_summary.json is a local `gh aw audit` cache); when absent the
// per-workflow breakdown is simply empty.
//
//nolint:gochecknoglobals // immutable name-precedence table; Go has no const slices.
var runSummaryArtifactNames = []string{"run_summary", "run-summary"}

// selectArtifactIDs picks the aw_info and run_summary artifact IDs from a run's
// artifact listing, honoring the precedence in awInfoArtifactNames /
// runSummaryArtifactNames (earlier entries win when a run carries more than one
// candidate). The returned awInfoID and runSummaryID are 0 when that kind is
// absent.
func selectArtifactIDs(artifacts []artifactRef) (int64, int64) {
	var awInfoID, runSummaryID int64
	bestAW, bestRS := -1, -1
	for _, a := range artifacts {
		if r := nameRank(awInfoArtifactNames, a.Name); r >= 0 && (bestAW < 0 || r < bestAW) {
			awInfoID, bestAW = a.ID, r
		}
		if r := nameRank(runSummaryArtifactNames, a.Name); r >= 0 && (bestRS < 0 || r < bestRS) {
			runSummaryID, bestRS = a.ID, r
		}
	}
	return awInfoID, runSummaryID
}

// nameRank returns the index of name in names, or -1 when absent. Lower index
// means higher precedence.
func nameRank(names []string, name string) int {
	for i, n := range names {
		if n == name {
			return i
		}
	}
	return -1
}

//nolint:gochecknoglobals // test-injection seam for gh api run-artifact fetch
var ghRunArtifactAPI = func(ctx context.Context, repo string, runID int64) (artifactPayload, error) {
	listPath := fmt.Sprintf("repos/%s/actions/runs/%d/artifacts", repo, runID)
	listCmd := exec.CommandContext(ctx, "gh", "api", listPath)
	listOut, err := runLoggedOutput(listCmd, "gh", "api", map[string]string{fieldPath: listPath})
	if err != nil {
		return artifactPayload{}, ghErr(err)
	}
	var listing struct {
		Artifacts []artifactRef `json:"artifacts"`
	}
	if decodeErr := json.Unmarshal(listOut, &listing); decodeErr != nil {
		return artifactPayload{}, fmt.Errorf("decode gh api artifacts list: %w", decodeErr)
	}
	awInfoID, runSummaryID := selectArtifactIDs(listing.Artifacts)
	out := artifactPayload{}
	if awInfoID != 0 {
		dec, decErr := downloadAndDecodeAWInfo(ctx, repo, awInfoID)
		if decErr != nil {
			return artifactPayload{}, decErr
		}
		out.AWInfo = dec
	}
	if runSummaryID != 0 {
		dec, decErr := downloadAndDecodeRunSummary(ctx, repo, runSummaryID)
		if decErr != nil {
			return artifactPayload{}, decErr
		}
		out.RunSummary = dec
	}
	return out, nil
}

func downloadAndDecodeAWInfo(ctx context.Context, repo string, artifactID int64) (awInfoPayload, error) {
	body, err := downloadArtifactZip(ctx, repo, artifactID)
	if err != nil {
		return awInfoPayload{}, err
	}
	raw, err := readZipFile(body, "aw_info.json")
	if err != nil {
		return awInfoPayload{}, fmt.Errorf("read aw_info.json: %w", err)
	}
	var p awInfoPayload
	if decErr := json.Unmarshal(raw, &p); decErr != nil {
		return awInfoPayload{}, fmt.Errorf("decode aw_info.json: %w", decErr)
	}
	return p, nil
}

func downloadAndDecodeRunSummary(ctx context.Context, repo string, artifactID int64) (runSummaryPayload, error) {
	body, err := downloadArtifactZip(ctx, repo, artifactID)
	if err != nil {
		return runSummaryPayload{}, err
	}
	raw, err := readZipFile(body, "run_summary.json")
	if err != nil {
		return runSummaryPayload{}, fmt.Errorf("read run_summary.json: %w", err)
	}
	var p runSummaryPayload
	if decErr := json.Unmarshal(raw, &p); decErr != nil {
		return runSummaryPayload{}, fmt.Errorf("decode run_summary.json: %w", decErr)
	}
	return p, nil
}

func downloadArtifactZip(ctx context.Context, repo string, artifactID int64) ([]byte, error) {
	path := fmt.Sprintf("repos/%s/actions/artifacts/%d/zip", repo, artifactID)
	// The artifact /zip endpoint replies with a 302 to blob storage; `gh api`
	// follows it and streams the zip body. Do NOT send
	// `Accept: application/octet-stream` — GitHub now rejects that on this
	// endpoint with HTTP 415 ("Must accept 'application/json'"). The default
	// `gh api` Accept header negotiates the redirect correctly.
	cmd := exec.CommandContext(ctx, "gh", "api", path)
	out, err := runLoggedOutput(cmd, "gh", "api", map[string]string{fieldPath: path})
	if err != nil {
		return nil, ghErr(err)
	}
	return out, nil
}

// readZipFile pulls one file by basename out of an in-memory zip body. The
// artifact /zip body holds files at the archive root (e.g. the "activation"
// artifact carries aw_info.json directly), but we match on basename suffix so
// any future nesting still resolves.
func readZipFile(body []byte, basename string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("zip reader: %w", err)
	}
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, basename) {
			rc, openErr := f.Open()
			if openErr != nil {
				return nil, openErr
			}
			defer func() { _ = rc.Close() }()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("file %q not found in zip", basename)
}

// fetchRunArtifacts retrieves and decodes the aw_info / run_summary artifact
// payload for one report. Returns a populated ConsumptionReport (with
// Profile/CostCenter unset — joined by AggregateConsumption).
func fetchRunArtifacts(ctx context.Context, ref reportRef) (*ConsumptionReport, error) {
	payload, err := ghRunArtifactAPI(ctx, ref.Repo, ref.RunID)
	if err != nil {
		return nil, err
	}
	report := &ConsumptionReport{
		Repo:            ref.Repo,
		Date:            ref.Date,
		RunID:           ref.RunID,
		GitHubAPICalls:  payload.AWInfo.GithubRateLimitUsage.CoreConsumed,
		SafeOutputCalls: payload.AWInfo.SafeOutputs.TotalCalls,
		Cost:            normalizeCost(payload.AWInfo.Cost),
		PerWorkflow:     toWorkflowConsumption(payload.RunSummary),
	}
	return report, nil
}

func toWorkflowConsumption(summary runSummaryPayload) []WorkflowConsumption {
	out := make([]WorkflowConsumption, 0, len(summary.Workflows))
	for _, w := range summary.Workflows {
		out = append(out, WorkflowConsumption{
			Workflow:     w.Name,
			Runs:         w.Runs,
			APICalls:     w.APICalls,
			AvgDurationS: w.AvgDurationSeconds,
			Cost:         normalizeCost(w.Cost),
		})
	}
	return out
}

// unsetCostCenter is the literal bucket key for repos that declare no
// cost_center value when grouping --by cost-center (FR-015). Angle brackets
// distinguish it from a real cost-center value named "unset".
const unsetCostCenter = "<unset>"

// ---------------------------------------------------------------------------
// Filter — shouldIncludeReport
// ---------------------------------------------------------------------------

// shouldIncludeReport applies the per-ref temporal + status filter. Returns
// include=true/false and an optional per-ref diagnostic. The caller is
// responsible for the per-repo "every candidate filtered" warning (T017).
//
// Decision logic:
//   - Expired (now.After(Expires)): always excluded; no per-ref diagnostic
//     (the per-repo aggregate emits one, see T017).
//   - In-progress + FetchLatest: excluded; per-ref hint emitted suggesting
//     --trailing 7d if the operator wants partial coverage.
//   - In-progress + FetchTrailing/FetchSince: included; per-ref warning
//     emitted that totals may be partial (FR-012).
//   - Window predicate (trailing/since): if outside the window, excluded;
//     no per-ref diagnostic.
func shouldIncludeReport(ref reportRef, mode FetchMode, now time.Time) (bool, *Diagnostic) {
	if !ref.Expires.IsZero() && now.After(ref.Expires) {
		return false, nil
	}
	switch mode.Kind {
	case FetchLatest:
		if ref.InProgress {
			return false, inProgressSkippedDiag(ref)
		}
		return true, nil
	case FetchTrailing:
		cutoff := now.Add(-time.Duration(mode.Days) * 24 * time.Hour)
		if ref.Date.Before(cutoff) {
			return false, nil
		}
		if ref.InProgress {
			return true, inProgressIncludedDiag(ref)
		}
		return true, nil
	case FetchSince:
		if ref.Date.Before(mode.Since) {
			return false, nil
		}
		if ref.InProgress {
			return true, inProgressIncludedDiag(ref)
		}
		return true, nil
	}
	return false, nil
}

// inProgressSkippedDiag is the per-ref hint emitted by shouldIncludeReport
// when a FetchLatest invocation filters out an in-progress report.
func inProgressSkippedDiag(ref reportRef) *Diagnostic {
	return newSoftDiagnostic(ref.Repo, fmt.Sprintf(
		"Skipping in-progress report from %s (%s) — pass --trailing 7d to include in-progress reports",
		ref.Repo, ref.Date.Format("2006-01-02"),
	))
}

// inProgressIncludedDiag is the per-ref warning emitted by
// shouldIncludeReport when a FetchTrailing/FetchSince invocation includes an
// in-progress report (FR-012).
func inProgressIncludedDiag(ref reportRef) *Diagnostic {
	return newSoftDiagnostic(ref.Repo, fmt.Sprintf(
		"Included in-progress report from %s (%s). Totals for this repo may be partial.",
		ref.Repo, ref.Date.Format("2006-01-02"),
	))
}

// ---------------------------------------------------------------------------
// Aggregation — AggregateConsumption
// ---------------------------------------------------------------------------

// ScopeToRepos returns a shallow copy of cfg whose Repos map is restricted to
// the named repos, preserving Profiles, Defaults, and LoadedFrom so profile
// and cost-center grouping still resolve against the full fleet definition. An
// empty names slice returns cfg unchanged (the whole-fleet default). When a
// single requested repo is absent it returns ErrRepoNotTracked (which names the
// config file to edit), matching deploy/sync/upgrade; when several are absent it
// lists every offender.
func ScopeToRepos(cfg *Config, names []string) (*Config, error) {
	if len(names) == 0 {
		return cfg, nil
	}
	scoped := make(map[string]RepoSpec, len(names))
	var missing []string
	for _, n := range names {
		spec, ok := cfg.Repos[n]
		if !ok {
			missing = append(missing, n)
			continue
		}
		scoped[n] = spec
	}
	switch len(missing) {
	case 0:
	case 1:
		return nil, ErrRepoNotTracked(missing[0], cfg.LoadedFrom)
	default:
		return nil, fmt.Errorf("repos not tracked in fleet config: %s", strings.Join(missing, ", "))
	}
	out := *cfg
	out.Repos = scoped
	return &out, nil
}

// AggregateConsumption walks every repo in cfg in sorted order, discovers
// each repo's consumption reports, applies the temporal/status filter,
// fetches the matching run artifacts, and assembles a ConsumptionResult
// grouped by the requested axis.
//
// The four supported axes are GroupByRepo, GroupByProfile, GroupByCostCenter,
// and GroupByWorkflow. Multi-profile repos contribute additively to every
// profile group (research.md Decision 5, FR-014). Repos with no cost-center
// land under the "<unset>" bucket (FR-015).
//
// Diagnostics surface per-repo issues (no reports discovered, every
// candidate filtered, retention-expired artifact fetch) and per-ref issues
// (in-progress included with caveat).
func AggregateConsumption(
	ctx context.Context,
	cfg *Config,
	mode FetchMode,
	by GroupByKind,
	source SourceKind,
) (*ConsumptionResult, []Diagnostic, error) {
	if source == SourceLogs {
		if err := ensureLogsSourceGhAwVersion(ctx); err != nil {
			return nil, nil, err
		}
	}

	now := time.Now().UTC()
	repoNames := make([]string, 0, len(cfg.Repos))
	for r := range cfg.Repos {
		repoNames = append(repoNames, r)
	}
	sort.Strings(repoNames)

	groups := map[string]*ConsumptionGroup{}
	var diags []Diagnostic
	var allReports []*ConsumptionReport

	for _, repo := range repoNames {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, diags, ctxErr
		}
		repoReports, repoDiags := collectReportsForSource(ctx, repo, mode, source, now)
		diags = append(diags, repoDiags...)
		spec := cfg.Repos[repo]
		if by == GroupByProfile && len(spec.Profiles) == 0 && len(repoReports) > 0 {
			diags = append(diags, *newSoftDiagnostic(repo, fmt.Sprintf(
				"Skipping %s under --by profile: repo declares no profiles in fleet config — "+
					"its consumption rolls into no group. Set a profile on the repo to include it.",
				repo,
			)))
		}
		for _, rep := range repoReports {
			rep.CostCenter = spec.CostCenter
			allReports = append(allReports, rep)
			addReportToGroups(groups, rep, spec, by)
		}
	}

	result := &ConsumptionResult{
		LoadedFrom: cfg.LoadedFrom,
		FetchMode:  formatFetchMode(mode),
		GroupBy:    by.String(),
		Source:     source.String(),
		Groups:     materializeGroups(groups),
		TopBurners: buildTopBurners(allReports),
	}
	if hint := sourceEmptyDataHint(source, result.Groups); hint != nil {
		diags = append(diags, *hint)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, diags, ctxErr
	}
	return result, diags, nil
}

// allCostNil reports whether every group's rolled-up Cost is nil — the
// expected state for a Copilot-engine fleet (see nilCostDiag). Called only
// when len(groups) > 0 so an all-nil empty result does not trigger the hint.
func allCostNil(groups []ConsumptionGroup) bool {
	for i := range groups {
		if groups[i].Cost != nil {
			return false
		}
	}
	return true
}

// nilCostDiag explains an entirely empty COST column: the Copilot engine bills
// in AI credits / premium requests, not USD, so gh-aw's aw_info.json reports no
// positive cost and normalizeCost (Decision 6) collapses every value to nil. A
// populated COST column requires an engine that emits total_cost_usd — e.g. the
// Claude engine with a metered Anthropic key. Emitted once per run, fleet-wide
// (no repo field), only when reports were found but none carried a cost.
func nilCostDiag() Diagnostic {
	msg := "Cost is unavailable for every report. Workflows on the GitHub Copilot " +
		"engine are billed in AI credits / premium requests, which gh-aw does not " +
		"report as USD — so the COST column stays empty. A populated cost requires " +
		"an engine that emits total_cost_usd (e.g. the Claude engine with a metered " +
		"Anthropic key)."
	return Diagnostic{Code: DiagHint, Message: msg, Fields: map[string]any{fieldHint: msg}}
}

// collectRepoReports runs discovery + filter + fetch for one repo and
// returns the included reports + diagnostics. Emits a per-repo warning when
// discovery is empty (FR-010) or every candidate is filtered (FR-011).
func collectRepoReports(
	ctx context.Context,
	repo string,
	mode FetchMode,
	now time.Time,
) ([]*ConsumptionReport, []Diagnostic) {
	var diags []Diagnostic
	refs, parseDiags, err := discoverReports(ctx, repo)
	diags = append(diags, parseDiags...)
	if err != nil {
		msg := fmt.Sprintf("Discovery failed for %s: %v", repo, err)
		diags = append(diags, *newSoftDiagnostic(repo, msg))
		return nil, diags
	}
	if len(refs) == 0 {
		msg := fmt.Sprintf(
			"No consumption reports discovered for %s — the api-consumption-report workflow "+
				"is either not deployed to this repo or has not yet produced a daily report.",
			repo,
		)
		diags = append(diags, *newSoftDiagnostic(repo, msg))
		return nil, diags
	}

	var included []reportRef
	for _, ref := range refs {
		ok, refDiag := shouldIncludeReport(ref, mode, now)
		if refDiag != nil {
			diags = append(diags, *refDiag)
		}
		if !ok {
			continue
		}
		included = append(included, ref)
		if mode.Kind == FetchLatest {
			break
		}
	}
	if len(included) == 0 {
		diags = append(diags, *newSoftDiagnostic(repo, allFilteredMessage(repo, mode)))
		return nil, diags
	}

	reports := make([]*ConsumptionReport, 0, len(included))
	for _, ref := range included {
		rep, fetchErr := fetchRunArtifacts(ctx, ref)
		if fetchErr != nil {
			hints := CollectHintDiagnostics(fetchErr.Error())
			if len(hints) == 0 {
				msg := fmt.Sprintf(
					"Run artifact for %s (run #%d on %s) could not be fetched: %v",
					repo, ref.RunID, ref.Date.Format("2006-01-02"), fetchErr,
				)
				diags = append(diags, *newSoftDiagnostic(repo, msg))
			} else {
				diags = append(diags, hints...)
			}
			continue
		}
		if rep == nil {
			continue
		}
		reports = append(reports, rep)
	}
	return reports, diags
}

// allFilteredMessage builds the per-repo "no surviving reports" diagnostic
// copy. The "--trailing 7d" alternative only makes sense under FetchLatest;
// suggesting it while the user is already on --trailing 30d or --since would
// be misleading.
func allFilteredMessage(repo string, mode FetchMode) string {
	msg := fmt.Sprintf("No valid consumption reports for %s after filtering.", repo)
	if mode.Kind == FetchLatest {
		msg += " Try --trailing 7d to include in-progress or older reports."
	}
	return msg
}

// addReportToGroups attributes one report's totals into the rollup groups
// according to the by axis.
func addReportToGroups(groups map[string]*ConsumptionGroup, rep *ConsumptionReport, spec RepoSpec, by GroupByKind) {
	switch by {
	case GroupByRepo:
		addToGroup(groups, rep.Repo, rep)
	case GroupByProfile:
		if len(spec.Profiles) == 0 {
			return
		}
		for _, p := range spec.Profiles {
			addToGroup(groups, p, rep)
		}
	case GroupByCostCenter:
		key := spec.CostCenter
		if key == "" {
			key = unsetCostCenter
		}
		addToGroup(groups, key, rep)
	case GroupByWorkflow:
		for _, wf := range rep.PerWorkflow {
			addWorkflowToGroup(groups, wf, rep)
		}
	}
}

// addToGroup folds one full report into the group keyed by key.
func addToGroup(groups map[string]*ConsumptionGroup, key string, rep *ConsumptionReport) {
	g, ok := groups[key]
	if !ok {
		g = &ConsumptionGroup{Key: key, Cost: zeroIfPresent(rep.Cost), AIC: zeroIfPresent(rep.AIC)}
		groups[key] = g
	}
	g.GitHubAPICalls += rep.GitHubAPICalls
	g.SafeOutputCalls += rep.SafeOutputCalls
	g.ReportCount++
	mergeCost(g, rep.Cost)
	mergeAIC(g, rep.AIC)
}

// addWorkflowToGroup folds one workflow's per-row consumption into the
// group keyed by workflow name. ReportCount counts reports, not runs.
//
// SafeOutputCalls intentionally stays at zero for workflow groups: the
// per-workflow run summary carries no safe-output count, only a fleet-wide
// total exists on the report, and adding that total to every workflow row
// would over-count by (workflows-per-report - 1)×. The SAFE_WRITES column
// is meaningful only for repo / profile / cost-center axes.
func addWorkflowToGroup(groups map[string]*ConsumptionGroup, wf WorkflowConsumption, _ *ConsumptionReport) {
	g, ok := groups[wf.Workflow]
	if !ok {
		g = &ConsumptionGroup{Key: wf.Workflow, Cost: zeroIfPresent(wf.Cost), AIC: zeroIfPresent(wf.AIC)}
		groups[wf.Workflow] = g
	}
	g.GitHubAPICalls += wf.APICalls
	g.ReportCount++
	mergeCost(g, wf.Cost)
	mergeAIC(g, wf.AIC)
}

// zeroIfPresent returns a pointer-to-zero when src is non-nil so the group
// starts in a state where mergeCost can accumulate, or nil to keep the
// all-or-nothing rule active.
func zeroIfPresent(src *float64) *float64 {
	if src == nil {
		return nil
	}
	z := 0.0
	return &z
}

// mergeFloat implements the all-or-nothing accumulation rule on a *float64
// group field: once the field is nil — or a contributing report's value is nil
// — the field stays nil; otherwise values accumulate. Shared by the Cost and
// AIC rollups so both honor identical nil semantics.
func mergeFloat(dst **float64, src *float64) {
	if *dst == nil {
		return
	}
	if src == nil {
		*dst = nil
		return
	}
	**dst += *src
}

// mergeCost applies the all-or-nothing rule to the group's Cost.
func mergeCost(g *ConsumptionGroup, src *float64) { mergeFloat(&g.Cost, src) }

// mergeAIC applies the all-or-nothing rule to the group's AIC. AIC is populated
// only under the logs source; the artifact source leaves it nil throughout.
func mergeAIC(g *ConsumptionGroup, src *float64) { mergeFloat(&g.AIC, src) }

// materializeGroups sorts the map by Key ascending and returns the slice
// for the ConsumptionResult envelope.
func materializeGroups(groups map[string]*ConsumptionGroup) []ConsumptionGroup {
	out := make([]ConsumptionGroup, 0, len(groups))
	for _, g := range groups {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// fetchModeLatest is the human-readable rendering of FetchMode{Kind: FetchLatest}
// emitted into ConsumptionResult.FetchMode.
const fetchModeLatest = "latest"

// formatFetchMode renders the FetchMode as the human-readable string
// emitted into ConsumptionResult.FetchMode.
func formatFetchMode(mode FetchMode) string {
	switch mode.Kind {
	case FetchLatest:
		return fetchModeLatest
	case FetchTrailing:
		return fmt.Sprintf("trailing-%dd", mode.Days)
	case FetchSince:
		return fmt.Sprintf("since-%s", mode.Since.Format("2006-01-02"))
	}
	return fetchModeLatest
}

// ---------------------------------------------------------------------------
// Top burners
// ---------------------------------------------------------------------------

// topBurnersCap is the maximum row count emitted into
// ConsumptionResult.TopBurners (FR-017 — "TOP 10 BURNERS" footer).
const topBurnersCap = 10

// workflowAcc holds running per-workflow totals while buildTopBurners folds
// in each report's PerWorkflow rows.
type workflowAcc struct {
	runs          int
	apiCalls      int
	costSum       float64
	costPresent   bool
	costMissing   bool
	aicSum        float64
	aicPresent    bool
	aicMissing    bool
	durationNum   float64
	durationDenom int
}

// buildTopBurners aggregates per-workflow consumption across every included
// report and returns the top-N descending where N == topBurnersCap. When
// every workflow has a populated Cost, sorts by Cost; otherwise by APICalls.
func buildTopBurners(reports []*ConsumptionReport) []WorkflowConsumption {
	totals := accumulateWorkflowTotals(reports)
	out, allCostPresent := materializeTopBurners(totals)
	sortTopBurners(out, allCostPresent)
	if len(out) > topBurnersCap {
		out = out[:topBurnersCap]
	}
	return out
}

// accumulateWorkflowTotals folds every report's PerWorkflow rows into a
// per-workflow accumulator keyed by workflow name.
func accumulateWorkflowTotals(reports []*ConsumptionReport) map[string]*workflowAcc {
	totals := map[string]*workflowAcc{}
	for _, rep := range reports {
		for _, wf := range rep.PerWorkflow {
			a, ok := totals[wf.Workflow]
			if !ok {
				a = &workflowAcc{}
				totals[wf.Workflow] = a
			}
			a.runs += wf.Runs
			a.apiCalls += wf.APICalls
			a.durationNum += wf.AvgDurationS * float64(wf.Runs)
			a.durationDenom += wf.Runs
			if wf.AIC == nil {
				a.aicMissing = true
			} else {
				a.aicSum += *wf.AIC
				a.aicPresent = true
			}
			if wf.Cost == nil {
				a.costMissing = true
				continue
			}
			a.costSum += *wf.Cost
			a.costPresent = true
		}
	}
	return totals
}

// materializeTopBurners turns the accumulator map into a slice plus the
// "every workflow has a cost" flag used to choose the sort key.
func materializeTopBurners(totals map[string]*workflowAcc) ([]WorkflowConsumption, bool) {
	out := make([]WorkflowConsumption, 0, len(totals))
	allCostPresent := len(totals) > 0
	for name, a := range totals {
		wc := WorkflowConsumption{Workflow: name, Runs: a.runs, APICalls: a.apiCalls}
		if a.durationDenom > 0 {
			wc.AvgDurationS = a.durationNum / float64(a.durationDenom)
		}
		if a.costPresent && !a.costMissing {
			cost := a.costSum
			wc.Cost = &cost
		} else {
			allCostPresent = false
		}
		if a.aicPresent && !a.aicMissing {
			aic := a.aicSum
			wc.AIC = &aic
		}
		out = append(out, wc)
	}
	return out, allCostPresent
}

// sortTopBurners sorts in place descending by Cost when every entry has one,
// otherwise descending by APICalls, with Workflow name as the tiebreaker.
func sortTopBurners(out []WorkflowConsumption, allCostPresent bool) {
	sort.Slice(out, func(i, j int) bool {
		if allCostPresent && out[i].Cost != nil && out[j].Cost != nil && *out[i].Cost != *out[j].Cost {
			return *out[i].Cost > *out[j].Cost
		}
		if out[i].APICalls != out[j].APICalls {
			return out[i].APICalls > out[j].APICalls
		}
		return out[i].Workflow < out[j].Workflow
	})
}
