package cmd

import (
	"bytes"
	"strings"
	"testing"
)

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
