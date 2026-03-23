package identity

import "errors"

var (
	// ErrNotFound indicates a requested resource does not exist.
	ErrNotFound = errors.New("not found")
	// ErrConflict indicates a resource already exists or a state transition is invalid.
	ErrConflict = errors.New("conflict")
	// ErrInvalidInput indicates request validation failed.
	ErrInvalidInput = errors.New("invalid input")
	// ErrUnauthorized indicates authentication failed.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrForbidden indicates the caller is not allowed to perform the operation.
	ErrForbidden = errors.New("forbidden")
	// ErrExpiredChallenge indicates a login challenge has expired.
	ErrExpiredChallenge = errors.New("expired challenge")
	// ErrInvalidCode indicates a login code does not match the challenge.
	ErrInvalidCode = errors.New("invalid code")
)
