package fleet

import "strings"

// Hint is a remediation suggestion keyed by a substring match against
// gh-aw CLI output.
type Hint struct {
	Pattern string
	Message string
}

// Ordered most-specific first; only the first match per input text is emitted.
//
//nolint:gochecknoglobals // immutable hint table; Go has no const slice of structs
var hints = []Hint{
	{
		Pattern: "Unknown property: mount-as-clis",
		Message: "Workflow uses `mount-as-clis`, an unreleased gh-aw feature. " +
			"`gh extension upgrade gh-aw` if your CLI is out of date; if already latest, " +
			"the upstream is ahead of the release — pin the source to a tagged release (e.g. `@v0.68.3`) " +
			"via `fleet sync --apply --force`.",
	},
	{
		Pattern: "Unknown property:",
		Message: "Workflow uses a property your installed `gh aw` CLI doesn't recognize. " +
			"Try `gh extension upgrade gh-aw`, or pin the workflow source to a tagged release.",
	},
	{
		Pattern: "HTTP 404",
		Message: "Source path not found. Check the spec — `github/gh-aw` workflows live under `.github/workflows/`; " +
			"`githubnext/agentics` workflows live under `workflows/`.",
	},
	{
		Pattern: "gpg failed to sign",
		Message: "gpg-agent couldn't prompt for a passphrase in this non-interactive context. " +
			"Unlock gpg-agent in your shell (`echo test | gpg -as > /dev/null`) and re-run.",
	},
}

// CollectHints scans any amount of output text for known error patterns and
// returns unique applicable remediation hints, ordered by first appearance.
// Most-specific hint wins per input string.
func CollectHints(texts ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range texts {
		for _, h := range hints {
			if strings.Contains(t, h.Pattern) {
				if !seen[h.Message] {
					out = append(out, h.Message)
					seen[h.Message] = true
				}
				break
			}
		}
	}
	return out
}
