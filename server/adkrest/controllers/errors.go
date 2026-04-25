package controllers

type statusError struct {
	Err  error
	Code int
}

func newStatusError(err error, code int) statusError {
	return statusError{Err: err, Code: code}
}

// Error returns an associated error
func (se statusError) Error() string {
	return se.Err.Error()
}

// Status returns an associated status code
func (se statusError) Status() int {
	return se.Code
}
