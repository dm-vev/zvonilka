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
	// ErrExpiredJoinRequest indicates a join request expired before review.
	ErrExpiredJoinRequest = errors.New("expired join request")
	// ErrInvalidCode indicates a login code does not match the challenge.
	ErrInvalidCode = errors.New("invalid code")
)

func isNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}
