package fleet

import (
	"fmt"
	"testing"
)

func TestErrRepoNotTracked(t *testing.T) {
	tests := []struct {
		name       string
		loadedFrom string
		want       string
	}{
		{
			name:       "base only",
			loadedFrom: ConfigFile,
			want:       `repo "owner/repo" not tracked in fleet.json`,
		},
		{
			name:       "local only",
			loadedFrom: LocalConfigFile,
			want:       `repo "owner/repo" not tracked in fleet.local.json`,
		},
		{
			name:       "both loaded",
			loadedFrom: fmt.Sprintf("%s + %s", ConfigFile, LocalConfigFile),
			want:       `repo "owner/repo" not tracked in fleet.json or fleet.local.json`,
		},
		{
			name:       "base only with path",
			loadedFrom: "/some/dir/fleet.json",
			want:       `repo "owner/repo" not tracked in /some/dir/fleet.json`,
		},
		{
			name:       "local only with path",
			loadedFrom: "/some/dir/fleet.local.json",
			want:       `repo "owner/repo" not tracked in /some/dir/fleet.local.json`,
		},
		{
			name:       "both with paths",
			loadedFrom: "/some/dir/fleet.json + /some/dir/fleet.local.json",
			want:       `repo "owner/repo" not tracked in /some/dir/fleet.json or /some/dir/fleet.local.json`,
		},
		{
			name:       "empty loadedFrom defaults to both",
			loadedFrom: "",
			want:       `repo "owner/repo" not tracked in fleet.json or fleet.local.json`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ErrRepoNotTracked("owner/repo", tt.loadedFrom)
			if got := err.Error(); got != tt.want {
				t.Errorf("ErrRepoNotTracked() = %q, want %q", got, tt.want)
			}
		})
	}
}
