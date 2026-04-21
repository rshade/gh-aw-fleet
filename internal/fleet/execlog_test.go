package fleet

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/rshade/gh-aw-fleet/internal/log"
	"github.com/rshade/gh-aw-fleet/internal/testutil"
)

func TestRunLoggedSuccess(t *testing.T) {
	buf := testutil.CaptureStderr(t, func() {
		if err := log.Configure("debug", "json"); err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(context.Background(), "true")
		if err := runLogged(cmd, "sh", "true", nil); err != nil {
			t.Fatalf("runLogged(true) = %v; want nil", err)
		}
	})
	obj := mustOneJSONEvent(t, buf)
	if obj["level"] != "debug" {
		t.Errorf("level = %v; want debug", obj["level"])
	}
	if obj["tool"] != "sh" {
		t.Errorf("tool = %v; want sh", obj["tool"])
	}
	if obj["subcommand"] != "true" {
		t.Errorf("subcommand = %v; want true", obj["subcommand"])
	}
	if code, _ := obj["exit_code"].(float64); code != 0 {
		t.Errorf("exit_code = %v; want 0", obj["exit_code"])
	}
	if dur, _ := obj["duration"].(float64); dur < 0 {
		t.Errorf("duration = %v; want >= 0", obj["duration"])
	}
}

func TestRunLoggedFailure(t *testing.T) {
	buf := testutil.CaptureStderr(t, func() {
		if err := log.Configure("debug", "json"); err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(context.Background(), "false")
		if err := runLogged(cmd, "sh", "false", nil); err == nil {
			t.Fatal("runLogged(false) = nil; want error")
		}
	})
	obj := mustOneJSONEvent(t, buf)
	if obj["level"] != "debug" {
		t.Errorf("level = %v; want debug", obj["level"])
	}
	code, _ := obj["exit_code"].(float64)
	if code != 1 {
		t.Errorf("exit_code = %v; want 1", obj["exit_code"])
	}
}

func mustOneJSONEvent(t *testing.T, buf string) map[string]any {
	t.Helper()
	line := strings.TrimSpace(buf)
	if line == "" {
		t.Fatal("no output emitted")
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("not valid JSON: %v; raw=%q", err, line)
	}
	return obj
}
