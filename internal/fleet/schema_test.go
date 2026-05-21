package fleet

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func boolPtr(b bool) *bool { return &b }

func TestEffectiveCompileStrict(t *testing.T) {
	const repo = "rshade/test"

	cases := []struct {
		name string

		spec       *bool
		visibility string
		visErr     error

		wantEffective bool
		wantSource    string
		wantReason    string // substring match; empty means must be ""
		wantProbeSkip bool   // assert ghRepoVisibility was NOT invoked
	}{
		{
			name: "explicit_true_short_circuits_lookup",
			spec: boolPtr(true), visibility: "private", visErr: nil,
			wantEffective: true, wantSource: "explicit", wantReason: "",
			wantProbeSkip: true,
		},
		{
			name: "explicit_false_short_circuits_lookup",
			spec: boolPtr(false), visibility: "public", visErr: nil,
			wantEffective: false, wantSource: "explicit", wantReason: "",
			wantProbeSkip: true,
		},
		{
			name: "auto_public",
			spec: nil, visibility: "public", visErr: nil,
			wantEffective: true, wantSource: "auto-public", wantReason: "",
		},
		{
			name: "auto_private",
			spec: nil, visibility: "private", visErr: nil,
			wantEffective: false, wantSource: "auto-private", wantReason: "",
		},
		{
			name: "auto_private_internal_treated_as_non_public",
			spec: nil, visibility: "internal", visErr: nil,
			wantEffective: false, wantSource: "auto-private", wantReason: "",
		},
		{
			name: "auto_fallback_on_lookup_error",
			spec: nil, visibility: "", visErr: errors.New("HTTP 403 Forbidden"),
			wantEffective: true, wantSource: "auto-fallback", wantReason: "403",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orig := ghRepoVisibility
			t.Cleanup(func() { ghRepoVisibility = orig })

			var calls int
			ghRepoVisibility = func(_ context.Context, _ string) (string, error) {
				calls++
				return tc.visibility, tc.visErr
			}

			cfg := &Config{Repos: map[string]RepoSpec{
				repo: {CompileStrict: tc.spec},
			}}
			gotEff, gotSrc, gotReason := cfg.EffectiveCompileStrict(context.Background(), repo)

			if gotEff != tc.wantEffective {
				t.Errorf("effective = %v; want %v", gotEff, tc.wantEffective)
			}
			if gotSrc != tc.wantSource {
				t.Errorf("source = %q; want %q", gotSrc, tc.wantSource)
			}
			if tc.wantReason == "" {
				if gotReason != "" {
					t.Errorf("reason = %q; want \"\"", gotReason)
				}
			} else if !strings.Contains(gotReason, tc.wantReason) {
				t.Errorf("reason = %q; want substring %q", gotReason, tc.wantReason)
			}
			if tc.wantProbeSkip && calls != 0 {
				t.Errorf("ghRepoVisibility calls = %d; want 0 (FR-008: explicit override skips lookup)", calls)
			}
			if !tc.wantProbeSkip && calls != 1 {
				t.Errorf("ghRepoVisibility calls = %d; want 1", calls)
			}
		})
	}
}

