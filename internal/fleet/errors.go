package fleet

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ErrRepoNotTracked returns a consistent error message for a repo that is not
// present in the loaded config. The message names the file(s) actually loaded
// so operators know whether to edit fleet.json, fleet.local.json, or both.
func ErrRepoNotTracked(repo, loadedFrom string) error {
	baseSource, localSource := parseLoadedSources(loadedFrom)

	switch {
	case baseSource != "" && localSource == "":
		return fmt.Errorf("repo %q not tracked in %s", repo, baseSource)
	case localSource != "" && baseSource == "":
		return fmt.Errorf("repo %q not tracked in %s", repo, localSource)
	default:
		if baseSource == "" {
			baseSource = ConfigFile
		}
		if localSource == "" {
			localSource = LocalConfigFile
		}
		return fmt.Errorf("repo %q not tracked in %s or %s", repo, baseSource, localSource)
	}
}

// parseLoadedSources inspects the LoadedFrom string (e.g. "/tmp/fleet.json",
// "/tmp/fleet.local.json", or "/tmp/fleet.json + /tmp/fleet.local.json") and
// returns the original source path for each recognized config file.
func parseLoadedSources(loadedFrom string) (string, string) {
	var base, local string
	parts := strings.Split(loadedFrom, " + ")
	for _, p := range parts {
		source := strings.TrimSpace(p)
		switch filepath.Base(source) {
		case ConfigFile:
			base = source
		case LocalConfigFile:
			local = source
		}
	}
	return base, local
}
