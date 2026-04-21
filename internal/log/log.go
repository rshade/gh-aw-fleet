// Package log installs the process-wide zerolog logger from CLI flag values.
package log

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
)

// ErrInvalidLevel and ErrInvalidFormat wrap invalid --log-level / --log-format
// inputs so callers can distinguish configuration errors from post-init errors
// with errors.Is (see main.go's exit path, which must not re-route these
// through the logger they failed to initialize).
var (
	ErrInvalidLevel  = errors.New("invalid --log-level")
	ErrInvalidFormat = errors.New("invalid --log-format")
)

// Configure parses level and format strings and installs a global logger
// writing to os.Stderr.
func Configure(level, format string) error {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("%w %q: %w", ErrInvalidLevel, level, err)
	}

	var w io.Writer
	switch format {
	case "json":
		w = os.Stderr
	case "console":
		w = zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	default:
		return fmt.Errorf("%w %q: must be %q or %q", ErrInvalidFormat, format, "console", "json")
	}

	//nolint:reassign // zerolog's documented global-config pattern
	zerolog.TimeFieldFormat = time.RFC3339
	zerolog.SetGlobalLevel(lvl)
	//nolint:reassign // zerolog's documented global-logger-replacement pattern
	zlog.Logger = zerolog.New(w).With().Timestamp().Logger()
	return nil
}
