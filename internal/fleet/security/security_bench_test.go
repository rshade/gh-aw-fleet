package security

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkRun10Workflows replicates one fixture into a tmp dir 10 times
// and times Run end-to-end. Records the measured number for SC-003
// (target: < 2s on a 10-workflow profile). The benchmark itself does not
// assert a hard threshold — record the result in the PR description per
// quickstart Step 9.
func BenchmarkRun10Workflows(b *testing.B) {
	tmp := b.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".github", "workflows"), 0o755); err != nil {
		b.Fatalf("mkdir: %v", err)
	}
	src, err := os.ReadFile(filepath.Join(fixturesRoot, "clean-agentics-workflow.md"))
	if err != nil {
		b.Fatalf("read fixture: %v", err)
	}
	for i := range 10 {
		dst := filepath.Join(tmp, ".github", "workflows", fmt.Sprintf("clean-%d.md", i))
		if err := os.WriteFile(dst, src, 0o644); err != nil {
			b.Fatalf("write: %v", err)
		}
	}
	b.ResetTimer()
	for b.Loop() {
		_ = Run(context.Background(), tmp)
	}
}
