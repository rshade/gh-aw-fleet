package fleet

import (
	"encoding/json"

	"github.com/rshade/gh-aw-fleet/internal/fleet/security"
)

type deployResultJSON struct {
	Repo                   string              `json:"repo"`
	CloneDir               string              `json:"clone_dir"`
	Added                  []WorkflowOutcome   `json:"added"`
	Skipped                []WorkflowOutcome   `json:"skipped"`
	Failed                 []WorkflowOutcome   `json:"failed"`
	InitWasRun             bool                `json:"init_was_run"`
	BranchPushed           string              `json:"branch_pushed"`
	PRURL                  string              `json:"pr_url"`
	MissingSecret          string              `json:"missing_secret"`
	SecretKeyURL           string              `json:"secret_key_url"`
	ActionsDisabled        bool                `json:"actions_disabled"`
	WorkflowTokenReadOnly  bool                `json:"workflow_token_read_only"`
	CompileStrictApplied   bool                `json:"compile_strict_applied"`
	CompileStrictEffective bool                `json:"compile_strict_effective"`
	CompileStrictSource    string              `json:"compile_strict_source"`
	SecurityFindings       *[]security.Finding `json:"security_findings,omitempty"`
}

// MarshalJSON encodes DeployResult with security_findings omitted when the
// scanner did not run and emitted as [] when it ran clean.
func (r DeployResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(deployResultJSON{
		Repo:                   r.Repo,
		CloneDir:               r.CloneDir,
		Added:                  nonNilWorkflowOutcomes(r.Added),
		Skipped:                nonNilWorkflowOutcomes(r.Skipped),
		Failed:                 nonNilWorkflowOutcomes(r.Failed),
		InitWasRun:             r.InitWasRun,
		BranchPushed:           r.BranchPushed,
		PRURL:                  r.PRURL,
		MissingSecret:          r.MissingSecret,
		SecretKeyURL:           r.SecretKeyURL,
		ActionsDisabled:        r.ActionsDisabled,
		WorkflowTokenReadOnly:  r.WorkflowTokenReadOnly,
		CompileStrictApplied:   r.CompileStrictApplied,
		CompileStrictEffective: r.CompileStrictEffective,
		CompileStrictSource:    r.CompileStrictSource,
		SecurityFindings:       optionalSecurityFindings(r.SecurityFindings),
	})
}

type syncResultJSON struct {
	Repo             string              `json:"repo"`
	CloneDir         string              `json:"clone_dir"`
	Missing          []string            `json:"missing"`
	Drift            []string            `json:"drift"`
	Expected         []string            `json:"expected"`
	Deploy           *DeployResult       `json:"deploy"`
	Pruned           []string            `json:"pruned"`
	DeployPreflight  *DeployResult       `json:"deploy_preflight"`
	SecurityFindings *[]security.Finding `json:"security_findings,omitempty"`
}

// MarshalJSON encodes SyncResult with security_findings omitted when the
// scanner did not run and emitted as [] when it ran clean.
func (r SyncResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(syncResultJSON{
		Repo:             r.Repo,
		CloneDir:         r.CloneDir,
		Missing:          nonNilStrings(r.Missing),
		Drift:            nonNilStrings(r.Drift),
		Expected:         nonNilStrings(r.Expected),
		Deploy:           r.Deploy,
		Pruned:           nonNilStrings(r.Pruned),
		DeployPreflight:  r.DeployPreflight,
		SecurityFindings: optionalSecurityFindings(r.SecurityFindings),
	})
}

type upgradeResultJSON struct {
	Repo                   string              `json:"repo"`
	CloneDir               string              `json:"clone_dir"`
	UpgradeOK              bool                `json:"upgrade_ok"`
	UpdateOK               bool                `json:"update_ok"`
	ChangedFiles           []string            `json:"changed_files"`
	Conflicts              []string            `json:"conflicts"`
	NoChanges              bool                `json:"no_changes"`
	BranchPushed           string              `json:"branch_pushed"`
	PRURL                  string              `json:"pr_url"`
	AuditJSON              json.RawMessage     `json:"audit_json"`
	OutputLog              string              `json:"output_log"`
	CompileStrictApplied   bool                `json:"compile_strict_applied"`
	CompileStrictEffective bool                `json:"compile_strict_effective"`
	CompileStrictSource    string              `json:"compile_strict_source"`
	SecurityFindings       *[]security.Finding `json:"security_findings,omitempty"`
}

// MarshalJSON encodes UpgradeResult with security_findings omitted when the
// scanner did not run and emitted as [] when it ran clean.
func (r UpgradeResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(upgradeResultJSON{
		Repo:                   r.Repo,
		CloneDir:               r.CloneDir,
		UpgradeOK:              r.UpgradeOK,
		UpdateOK:               r.UpdateOK,
		ChangedFiles:           nonNilStrings(r.ChangedFiles),
		Conflicts:              nonNilStrings(r.Conflicts),
		NoChanges:              r.NoChanges,
		BranchPushed:           r.BranchPushed,
		PRURL:                  r.PRURL,
		AuditJSON:              r.AuditJSON,
		OutputLog:              r.OutputLog,
		CompileStrictApplied:   r.CompileStrictApplied,
		CompileStrictEffective: r.CompileStrictEffective,
		CompileStrictSource:    r.CompileStrictSource,
		SecurityFindings:       optionalSecurityFindings(r.SecurityFindings),
	})
}

func optionalSecurityFindings(findings []security.Finding) *[]security.Finding {
	if findings == nil {
		return nil
	}
	return &findings
}

func nonNilWorkflowOutcomes(outcomes []WorkflowOutcome) []WorkflowOutcome {
	if outcomes == nil {
		return []WorkflowOutcome{}
	}
	return outcomes
}
