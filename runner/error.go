package runner

import "errors"

// ErrFatal is a sentinel error that marks a runner-level error as fatal.
// When a fatal error is encountered during agent execution, the runner stops
// the invocation immediately instead of continuing to yield events.
var ErrFatal = errors.New("fatal runner error")

// FatalError wraps an underlying error as fatal.
type FatalError struct {
	Cause error
}

func (e *FatalError) Error() string {
	if e.Cause == nil {
		return ErrFatal.Error()
	}
	return e.Cause.Error()
}

func (e *FatalError) Unwrap() error {
	return e.Cause
}

// Is reports whether the target is ErrFatal, so errors.Is works.
func (e *FatalError) Is(target error) bool {
	return target == ErrFatal
}
