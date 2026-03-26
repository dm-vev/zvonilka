package call

import "errors"

var (
	// ErrInvalidInput indicates malformed or incomplete input.
	ErrInvalidInput = errors.New("invalid input")
	// ErrNotFound indicates missing call state.
	ErrNotFound = errors.New("not found")
	// ErrConflict indicates a state conflict.
	ErrConflict = errors.New("conflict")
	// ErrForbidden indicates the caller is not allowed to perform the action.
	ErrForbidden = errors.New("forbidden")
)
