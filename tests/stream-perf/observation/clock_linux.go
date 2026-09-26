package observation

import (
	"github.com/Laisky/errors/v2"
	"golang.org/x/sys/unix"
)

// MonotonicNS returns Linux CLOCK_MONOTONIC for same-host, same-time-namespace correlation.
// Its epoch is unrelated to UTC; consumers must verify namespace and clock alignment.
func MonotonicNS() (int64, error) {
	var t unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &t); err != nil {
		return 0, errors.Wrap(err, "CLOCK_MONOTONIC")
	}
	return t.Nano(), nil
}
