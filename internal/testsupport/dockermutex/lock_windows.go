//go:build windows

package dockermutex

import "testing"

// Acquire is a no-op on Windows.
func Acquire(t *testing.T, name string) func() {
	t.Helper()

	return func() {}
}
