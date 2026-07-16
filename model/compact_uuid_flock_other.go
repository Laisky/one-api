//go:build !unix

package model

// This file provides the non-Unix fallback for SQLite compact ownership (AUTO-008).
//
// Platforms without flock fall back to the in-process claim already held by the caller. That is
// a real reduction in scope — two processes on such a platform sharing one SQLite file could
// both run a cycle — and it is safe precisely because election is not a correctness
// dependency: every side effect is verify-before/create/verify-after, conditional, or
// idempotent. The cost of losing the lock is duplicated work, not divergence.

import (
	"os"
)

// tryLockFile reports a successful claim without an OS-level lock.
// Parameters:
//   - file: open sidecar lock file, unused on this platform.
//
// Return values:
//   - bool: always true.
//   - error: always nil.
func tryLockFile(file *os.File) (bool, error) {
	return true, nil
}

// unlockFile is a no-op on platforms without advisory file locks.
// Parameters:
//   - file: open sidecar lock file, unused on this platform.
//
// Return values:
//   - error: always nil.
func unlockFile(file *os.File) error {
	return nil
}
