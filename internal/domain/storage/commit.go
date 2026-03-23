package storage

import "errors"

type commitError struct {
	err error
}

func (e *commitError) Error() string {
	return e.err.Error()
}

func (e *commitError) Unwrap() error {
	return e.err
}

// Commit marks an application error as a committed transaction outcome.
//
// The wrapped error is still returned to the caller, but transaction runners
// should commit the underlying state instead of rolling it back.
func Commit(err error) error {
	if err == nil {
		return nil
	}
	if IsCommit(err) {
		return err
	}

	return &commitError{err: err}
}

// IsCommit reports whether an error requests commit-on-error semantics.
func IsCommit(err error) bool {
	if err == nil {
		return false
	}

	var target *commitError
	return errors.As(err, &target)
}

// UnwrapCommit returns the application error wrapped by Commit.
func UnwrapCommit(err error) error {
	if err == nil {
		return nil
	}

	var target *commitError
	if !errors.As(err, &target) {
		return err
	}

	return target.err
}
