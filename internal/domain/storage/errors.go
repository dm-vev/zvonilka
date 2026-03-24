package storage

import "errors"

var (
	// ErrNotFound indicates that a provider or selection could not be resolved.
	ErrNotFound = errors.New("not found")
	// ErrConflict indicates that a provider name or logical binding already exists.
	ErrConflict = errors.New("conflict")
	// ErrForbidden indicates that the caller is not allowed to use the provider.
	ErrForbidden = errors.New("forbidden")
	// ErrInvalidInput indicates that the requested storage configuration is malformed.
	ErrInvalidInput = errors.New("invalid input")
)
