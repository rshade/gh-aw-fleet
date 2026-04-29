package cmd

import "errors"

type commandExitError struct {
	err    error
	code   int
	silent bool
}

func newCommandExitError(err error, code int, silent bool) error {
	if err == nil {
		return nil
	}
	return &commandExitError{err: err, code: code, silent: silent}
}

func (e *commandExitError) Error() string {
	return e.err.Error()
}

func (e *commandExitError) Unwrap() error {
	return e.err
}

func (e *commandExitError) ExitCode() int {
	return e.code
}

func (e *commandExitError) SuppressLog() bool {
	return e.silent
}

// ExitCodeForError returns the intended process exit code for command errors
// that carry one explicitly.
func ExitCodeForError(err error) (int, bool) {
	var exitErr *commandExitError
	if !errors.As(err, &exitErr) {
		return 0, false
	}
	return exitErr.ExitCode(), true
}

// SuppressErrorLog reports whether main should avoid logging err as a fatal
// internal failure. Expected gate failures such as status drift use this path.
func SuppressErrorLog(err error) bool {
	var exitErr *commandExitError
	return errors.As(err, &exitErr) && exitErr.SuppressLog()
}
