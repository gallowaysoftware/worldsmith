package world

import (
	"errors"
	"testing"
)

func TestLockExclusive(t *testing.T) {
	l := Layout{Root: t.TempDir()}

	first, err := Acquire(l)
	if err != nil {
		t.Fatalf("first Lock: %v", err)
	}

	// A second lock on the same world must fail fast (non-blocking) with
	// ErrLocked rather than hang or succeed.
	if _, err := Acquire(l); !errors.Is(err, ErrLocked) {
		t.Fatalf("second Lock = %v, want ErrLocked", err)
	}

	if err := first.Unlock(); err != nil {
		t.Fatalf("Unlock: %v", err)
	}

	// After release the lock is acquirable again.
	again, err := Acquire(l)
	if err != nil {
		t.Fatalf("re-Lock after Unlock: %v", err)
	}
	if err := again.Unlock(); err != nil {
		t.Fatalf("second Unlock: %v", err)
	}
}

func TestLockPerWorld(t *testing.T) {
	a := Layout{Root: t.TempDir()}
	b := Layout{Root: t.TempDir()}

	la, err := Acquire(a)
	if err != nil {
		t.Fatalf("lock a: %v", err)
	}
	defer func() { _ = la.Unlock() }()

	// A different world is independently lockable while a is held.
	lb, err := Acquire(b)
	if err != nil {
		t.Fatalf("lock b while a held: %v", err)
	}
	if err := lb.Unlock(); err != nil {
		t.Fatalf("unlock b: %v", err)
	}
}
