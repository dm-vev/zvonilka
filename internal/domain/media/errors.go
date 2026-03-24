package media

import "errors"

var (
	// ErrNotFound indicates that a media asset could not be located.
	ErrNotFound = errors.New("not found")
	// ErrConflict indicates that the requested media operation would violate state.
	ErrConflict = errors.New("conflict")
	// ErrForbidden indicates that the caller is not allowed to access the asset.
	ErrForbidden = errors.New("forbidden")
	// ErrInvalidInput indicates that the request payload is malformed.
	ErrInvalidInput = errors.New("invalid input")
)
