package tracing

import "time"

// unixMilliToTime converts a Unix-millisecond instant into a UTC time.Time.
//
// Parameters:
//   - millis: the instant in Unix milliseconds; 0 yields the current time so a
//     missing mark never produces a 1970 span boundary.
//
// Return values:
//   - time.Time: the corresponding UTC instant.
func unixMilliToTime(millis int64) time.Time {
	if millis <= 0 {
		return time.Now().UTC()
	}
	return time.UnixMilli(millis).UTC()
}
