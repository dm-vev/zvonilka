package conversation

import "errors"

var (
	// ErrNotFound indicates that a requested conversation entity does not exist.
	ErrNotFound = errors.New("not found")
	// ErrConflict indicates that a requested write would violate a domain invariant.
	ErrConflict = errors.New("conflict")
	// ErrInvalidInput indicates that request validation failed.
	ErrInvalidInput = errors.New("invalid input")
	// ErrForbidden indicates that the caller cannot perform the requested action.
	ErrForbidden = errors.New("forbidden")
)
