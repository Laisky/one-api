package model

import "github.com/Laisky/one-api/common/config"

// applyRecordLineFormat selects the record-log line shape for benchmarks in the
// tree that supports selecting one.
//
// Parameters:
//   - format: "full" or "compact".
//
// Return values: none.
func applyRecordLineFormat(format string) {
	config.LogRecordLineFormat = format
}
