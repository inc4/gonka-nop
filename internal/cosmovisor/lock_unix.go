//go:build !windows

package cosmovisor

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// ErrLockHeld is returned when another process already holds the install lock.
// Callers should treat this as a non-fatal "try again later" condition rather
// than a hard error (a concurrent installer is doing exactly what this one
// wants to do).
var ErrLockHeld = errors.New("cosmovisor install lock is held by another process")

// AcquireLock takes an exclusive non-blocking flock on path. The returned
// release function closes the file (which releases the lock per POSIX).
// File mode 0600 — the lock file is created in a host directory typically
// owned by root, so writability is gated on either sudo or pre-existing
// permissions. The lock file is NOT removed on release; the lock is on
// the file descriptor, not the path.
func AcquireLock(path string) (release func() error, err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600) // #nosec G304 - path constructed by installer from validated DeployDir
	if err != nil {
		return nil, fmt.Errorf("open lock file %s: %w", path, err)
	}

	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, ErrLockHeld
		}
		return nil, fmt.Errorf("flock %s: %w", path, err)
	}

	return func() error {
		// Close releases the file lock; explicit Flock(LOCK_UN) is redundant.
		return f.Close()
	}, nil
}
