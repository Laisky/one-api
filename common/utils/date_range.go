package utils

import (
	"fmt"
	"time"
)

// NormalizeDateRange parses inclusive date strings (YYYY-MM-DD) and returns
// a half-open [start, endExclusive) Unix second range in UTC.
// It validates that from <= to and enforces maxDays (inclusive day count) if >0.
// Returns start, endExclusive, error.
func NormalizeDateRange(fromStr, toStr string, maxDays int) (int64, int64, error) {
	const layout = "2006-01-02"
	fromDate, err := time.Parse(layout, fromStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid from_date format, expected YYYY-MM-DD: %w", err)
	}
	toDate, err := time.Parse(layout, toStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid to_date format, expected YYYY-MM-DD: %w", err)
	}

	// Truncate to UTC midnight boundaries
	fromDay := time.Date(fromDate.Year(), fromDate.Month(), fromDate.Day(), 0, 0, 0, 0, time.UTC)
	toDay := time.Date(toDate.Year(), toDate.Month(), toDate.Day(), 0, 0, 0, 0, time.UTC)

	if toDay.Before(fromDay) {
		return 0, 0, fmt.Errorf("from_date must be before to_date")
	}

	inclusiveDays := int(toDay.Sub(fromDay).Hours()/24) + 1
	if maxDays > 0 && inclusiveDays > maxDays {
		return 0, 0, fmt.Errorf("date range too large. Maximum allowed: %d days", maxDays)
	}

	start := fromDay.Unix()
	endExclusive := toDay.Add(24 * time.Hour).Unix()
	return start, endExclusive, nil
}
