// Package testutil provides shared test helpers.
package testutil

import (
	"io"
	"os"
	"testing"
)

// CaptureStderr swaps os.Stderr with a pipe for the duration of fn, restores
// it, and returns everything written. Callers that reconfigure a global
// logger inside fn must re-Configure after the call to restore the sink.
//
// Cleanup is wrapped in defer so os.Stderr is restored even if fn calls
// t.Fatal/t.FailNow (which invokes runtime.Goexit) or panics.
func CaptureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w //nolint:reassign // test harness swaps stderr to capture logger output
	defer func() {
		_ = w.Close()
		os.Stderr = orig //nolint:reassign // restore stderr after capture
		_ = r.Close()
	}()

	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	fn()

	_ = w.Close()
	return <-done
}
