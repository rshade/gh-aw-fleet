package cmd

import (
	zlog "github.com/rs/zerolog/log"
)

func emitHints(repo string, hints []string) {
	for _, h := range hints {
		ev := zlog.Warn()
		if repo != "" {
			ev = ev.Str("repo", repo)
		}
		ev.Str("hint", h).Msg("diagnostic hint")
	}
}
