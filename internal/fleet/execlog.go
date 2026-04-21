package fleet

import (
	"os"
	"os/exec"
	"time"

	zlog "github.com/rs/zerolog/log"
)

// Tool and subcommand labels are passed by callers as string literals, never
// derived from cmd.Args — keeping argv (which can carry credentials) out of
// log events at the type level.

// runLogged runs cmd.Run() and emits a debug-level "subprocess exited" event.
func runLogged(cmd *exec.Cmd, toolLabel, subcommand string, extraFields map[string]string) error {
	start := time.Now()
	err := cmd.Run()
	logSubprocessSummary(toolLabel, subcommand, cmd.ProcessState, start, extraFields)
	return err
}

// runLoggedCombined wraps cmd.CombinedOutput() with the same summary event.
func runLoggedCombined(cmd *exec.Cmd, toolLabel, subcommand string, extraFields map[string]string) ([]byte, error) {
	start := time.Now()
	out, err := cmd.CombinedOutput()
	logSubprocessSummary(toolLabel, subcommand, cmd.ProcessState, start, extraFields)
	return out, err
}

// runLoggedOutput wraps cmd.Output() with the same summary event.
func runLoggedOutput(cmd *exec.Cmd, toolLabel, subcommand string, extraFields map[string]string) ([]byte, error) {
	start := time.Now()
	out, err := cmd.Output()
	logSubprocessSummary(toolLabel, subcommand, cmd.ProcessState, start, extraFields)
	return out, err
}

func logSubprocessSummary(
	toolLabel, subcommand string,
	ps *os.ProcessState,
	start time.Time,
	extraFields map[string]string,
) {
	ev := zlog.Debug().
		Str("tool", toolLabel).
		Str("subcommand", subcommand)
	if ps != nil {
		ev = ev.Int("exit_code", ps.ExitCode()).
			Dur("duration", time.Since(start))
	} else {
		ev = ev.Int("exit_code", -1)
	}
	for k, v := range extraFields {
		ev = ev.Str(k, v)
	}
	ev.Msg("subprocess exited")
}

func subcommandLabel(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}
