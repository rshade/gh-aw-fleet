package fleet

// status.go implements the read-only drift-detection command. Status
// compares the declared workflow set (fleet.json + fleet.local.json)
// against each repo's actual .github/workflows/*.md `source:` frontmatter —
// without cloning. The orchestrator fans out across repos with a bounded
// worker pool; per-repo failures are isolated and surfaced as
// RepoStatus.ErrorMessage plus a sibling Diagnostic.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Drift states emitted on RepoStatus.DriftState. The closed set of values
// the JSON envelope's drift_state field promises to consumers (FR-006).
const (
	driftStateAligned = "aligned"
	driftStateDrifted = "drifted"
	driftStateErrored = "errored"
)

// StatusOpts controls a single Status() invocation.
type StatusOpts struct {
	// Repo is the optional positional argument. When non-empty, Status queries
	// only this repo (which MUST be declared in cfg.Repos). When empty,
	// Status queries every repo in the loaded fleet config.
	Repo string

	// fetcher is an in-package test seam. Nil means use the production gh api
	// fetcher; keeping it per-call avoids package-global races in parallel tests.
	fetcher statusFetcher
}

// StatusResult is the result payload embedded in the JSON envelope's
// `result` field for the `status` command. Single field today; left as a
// struct so future additions (aggregate counts, run timing) are non-breaking.
type StatusResult struct {
	// Repos is the per-repo drift report set, sorted alphabetically by Repo.
	Repos []RepoStatus `json:"repos"`
}

// RepoStatus is the drift report for a single repository. Exactly one
// RepoStatus is emitted per repo queried (success or failure). Categories
// are mutually exclusive: a workflow appears in at most one of
// missing / extra / drifted / unpinned per RepoStatus.
type RepoStatus struct {
	// Repo is the canonical owner/name from fleet.json.
	Repo string `json:"repo"`
	// DriftState is one of: driftStateAligned, "drifted", driftStateErrored.
	DriftState string `json:"drift_state"`
	// Missing lists declared workflow names absent from the repo.
	Missing []string `json:"missing"`
	// Extra lists gh-aw-managed workflows present on the repo but not declared.
	Extra []string `json:"extra"`
	// Drifted lists workflows present-but-different-ref versus desired.
	Drifted []WorkflowDrift `json:"drifted"`
	// Unpinned lists workflows present but lacking parseable `source:` frontmatter.
	Unpinned []string `json:"unpinned"`
	// ErrorMessage is empty unless DriftState == driftStateErrored.
	ErrorMessage string `json:"error_message"`
}

// WorkflowDrift describes one workflow whose installed source ref differs
// from what fleet.json declares. Used inside RepoStatus.Drifted.
type WorkflowDrift struct {
	// Name is the workflow basename, no .md suffix (e.g. "audit").
	Name string `json:"name"`
	// DesiredRef is the literal ref string from fleet.json (e.g. "v0.68.3").
	DesiredRef string `json:"desired_ref"`
	// ActualRef is the literal ref segment of the installed `source:`
	// frontmatter (the part after "@"). May be a tag, branch, or SHA.
	ActualRef string `json:"actual_ref"`
}

// statusJob is one work unit dequeued by a worker. Lives in the buffered
// jobs channel; never observed outside the worker pool.
type statusJob struct {
	repo     string
	declared []ResolvedWorkflow
	// resolveErr surfaces a cfg.ResolveRepoWorkflows failure (broken profile
	// reference, etc.). When non-nil, the worker short-circuits to an errored
	// RepoStatus rather than fetching anything.
	resolveErr error
}

// statusFetcher is the seam between Status() and the gh api primitives,
// used to inject fakes in tests. Production binding: a thin wrapper over
// ghAPIRaw and ghAPIJSON that returns parsed structures.
type statusFetcher interface {
	listWorkflowsDir(ctx context.Context, repo string) ([]string, error)
	fetchWorkflowBody(ctx context.Context, repo, file string) (string, error)
}

// statusWorkerPoolSize is the number of concurrent repo workers
// (clarification 1: 4–8).
const statusWorkerPoolSize = 6

