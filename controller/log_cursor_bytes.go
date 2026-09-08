package controller

// Response byte cap for cursor pages (W2.4, "cap page size and encoded response
// bytes ... Oversized individual records receive an explicit error, not silent
// field truncation").
//
// A page is bounded by row count, but rows are not uniform: log content and
// metadata vary widely, so a page of the permitted row count can still be
// enormous. Rows are therefore encoded one at a time and accumulated against a
// byte budget. When the next row would exceed it the page stops early and says
// so, and the caller's next cursor resumes at the first omitted row, so
// traversal still makes forward progress.
//
// No field is ever truncated. A single record larger than the whole budget is
// returned alone and flagged, because silently shortening a value would be
// indistinguishable from that value being short.

import (
	"encoding/json"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/dto"
)

// encodeCursorItems renders page rows within the configured byte budget.
//
// The returned consumed count is the number of source rows the returned items
// account for. The caller derives the next cursor from row consumed-1, so the
// two must stay aligned: a row is never skipped over, only stopped at. Silently
// dropping one would put a hole in a traversal that promises to visit every row
// exactly once.
//
// Parameters:
//   - lg: the request logger.
//   - rows: the page rows, newest first.
//
// Return values:
//   - []json.RawMessage: the rows that fit, in order.
//   - int: how many source rows those items account for.
//   - bool: whether the page was cut short before its last row.
//   - bool: whether the first row alone exceeded the whole budget.
func encodeCursorItems(lg glog.Logger, rows []dto.LogResponse) ([]json.RawMessage, int, bool, bool) {
	items := make([]json.RawMessage, 0, len(rows))
	budget := config.LogCursorMaxResponseBytes

	var used int
	for i := range rows {
		encoded, err := json.Marshal(rows[i])
		if err != nil {
			// Stop here rather than skip: the next cursor resumes at this row,
			// so the traversal stays complete and the failure is visible.
			lg.Error("a log row could not be encoded; truncating the page at it",
				zap.Int("index", i), zap.Error(err))
			return items, i, true, len(items) == 0
		}

		if budget > 0 && used+len(encoded) > budget {
			if len(items) == 0 {
				// Returning it alone keeps the traversal moving; returning
				// nothing would stall it forever on this row.
				lg.Warn("a single log row exceeds the cursor response budget",
					zap.Int("row_bytes", len(encoded)),
					zap.Int("budget_bytes", budget))
				return []json.RawMessage{encoded}, 1, true, true
			}
			return items, i, true, false
		}

		used += len(encoded)
		items = append(items, encoded)
	}

	return items, len(rows), false, false
}
