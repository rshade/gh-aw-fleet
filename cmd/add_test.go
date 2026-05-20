package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func swapAddVisibilitySeam(t *testing.T, fn func(context.Context, string) (string, error)) {
	t.Helper()
	orig := ghRepoVisibilityForAdd
	t.Cleanup(func() { ghRepoVisibilityForAdd = orig })
	ghRepoVisibilityForAdd = fn
}

func TestAdd_PublicRepo_PrintsAutoOnInfoLine(t *testing.T) {
	swapAddVisibilitySeam(t, func(_ context.Context, _ string) (string, error) {
		return "public", nil
	})
	var buf bytes.Buffer
	printAddCompileStrictInfo(context.Background(), &buf, "rshade/test")

	out := buf.String()
	for _, want := range []string{"auto-on", `"compile_strict": false`} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout = %q; want substring %q", out, want)
		}
	}
}

func TestAdd_PrivateRepo_PrintsAutoOffInfoLine(t *testing.T) {
	swapAddVisibilitySeam(t, func(_ context.Context, _ string) (string, error) {
		return "private", nil
	})
	var buf bytes.Buffer
	printAddCompileStrictInfo(context.Background(), &buf, "rshade/test")

	out := buf.String()
	for _, want := range []string{"auto-off", `"compile_strict": true`} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout = %q; want substring %q", out, want)
		}
	}
}

func TestAdd_InternalRepo_PrintsAutoOffInfoLine(t *testing.T) {
	swapAddVisibilitySeam(t, func(_ context.Context, _ string) (string, error) {
		return "internal", nil
	})
	var buf bytes.Buffer
	printAddCompileStrictInfo(context.Background(), &buf, "rshade/test")

	out := buf.String()
	if !strings.Contains(out, "auto-off") {
		t.Errorf("stdout = %q; want auto-off (internal treated as non-public)", out)
	}
}

func TestAdd_VisibilityLookupFails_SuppressesInfoLine(t *testing.T) {
	swapAddVisibilitySeam(t, func(_ context.Context, _ string) (string, error) {
		return "", errors.New("network error")
	})
	var buf bytes.Buffer
	printAddCompileStrictInfo(context.Background(), &buf, "rshade/test")

	out := buf.String()
	if out != "" {
		t.Errorf("stdout = %q; want empty (FR-010: suppressed on lookup error)", out)
	}
}
