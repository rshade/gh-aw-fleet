package main

import (
	"errors"
	"os"

	zlog "github.com/rs/zerolog/log"

	"github.com/rshade/gh-aw-fleet/cmd"
	logpkg "github.com/rshade/gh-aw-fleet/internal/log"
)

func main() {
	err := cmd.Execute()
	if err == nil {
		return
	}
	// Logger-config errors predate logger initialization; cobra surfaces
	// them as plain-text stderr rather than routing them through a logger
	// that never came up.
	if !errors.Is(err, logpkg.ErrInvalidLevel) && !errors.Is(err, logpkg.ErrInvalidFormat) {
		zlog.Error().Err(err).Msgf("fatal: %s", err)
	}
	os.Exit(1)
}
