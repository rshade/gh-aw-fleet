package security

import (
	"os"
	"path/filepath"
	"strings"
)

// walkEntry is one workflow file located under <cloneDir>/.github/workflows.
// Rel is in slash form (e.g. ".github/workflows/foo.md") so it surfaces
// portably on Finding.File regardless of the host filesystem separator;
// Full is the os-native absolute path for reading the file.
type walkEntry struct {
	Rel  string
	Full string
}

// walkWorkflows returns every non-directory entry in
// <cloneDir>/.github/workflows whose name has the given suffix
// (e.g. ".md", ".lock.yml"). Returns nil when the workflows directory does
// not exist; never returns an error — scanners surface the missing-dir
// case as "no findings" rather than as an error.
func walkWorkflows(cloneDir, suffix string) []walkEntry {
	workflowsDir := filepath.Join(cloneDir, ".github", "workflows")
	entries, err := os.ReadDir(workflowsDir)
	if err != nil {
		return nil
	}
	var out []walkEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), suffix) {
			continue
		}
		out = append(out, walkEntry{
			Rel:  ".github/workflows/" + e.Name(),
			Full: filepath.Join(workflowsDir, e.Name()),
		})
	}
	return out
}
