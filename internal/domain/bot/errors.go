package bot

import "errors"

var (
	// ErrInvalidInput indicates that the caller supplied malformed bot input.
	ErrInvalidInput = errors.New("invalid input")
	// ErrNotFound indicates that no bot state exists for the requested key.
	ErrNotFound = errors.New("not found")
	// ErrConflict indicates that the requested operation conflicts with current bot state.
	ErrConflict = errors.New("conflict")
	// ErrForbidden indicates that the bot cannot access the requested resource.
	ErrForbidden = errors.New("forbidden")
)