func TestLoad_CompileStrictRoundtrip(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantPtr *bool
	}{
		{
			name:    "absent",
			body:    `{"version":1,"repos":{"a/b":{"profiles":["default"]}}}` + "\n",
			wantPtr: nil,
		},
		{
			name:    "explicit_true",
			body:    `{"version":1,"repos":{"a/b":{"profiles":["default"],"compile_strict":true}}}` + "\n",
			wantPtr: boolPtr(true),
		},
		{
			name:    "explicit_false",
			body:    `{"version":1,"repos":{"a/b":{"profiles":["default"],"compile_strict":false}}}` + "\n",
			wantPtr: boolPtr(false),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, LocalConfigFile)
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatalf("seed: %v", err)
			}

			cfg, err := LoadConfig(dir)
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			assertCompileStrictPtrEqual(t, cfg.Repos["a/b"].CompileStrict, tc.wantPtr, "after initial load")

			if err := SaveLocalConfig(dir, cfg); err != nil {
				t.Fatalf("SaveLocalConfig: %v", err)
			}

			cfg2, err := LoadConfig(dir)
			if err != nil {
				t.Fatalf("LoadConfig (post-save): %v", err)
			}
			assertCompileStrictPtrEqual(
				t, cfg2.Repos["a/b"].CompileStrict, tc.wantPtr,
				"after save+reload (SC-007 semantic round-trip)",
			)

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read back: %v", err)
			}
			gotJSON := string(data)
			switch tc.wantPtr {
			case nil:
				if strings.Contains(gotJSON, "compile_strict") {
					t.Errorf("absence not preserved: file contains compile_strict; got=%s", gotJSON)
				}
			default:
				wantField := "\"compile_strict\": true"
				if !*tc.wantPtr {
					wantField = "\"compile_strict\": false"
				}
				if !strings.Contains(gotJSON, wantField) {
					t.Errorf("written file missing %q; got=%s", wantField, gotJSON)
				}
			}

			// SC-007 byte-identity check: writeJSON canonicalizes via
			// json.MarshalIndent, so byte-identity from arbitrary minified
			// seed is impossible. The meaningful contract is that a second
			// save of the same in-memory Config produces bytes identical to
			// the first save — i.e., the round-trip is a stable fixed point.
			if err := SaveLocalConfig(dir, cfg2); err != nil {
				t.Fatalf("SaveLocalConfig (second): %v", err)
			}
			data2, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read back (second): %v", err)
			}
			if !bytes.Equal(data, data2) {
				t.Errorf("SC-007: second save not byte-identical to first save\nfirst:  %q\nsecond: %q", data, data2)
			}
		})
	}
}

func assertCompileStrictPtrEqual(t *testing.T, got, want *bool, label string) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s: CompileStrict = %v; want nil", label, *got)
	case want != nil && got == nil:
		t.Errorf("%s: CompileStrict = nil; want %v", label, *want)
	case want != nil && got != nil && *want != *got:
		t.Errorf("%s: CompileStrict = %v; want %v", label, *got, *want)
	}
}

func TestEffectiveCompileStrict_ReasonTruncated(t *testing.T) {
	orig := ghRepoVisibility
	t.Cleanup(func() { ghRepoVisibility = orig })

	long := strings.Repeat("x", effectiveCompileStrictReasonMax+50)
	ghRepoVisibility = func(_ context.Context, _ string) (string, error) {
		return "", errors.New(long)
	}

	cfg := &Config{Repos: map[string]RepoSpec{"rshade/test": {}}}
	_, _, gotReason := cfg.EffectiveCompileStrict(context.Background(), "rshade/test")
	if len(gotReason) > effectiveCompileStrictReasonMax {
		t.Errorf("len(reason) = %d; want <= %d", len(gotReason), effectiveCompileStrictReasonMax)
	}
}

// TestTruncateReason_MultibyteRuneSafety exercises the UTF-8 boundary
// back-off: a string that would have a multi-byte sequence split exactly
// at the byte limit must be truncated at the preceding rune boundary,
// not mid-character.
func TestTruncateReason_MultibyteRuneSafety(t *testing.T) {
	// 3-byte rune (€) split: position the rune so that its first byte is
	// at byte (limit-1) and the next two bytes are beyond limit.
	limit := 10
	prefix := strings.Repeat("a", limit-1) // 9 ASCII bytes
	in := prefix + "€" + "xyz"             // total 9 + 3 + 3 = 15 bytes

	got := truncateReason(in, limit)
	if !utf8.ValidString(got) {
		t.Errorf("truncateReason produced invalid UTF-8: % x", []byte(got))
	}
	if len(got) > limit {
		t.Errorf("len(got) = %d; want <= %d", len(got), limit)
	}
	// The € rune starts at byte index 9; truncating at 10 splits it.
	// Expected outcome: drop the partial rune, return the 9 ASCII chars.
	if got != prefix {
		t.Errorf("got = %q; want %q (rune at boundary must be dropped)", got, prefix)
	}
}
