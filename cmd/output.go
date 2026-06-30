package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
)

// SchemaVersion is the JSON envelope wire-contract version. Bumped only on
// breaking changes to envelope or result struct field shapes (FR-005).
// Additive changes (new optional fields, new diagnostic codes) do not bump.
const SchemaVersion = 1

// outputJSON is the --output value that selects the JSON envelope serializer.
// outputText is the default. Centralized so subcommand mode-checks share one source.
const (
	outputJSON = "json"
	outputText = "text"
)

const (
	commandConsumption = "consumption"
	commandDeploy      = "deploy"
	commandList        = "list"
	commandOverview    = "overview"
	commandStatus      = "status"
	commandSync        = "sync"
	commandUpgrade     = "upgrade"
)

// strictFlagUsage is the shared --strict help text for deploy, sync, and upgrade.
const strictFlagUsage = "Fail when HIGH Layer 1 security findings are present (does not change gh aw compile --strict)"

// yesFlagUsage is the shared --yes help text for deploy, sync, and upgrade. It
// skips only the interactive findings confirmation — the stderr findings and
// the PR-body Security Findings section are still produced.
const yesFlagUsage = "Skip the interactive security-findings confirmation prompt (findings still print on stderr and in the PR body)"

const (
	diagnosticFieldDrift  = "drift"
	diagnosticFieldRepo   = "repo"
	diagnosticFieldSecret = "secret"
	diagnosticFieldURL    = "url"
)

// Envelope is the top-level JSON object emitted on stdout in --output json mode.
// Field order is the JSON-key emission order; consumers MUST NOT depend on it.
// All seven keys are always present (FR-004) — no omitempty.
type Envelope struct {
	SchemaVersion int                `json:"schema_version"`
	Command       string             `json:"command"`
	Repo          string             `json:"repo"`
	Apply         bool               `json:"apply"`
	Result        any                `json:"result"`
	Warnings      []fleet.Diagnostic `json:"warnings"`
	Hints         []fleet.Diagnostic `json:"hints"`
}

// writeEnvelope emits one envelope on the cobra command's stdout writer.
// Thin wrapper over writeEnvelopeTo for production callers.
func writeEnvelope(
	cmd *cobra.Command,
	commandName, repo string,
	apply bool,
	result any,
	warnings, hints []fleet.Diagnostic,
) error {
	return writeEnvelopeTo(cmd.OutOrStdout(), commandName, repo, apply, result, warnings, hints)
}

// writeEnvelopeTo is the test-friendly entry point. Normalizes required nil
// slices to non-nil empty (FR-009), walks the result struct via initSlices to
// do the same recursively, then emits compact JSON + trailing newline via
// json.NewEncoder.Encode (research.md R4: per-call flush enables NDJSON
// streaming for upgrade --all).
//
// For future parallelization of upgrade --all, callers MUST serialize Encode
// calls behind a mutex to prevent interleaved partial lines (contracts/ndjson.md).
func writeEnvelopeTo(
	w io.Writer,
	commandName, repo string,
	apply bool,
	result any,
	warnings, hints []fleet.Diagnostic,
) error {
	if result != nil {
		initSlices(result)
	}
	if warnings == nil {
		warnings = []fleet.Diagnostic{}
	}
	if hints == nil {
		hints = []fleet.Diagnostic{}
	}
	env := Envelope{
		SchemaVersion: SchemaVersion,
		Command:       commandName,
		Repo:          repo,
		Apply:         apply,
		Result:        result,
		Warnings:      warnings,
		Hints:         hints,
	}
	enc := json.NewEncoder(w)
	return enc.Encode(env)
}

// ensureFailureHint preserves a machine-readable failure reason on stdout when
// a command returns a partial result but the diagnostics layer finds no hint.
func ensureFailureHint(hints []fleet.Diagnostic, err error) []fleet.Diagnostic {
	if err == nil || len(hints) > 0 {
		return hints
	}
	return append(hints, fleet.HintFromError(err))
}

// foldCompileStrictError walks err for a *fleet.CompileStrictError and, when
// present, appends a typed fleet.Diagnostic to warnings (carrying the
// structured Fields the json-envelope.md warnings[] contract documents:
// detected_version, minimum_version, repo, clone_dir). Returns the
// possibly-modified warnings slice and a bool indicating whether the
// diagnostic was folded in. Callers use the bool to skip the generic
// ensureFailureHint append so a single root cause doesn't surface twice.
func foldCompileStrictError(
	warnings []fleet.Diagnostic, err error,
) ([]fleet.Diagnostic, bool) {
	var cse *fleet.CompileStrictError
	if !errors.As(err, &cse) || cse == nil {
		return warnings, false
	}
	return append(warnings, fleet.Diagnostic{
		Code:    cse.Code,
		Message: cse.Message,
		Fields:  cse.Fields,
	}), true
}

// securityOptsFor builds the invocation's SecurityOpts. It forces Yes in JSON
// mode: a prompt written to stdout would corrupt the JSON envelope and a
// machine cannot answer it, so --output json is treated as non-interactive
// (FR-018). Centralizing this keeps the deploy/sync/upgrade RunE closures
// simple and the JSON-suppression rule in one place.
func securityOptsFor(strict, yes, jsonMode bool) fleet.SecurityOpts {
	return fleet.SecurityOpts{Strict: strict, Yes: yes || jsonMode}
}

