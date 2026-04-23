package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"

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

// writeEnvelopeTo is the test-friendly entry point. Normalizes nil slices to
// non-nil empty (FR-009), walks the result struct via initSlices to do the
// same recursively, then emits compact JSON + trailing newline via
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
// don't honor --output json (template fetch, add, status). Returns nil when
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
// hint diagnostic, then returns err so the caller can propagate the non-zero
// exit code. Used by every subcommand's JSON path when an error blocks result
// construction (config parse, repo not in fleet, missing tool).
func preResultFailureEnvelope(
	cmd *cobra.Command,
	command, repo string,
	apply bool,
	err error,
) error {
	hints := ensureFailureHint(nil, err)
	if writeErr := writeEnvelope(cmd, command, repo, apply, nil, nil, hints); writeErr != nil {
		return writeErr
	}
	return err
}

// initSlices replaces every nil slice field in v with a non-nil empty slice,
// recursing into nested structs and pointer-to-struct fields.
//
// Why this exists: stdlib encoding/json marshals nil slices as JSON null but
// non-nil empty slices as []. FR-009 requires [] for downstream consumer
// iteration without nil-guards. Initializing the business-logic structs at
// construction time would scatter this concern; running the helper at the
// envelope-writer boundary keeps the JSON contract isolated.
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

// walkInitSlices is intentionally a switch on three relevant Kinds; other
// kinds (numeric, bool, string, map, etc.) need no normalization.
//
//nolint:gocognit,exhaustive // exhaustive: only the three kinds matter for FR-009; gocognit: shape mirrors the kind switch by design
func walkInitSlices(rv reflect.Value) {
	for i := range rv.NumField() {
		field := rv.Field(i)
		if !field.CanSet() {
			continue
		}
		switch field.Kind() {
		case reflect.Slice:
			// Skip []byte (incl. json.RawMessage) — these have custom MarshalJSON
			// that expects nil → null, valid JSON bytes → nested object, and empty
			// non-nil []byte → marshal error. Don't normalize them.
			if field.Type().Elem().Kind() == reflect.Uint8 {
				continue
			}
			if field.IsNil() {
				field.Set(reflect.MakeSlice(field.Type(), 0, 0))
			}
			// Recurse into struct elements so nested struct slice-fields normalize too.
			if field.Type().Elem().Kind() == reflect.Struct {
				for j := range field.Len() {
					walkInitSlices(field.Index(j))
				}
			}
		case reflect.Pointer:
			if !field.IsNil() && field.Elem().Kind() == reflect.Struct {
				walkInitSlices(field.Elem())
			}
		case reflect.Struct:
			walkInitSlices(field)
		}
	}
}
