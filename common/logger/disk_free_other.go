//go:build !unix

package logger

// freeDiskBytes is unavailable on this platform, so the free-disk guard is
// inert rather than wrong.
//
// Parameters:
//   - path: ignored.
//
// Return values:
//   - uint64: always 0.
//   - error: always nil; callers use diskFreeSupported to decide whether to ask.
func freeDiskBytes(string) (uint64, error) { return 0, nil }

// diskFreeSupported reports whether free-space inspection works on this platform.
//
// Parameters: none.
//
// Return values:
//   - bool: always false off unix.
func diskFreeSupported() bool { return false }