// mapOperatorDecline converts a fleet *OperatorDeclinedError into a clean,
// silent, non-zero exit: it prints the actionable abort message to stderr and
// returns a commandExitError so main.go skips the fatal-crash logger and the
// decline is never routed through the diagnostic hint engine. Any other error
// (including nil) passes through unchanged. Called on the text-mode return
// paths of deploy/sync/upgrade; the JSON paths suppress the prompt entirely
// (FR-018), so a decline cannot arise there.
func mapOperatorDecline(cmd *cobra.Command, err error) error {
	if err == nil || !fleet.IsOperatorDeclinedError(err) {
		return err
	}
	fmt.Fprintln(cmd.ErrOrStderr(), err.Error())
	return newCommandExitError(err, 1, true)
}

// outputMode reads the resolved --output value. PersistentPreRunE has already
// validated, so this is a defensive read; empty resolves to outputText.
func outputMode(cmd *cobra.Command) string {
	v, _ := cmd.Flags().GetString("output")
	if v == "" {
		return outputText
	}
	return v
}

// validateOutputMode enforces the closed-set policy from contracts/cli-flags.md.
// Called from root's PersistentPreRunE; rejects yaml/JSON/empty/etc with an
// error message naming the accepted values.
func validateOutputMode(mode string) error {
	switch mode {
	case outputText, outputJSON:
		return nil
	default:
		return fmt.Errorf("unsupported output mode %q: expected one of: text, json", mode)
	}
}

// rejectJSONMode is invoked at the top of subcommand RunE for commands that
// don't honor --output json (template fetch, add). Returns nil when
// the active mode is text; returns an explicit error when json is requested
// (FR-013 — silent fallback would surprise jq consumers).
func rejectJSONMode(cmd *cobra.Command, subcommand string) error {
	if outputMode(cmd) == outputJSON {
		return fmt.Errorf(
			"command %q does not support --output json; use --output text or omit the flag",
			subcommand,
		)
	}
	return nil
}

// preResultFailureEnvelope writes a result:null envelope wrapping err as a
// diagnostic hint, then returns err so the caller can propagate the non-zero
// exit code. Used by every subcommand's JSON path when an error blocks result
// construction (config parse, repo not in fleet, missing tool).
func preResultFailureEnvelope(
	cmd *cobra.Command,
	command, repo string,
	apply bool,
	err error,
) error {
	hints := failureHintDiagnostics(err)
	if writeErr := writeEnvelope(cmd, command, repo, apply, nil, nil, hints); writeErr != nil {
		return writeErr
	}
	return err
}

func failureHintDiagnostics(err error) []fleet.Diagnostic {
	if err == nil {
		return nil
	}
	if diag, ok := fleet.DiagnosticFromError(err); ok {
		return []fleet.Diagnostic{diag}
	}
	hints := fleet.CollectHintDiagnostics(err.Error())
	if len(hints) > 0 {
		return hints
	}
	return ensureFailureHint(nil, err)
}

// initSlices replaces every required nil slice field in v with a non-nil
// empty slice, recursing into nested structs and pointer-to-struct fields.
//
// Why this exists: stdlib encoding/json marshals nil slices as JSON null but
// non-nil empty slices as []. FR-009 requires [] for downstream consumer
// iteration without nil-guards. Initializing the business-logic structs at
// construction time would scatter this concern; running the helper at the
// envelope-writer boundary keeps the JSON contract isolated. Optional slices
// such as security_findings are skipped because nil means "scanner did not
// run" while a non-nil empty slice means "scanner ran clean."
//
// Assumes v is acyclic — it's called only on the known-finite result types
// (ListResult, DeployResult, SyncResult, UpgradeResult), which have no
// back-edges. No cycle guard is installed.
func initSlices(v any) {
	if v == nil {
		return
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return
	}
	walkInitSlices(rv)
}

// walkInitSlices normalizes slices nested inside structs and pointers.
func walkInitSlices(rv reflect.Value) {
	for i := range rv.NumField() {
		field := rv.Field(i)
		if !field.CanSet() {
			continue
		}
		if field.Kind() == reflect.Slice {
			initSliceField(rv.Type().Field(i), field)
			continue
		}
		if field.Kind() == reflect.Pointer {
			if !field.IsNil() && field.Elem().Kind() == reflect.Struct {
				walkInitSlices(field.Elem())
			}
			continue
		}
		if field.Kind() == reflect.Struct {
			walkInitSlices(field)
		}
	}
}

func initSliceField(structField reflect.StructField, field reflect.Value) {
	if isOptionalSliceField(structField) {
		return
	}
	// Skip []byte (incl. json.RawMessage) — these have custom MarshalJSON
	// that expects nil → null, valid JSON bytes → nested object, and empty
	// non-nil []byte → marshal error. Don't normalize them.
	if field.Type().Elem().Kind() == reflect.Uint8 {
		return
	}
	if field.IsNil() {
		field.Set(reflect.MakeSlice(field.Type(), 0, 0))
	}
	if field.Type().Elem().Kind() != reflect.Struct {
		return
	}
	for j := range field.Len() {
		walkInitSlices(field.Index(j))
	}
}

func isOptionalSliceField(field reflect.StructField) bool {
	jsonKey, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	return jsonKey == "security_findings"
}
