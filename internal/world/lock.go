package world

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// lockFileName is the per-world advisory lock file. It lives inside the
// world dir so the lock scope is exactly one world: two different worlds
// can run concurrently, two runs on the same world cannot.
const lockFileName = ".worldsmith.lock"

// Lock is a held exclusive lock on one world. Acquire it with Lock and
// release it with Unlock (typically `defer lock.Unlock()`).
type Lock struct {
	f *os.File
}

// ErrLocked is returned when another worldsmith process already holds the
// world's lock. Callers can errors.Is against it to special-case contention.
var ErrLocked = errors.New("world is locked by another worldsmith process")

// Acquire takes an exclusive, non-blocking advisory lock on the world. It
// returns ErrLocked (wrapped, with the world path) if another process
// already holds it, so a second mutating run fails fast instead of
// corrupting shared state (canon.md, timeline.json, the run dirs).
//
// The lock is flock-based: it is released automatically if the process
// dies (the kernel drops the lock when the fd closes), so a killed run
// never leaves a stale lock behind.
func Acquire(l Layout) (*Lock, error) {
	if err := os.MkdirAll(l.Root, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(l.Root, lockFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%w: %q (release it or wait): %w", ErrLocked, l.Root, err)
		}
		return nil, fmt.Errorf("lock world %q: %w", l.Root, err)
	}
	return &Lock{f: f}, nil
}

// Unlock releases the world lock. Safe to call once; subsequent calls
// are no-ops. Closing the fd drops the flock.
func (lk *Lock) Unlock() error {
	if lk == nil || lk.f == nil {
		return nil
	}
	f := lk.f
	lk.f = nil
	// Closing the descriptor releases the flock; an explicit LOCK_UN
	// first makes the release immediate even if other fds were dup'd.
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return f.Close()
}
