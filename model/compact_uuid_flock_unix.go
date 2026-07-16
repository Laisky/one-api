//go:build unix

package model

// This file provides the Unix advisory file lock backing SQLite compact ownership (AUTO-008).
//
// The lock is non-blocking by design: an ownership attempt is capped at five seconds and a
// contended lock simply means another instance owns the cycle, which is an ordinary outcome
// rather than a failure. The kernel releases the lock when the descriptor closes or the
// process exits, so a killed owner cannot strand the migration.

import (
	"os"
	"syscall"

	"github.com/Laisky/errors/v2"
)

// tryLockFile attempts to take an exclusive advisory lock without blocking.
// Parameters:
//   - file: open sidecar lock file.
//
// Return values:
//   - bool: true when the lock was taken.
//   - error: wrapped error when the attempt failed for a reason other than contention.
func tryLockFile(file *os.File) (bool, error) {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if err == syscall.EWOULDBLOCK {
		// Another instance holds the claim. Expected, not an error.
		return false, nil
	}
	return false, errors.Wrap(err, "acquire compact ownership file lock")
}

// unlockFile releases an advisory lock held on the sidecar file.
// Parameters:
//   - file: open sidecar lock file.
//
// Return values:
//   - error: wrapped error when the release failed.
func unlockFile(file *os.File) error {
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
		return errors.Wrap(err, "release compact ownership file lock")
	}
	return nil
}
