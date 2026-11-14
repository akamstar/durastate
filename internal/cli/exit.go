package cli

import "errors"

// Exit codes, part of the CLI contract (scripts and CI rely on them).
const (
	ExitOK            = 0
	ExitError         = 1
	ExitInvalidConfig = 2
	ExitUnreachable   = 3 // daemon not reachable
)

// exitError carries an exit code. A silent error was already reported.
type exitError struct {
	code   int
	err    error
	silent bool
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

// ExitCode returns the process exit code for an error returned by Execute.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var e *exitError
	if errors.As(err, &e) {
		return e.code
	}
	return ExitError
}

// Silent reports whether err was already printed by the command.
func Silent(err error) bool {
	var e *exitError
	return errors.As(err, &e) && e.silent
}
