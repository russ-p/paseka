package main

import "errors"

// exitError carries an explicit process status through the command layer. A
// command that reports a verdict rather than a usage error — `skills load`
// refusing a procedure the run was not offered is exit 2 — returns one, and
// main maps it to os.Exit without every call site owning a global.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }

func (e *exitError) Unwrap() error { return e.err }

// newExitError wraps err with the process status the command's contract names.
func newExitError(code int, err error) error {
	return &exitError{code: code, err: err}
}

// exitStatus is the one mapping from a command error to a process status. A
// plain error is the generic failure (1); a typed exit keeps its own code so a
// caller (an agent retrying a load, a script) can tell the two apart.
func exitStatus(err error) int {
	var typed *exitError
	if errors.As(err, &typed) {
		return typed.code
	}
	return 1
}
