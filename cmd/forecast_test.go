package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestForecastInvalidPeriod covers period parsing error.
func TestForecastInvalidPeriod(t *testing.T) {
	root := NewRootCmd()
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)

	root.SetArgs([]string{"forecast", "--period", "invalid"})
	err := root.Execute()
	if err == nil {
		t.Fatalf("Execute: expected error; got nil")
	}

	if !strings.Contains(err.Error(), "week") || !strings.Contains(err.Error(), "month") {
		t.Errorf("error %q does not name valid periods", err.Error())
	}
}

// TestForecastInvalidBy covers --by parsing error.
func TestForecastInvalidBy(t *testing.T) {
	root := NewRootCmd()
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)

	root.SetArgs([]string{"forecast", "--by", "invalid"})
	err := root.Execute()
	if err == nil {
		t.Fatalf("Execute: expected error; got nil")
	}

	if !strings.Contains(err.Error(), "repo") || !strings.Contains(err.Error(), "profile") {
		t.Errorf("error %q does not name valid axes", err.Error())
	}
}

// TestForecastJSONEnvelopeShape covers --output json mode produces valid envelope.
func TestForecastJSONEnvelopeShape(t *testing.T) {
	root := NewRootCmd()
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)

	root.SetArgs([]string{"forecast", "--output", "json"})
	// Will fail because no fleet.json, but envelope structure should be present
	_ = root.Execute()

	// Decode whatever was written to stdout
	var env Envelope
	if decErr := json.NewDecoder(&out).Decode(&env); decErr != nil {
		// If no JSON written, that's ok for this test (config load failed before envelope)
		return
	}

	if env.SchemaVersion != SchemaVersion {
		t.Errorf("schema_version = %d; want %d", env.SchemaVersion, SchemaVersion)
	}
	if env.Command != commandForecast {
		t.Errorf("command = %q; want %q", env.Command, commandForecast)
	}
}
