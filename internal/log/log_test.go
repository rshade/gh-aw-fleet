package log

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	zlog "github.com/rs/zerolog/log"

	"github.com/rshade/gh-aw-fleet/internal/testutil"
)

func TestConfigureValidLevels(t *testing.T) {
	for _, lvl := range []string{"trace", "debug", "info", "warn", "error"} {
		if err := Configure(lvl, "console"); err != nil {
			t.Errorf("Configure(%q, console) = %v; want nil", lvl, err)
		}
	}
}

func TestConfigureValidFormats(t *testing.T) {
	for _, f := range []string{"console", "json"} {
		if err := Configure("info", f); err != nil {
			t.Errorf("Configure(info, %q) = %v; want nil", f, err)
		}
	}
}

func TestConfigureInvalidLevel(t *testing.T) {
	err := Configure("shouting", "console")
	if err == nil {
		t.Fatal("Configure(shouting, console) = nil; want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "--log-level") {
		t.Errorf("error message %q missing --log-level", msg)
	}
	if !strings.Contains(msg, "shouting") {
		t.Errorf("error message %q missing offending value", msg)
	}
}

func TestConfigureInvalidFormat(t *testing.T) {
	err := Configure("info", "yaml")
	if err == nil {
		t.Fatal("Configure(info, yaml) = nil; want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "--log-format") {
		t.Errorf("error message %q missing --log-format", msg)
	}
	if !strings.Contains(msg, "yaml") {
		t.Errorf("error message %q missing offending value", msg)
	}
}

func TestConfigureJSONEmitsValidLine(t *testing.T) {
	buf := testutil.CaptureStderr(t, func() {
		if err := Configure("info", "json"); err != nil {
			t.Fatal(err)
		}
		zlog.Info().Msg("t")
	})
	var obj map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf)), &obj); err != nil {
		t.Fatalf("stderr line is not valid JSON: %v; raw=%q", err, buf)
	}
	if obj["level"] != "info" {
		t.Errorf("level = %v; want info", obj["level"])
	}
	if obj["message"] != "t" {
		t.Errorf("message = %v; want t", obj["message"])
	}
}

func TestConfigureErrorLevelSilencesWarn(t *testing.T) {
	buf := testutil.CaptureStderr(t, func() {
		if err := Configure("error", "console"); err != nil {
			t.Fatal(err)
		}
		zlog.Warn().Msg("should be dropped")
	})
	if buf != "" {
		t.Errorf("got %q on stderr; want empty (warn silenced at error level)", buf)
	}
}

func TestConfigureErrorDualWrite(t *testing.T) {
	buf := testutil.CaptureStderr(t, func() {
		if err := Configure("info", "json"); err != nil {
			t.Fatal(err)
		}
		boom := errors.New("boom")
		zlog.Error().Err(boom).Msgf("deploy failed: %s", boom)
	})
	line := strings.TrimSpace(buf)
	if line == "" {
		t.Fatal("no output emitted")
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("not valid JSON: %v; raw=%q", err, line)
	}
	if obj["error"] != "boom" {
		t.Errorf("error field = %v; want boom", obj["error"])
	}
	msg, _ := obj["message"].(string)
	if !strings.Contains(msg, "boom") {
		t.Errorf("message %q missing error text", msg)
	}
}
