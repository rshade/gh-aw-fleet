package cmd

// This file is a compile-time gate for the spec-003 envelope helpers that the
// `status` subcommand depends on. If any helper is removed or its signature
// drifts from contracts/json-envelope.md, this file fails to compile and the
// PR cannot land — making the verification a hard gate rather than a doc step.
//
// Referenced by: specs/004-status-drift-detection/contracts/json-envelope.md.

import (
	"io"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
)

// Each helper is referenced as a value of its expected function type. Any
// signature drift surfaces here as a compile error, before any runtime test.
var (
	_ func(*cobra.Command, string, string, bool, any, []fleet.Diagnostic, []fleet.Diagnostic) error = writeEnvelope
	_ func(*cobra.Command, string, string, bool, error) error                                       = preResultFailureEnvelope
	_ func(*cobra.Command) string                                                                   = outputMode
	_ func(string) error                                                                            = validateOutputMode
	_ func([]fleet.Diagnostic, error) []fleet.Diagnostic                                            = ensureFailureHint
)

// TestStatusEnvelopeHelperBindings runs as a no-op; its purpose is to surface
// compile errors via the var block above when an envelope helper drifts.
func TestStatusEnvelopeHelperBindings(t *testing.T) {
	t.Helper()
	_ = io.Discard
}
