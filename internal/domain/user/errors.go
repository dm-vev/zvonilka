package user

import "errors"

var (
	// ErrNotFound indicates a requested profile-side resource does not exist.
	ErrNotFound = errors.New("not found")
	// ErrConflict indicates a uniqueness or state conflict.
	ErrConflict = errors.New("conflict")
	// ErrInvalidInput indicates request validation failed.
	ErrInvalidInput = errors.New("invalid input")
	// ErrForbidden indicates the caller is not allowed to perform the action.
	ErrForbidden = errors.New("forbidden")
)
