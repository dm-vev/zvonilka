package callhook

import "errors"

var (
	// ErrInvalidInput indicates malformed hook payload or job data.
	ErrInvalidInput = errors.New("callhook: invalid input")
	// ErrNotFound indicates missing persisted hook state.
	ErrNotFound = errors.New("callhook: not found")
	// ErrConflict indicates stale or conflicting hook/job state.
	ErrConflict = errors.New("callhook: conflict")
)
