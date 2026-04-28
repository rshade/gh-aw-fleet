package fleet

import (
	"slices"
	"testing"
)

func TestParseGitStatusPorcelain(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "leading space on first line preserved",
			in:   " M .github/workflows/ci-doctor.lock.yml\n M .github/workflows/pr-fix.lock.yml\n",
			want: []string{
				".github/workflows/ci-doctor.lock.yml",
				".github/workflows/pr-fix.lock.yml",
			},
		},
		{
			name: "no trailing newline",
			in:   " M .github/a\n M .github/b",
			want: []string{".github/a", ".github/b"},
		},
		{
			name: "empty output",
			in:   "",
			want: nil,
		},
		{
			name: "only newline",
			in:   "\n",
			want: nil,
		},
		{
			name: "root-level file",
			in:   " M README.md\n",
			want: []string{"README.md"},
		},
		{
			name: "added file (A in index column)",
			in:   "A  .github/workflows/new.yml\n",
			want: []string{".github/workflows/new.yml"},
		},
		{
			name: "modified in both index and worktree",
			in:   "MM .github/workflows/both.yml\n",
			want: []string{".github/workflows/both.yml"},
		},
		{
			name: "short line skipped",
			in:   " M\n M .github/ok.yml\n",
			want: []string{".github/ok.yml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGitStatusPorcelain(tt.in)
			if !slices.Equal(got, tt.want) {
				t.Errorf("parseGitStatusPorcelain(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
