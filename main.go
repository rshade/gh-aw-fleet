// Command gh-aw-fleet is the declarative fleet manager for GitHub Agentic
// Workflows. It reconciles target repos toward the desired state declared in
// fleet.json, delegating to `gh aw`, `gh`, and `git` under the hood.
package main

import (
	"errors"
	"fmt"
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
	// Logger-config errors predate logger initialization, so they're printed
	// as plain text rather than routed through a logger that never came up.
	// Cobra's SilenceErrors is true on the root, so main is the single place
	// that surfaces runtime errors to the user.
	if errors.Is(err, logpkg.ErrInvalidLevel) || errors.Is(err, logpkg.ErrInvalidFormat) {
		fmt.Fprintln(os.Stderr, err)
	} else {
		zlog.Error().Err(err).Msgf("fatal: %s", err)
	}
	os.Exit(1)
}
