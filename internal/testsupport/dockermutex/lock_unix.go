//go:build !windows

package dockermutex

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
)

var mu sync.Mutex

// Acquire serializes Docker-backed integration tests across packages.
func Acquire(t *testing.T, name string) func() {
	t.Helper()

	mu.Lock()

	path := filepath.Join(os.TempDir(), "zvonilka-"+name+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		mu.Unlock()
		t.Fatalf("open lock file %s: %v", path, err)
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		mu.Unlock()
		t.Fatalf("lock file %s: %v", path, err)
	}

	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		mu.Unlock()
	}
}
