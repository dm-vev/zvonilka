package search

import "errors"

var (
	// ErrInvalidInput indicates that the caller supplied malformed search input.
	ErrInvalidInput = errors.New("invalid input")
	// ErrNotFound indicates that no search row exists for the requested key.
	ErrNotFound = errors.New("not found")
	// ErrConflict indicates that the requested search change conflicts with existing state.
	ErrConflict = errors.New("conflict")
)