// ghStatusFetcher is the production binding over `gh api`. Methods are
// safe for concurrent use because each call shells out to a fresh
// exec.Command, isolated by the Go runtime.
type ghStatusFetcher struct{}

// listWorkflowsDir returns the `.md` filenames under .github/workflows on
// the repo's default branch (no ?ref=, per research R2).
func (ghStatusFetcher) listWorkflowsDir(ctx context.Context, repo string) ([]string, error) {
	listing, err := ghAPIJSON(ctx, fmt.Sprintf("/repos/%s/contents/.github/workflows", repo))
	if err != nil {
		if isGitHubNotFound(err) {
			_, repoErr := ghAPIJSON(ctx, fmt.Sprintf("/repos/%s", repo))
			if repoErr == nil {
				return []string{}, nil
			}
			return nil, fmt.Errorf("list %s workflows: %w", repo, repoErr)
		}
		return nil, fmt.Errorf("list %s workflows: %w", repo, err)
	}
	entries, ok := listing.([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected contents response for %s", repo)
	}
	var out []string
	for _, e := range entries {
		m, entryOK := e.(map[string]any)
		if !entryOK {
			continue
		}
		typ, _ := m["type"].(string)
		name, _ := m["name"].(string)
		if typ != "file" || !strings.HasSuffix(name, ".md") {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func isGitHubNotFound(err error) bool {
	// Tracks gh 2.x stderr formatting as wrapped by ghErr ("HTTP 404").
	// Revisit if gh changes error text on a major version bump.
	return err != nil && strings.Contains(err.Error(), "HTTP 404")
}

// fetchWorkflowBody returns the raw markdown body of a single workflow file
// on the repo's default branch.
func (ghStatusFetcher) fetchWorkflowBody(ctx context.Context, repo, file string) (string, error) {
	body, err := ghAPIRaw(ctx, fmt.Sprintf("/repos/%s/contents/.github/workflows/%s", repo, file))
	if err != nil {
		return "", fmt.Errorf("fetch %s/%s: %w", repo, file, err)
	}
	return body, nil
}

// Status diffs declared (cfg) versus actual (fetched .github/workflows) for
// every repo in cfg, or just opts.Repo when set. Returns the wire payload
// plus a slice of structured per-repo / fleet-wide diagnostics that the CLI
// layer splits into the envelope's warnings[] and hints[]. The third return
// is non-nil for setup-time failures (single-repo arg not in cfg) OR when
// ctx is canceled mid-fan-out; per-repo fetch failures NEVER surface there.
// On ctx cancellation the StatusResult contains whatever per-repo work
// completed before the cancel was observed (callers should treat as partial).
func Status(ctx context.Context, cfg *Config, opts StatusOpts) (*StatusResult, []Diagnostic, error) {
	repos, err := selectRepos(cfg, opts)
	if err != nil {
		return nil, nil, err
	}

	jobs := buildStatusJobs(cfg, repos)
	fetcher := opts.fetcher
	if fetcher == nil {
		fetcher = ghStatusFetcher{}
	}

	var diags []Diagnostic
	if len(cfg.Repos) == 0 {
		diags = append(diags, Diagnostic{
			Code:    DiagEmptyFleet,
			Message: "fleet config declares zero repos; nothing to check",
		})
	}

	repoStatuses, repoDiags := runStatusWorkers(ctx, fetcher, jobs)
	diags = append(diags, repoDiags...)

	sort.Slice(repoStatuses, func(i, j int) bool { return repoStatuses[i].Repo < repoStatuses[j].Repo })
	sort.SliceStable(diags, func(i, j int) bool {
		ri, _ := diags[i].Fields["repo"].(string)
		rj, _ := diags[j].Fields["repo"].(string)
		return ri < rj
	})

	if ctxErr := ctx.Err(); ctxErr != nil {
		return &StatusResult{Repos: repoStatuses}, diags, ctxErr
	}
	return &StatusResult{Repos: repoStatuses}, diags, nil
}

// selectRepos resolves opts.Repo to the queryable set: either a single repo
// (validated in cfg) or every repo in cfg, alphabetically sorted. An empty
// cfg.Repos with no opts.Repo is a valid empty result plus DiagEmptyFleet;
// an explicit repo against that same config remains a setup-time validation
// error because the requested repo is not declared.
func selectRepos(cfg *Config, opts StatusOpts) ([]string, error) {
	if opts.Repo != "" {
		if _, ok := cfg.Repos[opts.Repo]; !ok {
			return nil, fmt.Errorf("repo %q is not declared in fleet config", opts.Repo)
		}
		return []string{opts.Repo}, nil
	}
	out := make([]string, 0, len(cfg.Repos))
	for r := range cfg.Repos {
		out = append(out, r)
	}
	sort.Strings(out)
	return out, nil
}

// buildStatusJobs pre-resolves declared workflows for each target repo so
// the workers don't need cfg access. A ResolveRepoWorkflows error is
// captured on the job and surfaced as an errored RepoStatus by the worker.
func buildStatusJobs(cfg *Config, repos []string) []statusJob {
	jobs := make([]statusJob, 0, len(repos))
	for _, repo := range repos {
		declared, err := cfg.ResolveRepoWorkflows(repo)
		jobs = append(jobs, statusJob{
			repo:       repo,
			declared:   declared,
			resolveErr: err,
		})
	}
	return jobs
}

// runStatusWorkers fans out per-repo work across statusWorkerPoolSize
// goroutines and collects RepoStatus + per-repo Diagnostic results.
func runStatusWorkers(
	ctx context.Context, fetcher statusFetcher, jobs []statusJob,
) ([]RepoStatus, []Diagnostic) {
	jobsCh := make(chan statusJob, len(jobs))
	resultsCh := make(chan RepoStatus, len(jobs))
	diagsCh := make(chan Diagnostic, len(jobs))

	for _, j := range jobs {
		jobsCh <- j
	}
	close(jobsCh)

	pool := max(min(statusWorkerPoolSize, len(jobs)), 1)

	var wg sync.WaitGroup
	for range pool {
		wg.Go(func() {
			for job := range jobsCh {
				if ctx.Err() != nil {
					return
				}
				rs := processJob(ctx, fetcher, job)
				resultsCh <- rs
				if rs.DriftState == driftStateErrored {
					diagsCh <- buildRepoErrorDiag(rs.Repo, rs.ErrorMessage)
				}
			}
		})
	}
	wg.Wait()
	close(resultsCh)
	close(diagsCh)

	statuses := make([]RepoStatus, 0, len(jobs))
	for rs := range resultsCh {
		statuses = append(statuses, rs)
	}
	diags := make([]Diagnostic, 0)
	for d := range diagsCh {
		diags = append(diags, d)
	}
	return statuses, diags
}

// processJob runs the per-repo logic for a single statusJob: surface a
// resolve-time error as errored, otherwise delegate to processRepo.
func processJob(ctx context.Context, fetcher statusFetcher, job statusJob) RepoStatus {
	if job.resolveErr != nil {
		return computeDrift(job.repo, nil, nil, nil, job.resolveErr)
	}
	return processRepo(ctx, fetcher, job.repo, job.declared)
}

// processRepo fetches the repo's workflow listing and the bodies needed to
// compute drift, then calls computeDrift. Per FR-018, workflow fetches
// within one repo are serial. Per FR-009, a single fetch failure surfaces
// as an errored RepoStatus rather than partial drift.
func processRepo(
	ctx context.Context, fetcher statusFetcher, repo string, declared []ResolvedWorkflow,
) RepoStatus {
	listing, err := fetcher.listWorkflowsDir(ctx, repo)
	if err != nil {
		return computeDrift(repo, declared, nil, nil, err)
	}

	declaredSet := map[string]bool{}
	for _, d := range declared {
		declaredSet[d.Name] = true
	}
	listingSet := map[string]bool{}
	for _, name := range listing {
		listingSet[name] = true
	}

	bodies := map[string]string{}
	// Fetch bodies for declared-and-present workflows AND for undeclared
	// workflows (so computeDrift can filter "extra" by parseable `source:`).
	for _, d := range declared {
		file := d.Name + ".md"
		if !listingSet[file] {
			continue
		}
		body, fetchErr := fetcher.fetchWorkflowBody(ctx, repo, file)
		if fetchErr != nil {
			return computeDrift(repo, declared, listing, nil, fetchErr)
		}
		bodies[file] = body
	}
	for _, file := range listing {
		name := strings.TrimSuffix(file, ".md")
		if declaredSet[name] {
			continue
		}
		body, fetchErr := fetcher.fetchWorkflowBody(ctx, repo, file)
		if fetchErr != nil {
			return computeDrift(repo, declared, listing, nil, fetchErr)
		}
		bodies[file] = body
	}

	return computeDrift(repo, declared, listing, bodies, nil)
}

// computeDrift is the pure, table-testable diff function. Given the
// declared set, the actual listing, and fetched bodies, it computes the
// per-repo drift report. No goroutines, no I/O.
func computeDrift(
	repo string,
	declared []ResolvedWorkflow,
	listing []string,
	fetchedBodies map[string]string,
	fetchErr error,
) RepoStatus {
	rs := RepoStatus{Repo: repo}
	if fetchErr != nil {
		rs.DriftState = driftStateErrored
		rs.ErrorMessage = fetchErr.Error()
		return rs
	}

	listingSet := map[string]bool{}
	for _, name := range listing {
		listingSet[name] = true
	}
	declaredSet := map[string]bool{}

	for _, d := range declared {
		declaredSet[d.Name] = true
		file := d.Name + ".md"
		if !listingSet[file] {
			rs.Missing = append(rs.Missing, d.Name)
			continue
		}
		actualRef, ok := readSourceRef(fetchedBodies[file])
		if !ok {
			rs.Unpinned = append(rs.Unpinned, d.Name)
			continue
		}
		if actualRef != d.Ref {
			rs.Drifted = append(rs.Drifted, WorkflowDrift{
				Name:       d.Name,
				DesiredRef: d.Ref,
				ActualRef:  actualRef,
			})
		}
	}

	for _, file := range listing {
		name := strings.TrimSuffix(file, ".md")
		if declaredSet[name] {
			continue
		}
		if _, ok := readSourceRef(fetchedBodies[file]); ok {
			rs.Extra = append(rs.Extra, name)
		}
	}

	sort.Strings(rs.Missing)
	sort.Strings(rs.Extra)
	sort.Strings(rs.Unpinned)
	sort.Slice(rs.Drifted, func(i, j int) bool { return rs.Drifted[i].Name < rs.Drifted[j].Name })

	if len(rs.Missing)+len(rs.Extra)+len(rs.Drifted)+len(rs.Unpinned) == 0 {
		rs.DriftState = driftStateAligned
	} else {
		rs.DriftState = driftStateDrifted
	}
	return rs
}

// readSourceRef parses a workflow body's frontmatter and returns the ref
// segment after "@" in the `source:` field. ok=false when the frontmatter
// is missing/malformed, the source field is missing/non-string, or the
// value lacks an "@" segment. Callers intentionally collapse all ok=false
// cases: declared workflows become unpinned, and undeclared workflows are
// ignored as not fleet-managed.
func readSourceRef(body string) (string, bool) {
	if body == "" {
		return "", false
	}
	fm, _ := SplitFrontmatter(body)
	parsed, err := ParseFrontmatter(fm)
	if err != nil {
		return "", false
	}
	raw, ok := parsed["source"].(string)
	if !ok {
		return "", false
	}
	at := strings.LastIndex(raw, "@")
	if at < 0 || at == len(raw)-1 {
		return "", false
	}
	return raw[at+1:], true
}

// buildRepoErrorDiag classifies a per-repo fetch failure and constructs the
// matching Diagnostic. Codes: rate_limited (substring match) or
// repo_inaccessible (default). Always includes Fields.repo for jq filtering.
func buildRepoErrorDiag(repo, errMsg string) Diagnostic {
	code := DiagRepoInaccessible
	message := errMsg
	if strings.Contains(errMsg, "API rate limit exceeded") {
		code = DiagRateLimited
		message = "GitHub API rate limit exceeded. Wait until the limit resets, or rotate to a different token."
	}
	return Diagnostic{
		Code:    code,
		Message: message,
		Fields:  map[string]any{"repo": repo},
	}
}
