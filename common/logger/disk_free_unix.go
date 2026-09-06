//go:build unix

package logger

import (
	"syscall"

	"github.com/Laisky/errors/v2"
)

// freeDiskBytes reports the free space available to an unprivileged process on
// the filesystem holding path.
//
// Parameters:
//   - path: any existing path on the filesystem to inspect.
//
// Return values:
//   - uint64: available bytes.
//   - error: wrapped statfs failure.
func freeDiskBytes(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, errors.Wrapf(err, "statfs %s", path)
	}
	// Bavail is what a non-root process may actually use; Bfree includes the
	// reserved blocks only root can touch.
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil
}

// diskFreeSupported reports whether free-space inspection works on this platform.
//
// Parameters: none.
//
// Return values:
//   - bool: always true on unix.
func diskFreeSupported() bool { return true }
