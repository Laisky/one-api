package model

// Keyset log pages for the additive cursor list (proposal
// docs/proposals/20260905_observability-data-tiering.md, W2.4).
//
// The statement shape is deliberate and measured. The obvious keyset predicate,
//
//	(created_at < ?) OR (created_at = ? AND id < ?)
//
// forms no usable index condition on PostgreSQL: a plan capture over a
// three-million-row table discarded 1.8 million rows in a filter for a single
// page, which is worse than the deep-offset scan this work exists to remove.
// Row-value constructors, `(created_at, id) < (?, ?)`, read well and are
// supported by every engine here, but MySQL 8.4 does not range-optimize `<` on
// a row constructor and examined 150k rows for one page.
//
// What does work on all three is a two-arm UNION ALL, where each arm is a
// simple, independently index-friendly range predicate:
//
//	arm EQ: created_at = anchor.created_at AND id < anchor.id
//	arm LT: created_at < anchor.created_at
//
// Each arm carries the full shared filter, orders and limits itself, and the
// outer query merges and re-limits. Both arms are bounded by page_size+1, so a
// page costs at most 2*(page_size+1) index entries regardless of how deep the
// anchor is or how many rows share one second.

import (
	"context"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/dto"
)

// LogCursorAnchor is the last row of the previous page.
//
// It is the only value a cursor contributes to the query: two integers that
// select a window of rows the caller is already authorized to read.
type LogCursorAnchor struct {
	// CreatedAt is the anchor row's created_at, in Unix seconds.
	CreatedAt int64
	// ID is the anchor row's primary key.
	ID int64
}

// LogCursorPage is one page of a keyset traversal.
type LogCursorPage struct {
	// Logs are the rows of this page, newest first.
	Logs []*Log
	// HasMore reports whether at least one further row exists.
	HasMore bool
	// Last is the anchor a caller passes back to fetch the following page. It
	// is meaningful only when Logs is non-empty.
	Last LogCursorAnchor
}

// logCursorColumns is the explicit projection every arm selects.
//
// It is spelled out rather than using `*` so a future column addition cannot
// silently widen the union arms or change their row shape.
const logCursorColumns = "id, uuid, user_id, user_uuid, created_at, type, content, username, " +
	"token_name, token_uuid, model_name, origin_model_name, quota, prompt_tokens, completion_tokens, " +
	"channel_id, channel_uuid, request_id, trace_id, updated_at, elapsed_time, is_stream, " +
	"system_prompt_reset, cached_prompt_tokens, metadata"

// maxLogCursorPageSize bounds a single page independently of caller input.
const maxLogCursorPageSize = 200

// buildLogCursorStatement renders the keyset page statement and its arguments.
//
// The LIMIT is interpolated as a validated integer rather than bound as a
// placeholder: MySQL accepts a placeholder there only through the prepared
// statement protocol, and this driver may run with interpolateParams. The value
// is derived from pageSize alone, which the caller has already clamped, so no
// caller-supplied text reaches the statement.
//
// Parameters:
//   - scope: the effective authorization scope.
//   - filter: the NORMALIZED filter.
//   - anchor: the previous page's last row, or nil for the first page.
//   - pageSize: rows requested, before the has-more probe row.
//
// Return values:
//   - string: the SQL statement.
//   - []any: its ordered arguments.
func buildLogCursorStatement(scope LogListScope, filter LogListFilter, anchor *LogCursorAnchor, pageSize int) (string, []any) {
	where, baseArgs := BuildLogListPredicate(scope, filter)
	limit := strconv.Itoa(pageSize + 1)
	order := " ORDER BY created_at DESC, id DESC LIMIT " + limit

	if anchor == nil {
		args := make([]any, 0, len(baseArgs))
		args = append(args, baseArgs...)
		return "SELECT " + logCursorColumns + " FROM logs WHERE " + where + order, args
	}

	var sb strings.Builder
	args := make([]any, 0, 2*len(baseArgs)+3)

	// Arm EQ: the remainder of the anchor's own second.
	sb.WriteString("SELECT * FROM (SELECT " + logCursorColumns + " FROM logs WHERE ")
	sb.WriteString(where)
	sb.WriteString(" AND created_at = ? AND id < ?")
	sb.WriteString(order)
	sb.WriteString(") AS eq_arm UNION ALL ")
	args = append(args, baseArgs...)
	args = append(args, anchor.CreatedAt, anchor.ID)

	// Arm LT: strictly older seconds.
	sb.WriteString("SELECT * FROM (SELECT " + logCursorColumns + " FROM logs WHERE ")
	sb.WriteString(where)
	sb.WriteString(" AND created_at < ?")
	sb.WriteString(order)
	sb.WriteString(") AS lt_arm")
	args = append(args, baseArgs...)
	args = append(args, anchor.CreatedAt)

	return "SELECT * FROM (" + sb.String() + ") AS page" + order, args
}

// FetchLogCursorPage returns one keyset page from the usage-owning handle.
//
// Parameters:
//   - ctx: request scope; cancellation aborts the query.
//   - scope: the effective authorization scope, derived from the request.
//   - filter: the NORMALIZED filter.
//   - anchor: the previous page's last row, or nil for the first page.
//   - pageSize: rows requested; values outside [1, maxLogCursorPageSize] are clamped.
//
// Return values:
//   - *LogCursorPage: the page, with channel names filled.
//   - error: wrapped failure from the query or the channel-name lookup.
func FetchLogCursorPage(ctx context.Context, scope LogListScope, filter LogListFilter, anchor *LogCursorAnchor, pageSize int) (*LogCursorPage, error) {
	if pageSize < 1 {
		pageSize = 1
	}
	if pageSize > maxLogCursorPageSize {
		pageSize = maxLogCursorPageSize
	}

	statement, args := buildLogCursorStatement(scope, filter, anchor, pageSize)

	var rows []*Log
	if err := LOG_DB.WithContext(ctx).Raw(statement, args...).Scan(&rows).Error; err != nil {
		return nil, errors.Wrap(err, "fetch log cursor page")
	}

	page := &LogCursorPage{}
	if len(rows) > pageSize {
		page.HasMore = true
		rows = rows[:pageSize]
	}
	page.Logs = rows

	if len(rows) > 0 {
		last := rows[len(rows)-1]
		page.Last = LogCursorAnchor{CreatedAt: last.CreatedAt, ID: int64(last.Id)}
	}

	if err := fillLogChannelNames(page.Logs); err != nil {
		return nil, errors.Wrap(err, "fill log cursor page channel names")
	}
	return page, nil
}

// LogCursorPageResponses converts a page into the external log shape.
//
// Parameters:
//   - page: the page to convert; a nil page yields an empty slice.
//
// Return values:
//   - []dto.LogResponse: the external rows, newest first.
func LogCursorPageResponses(page *LogCursorPage) []dto.LogResponse {
	if page == nil {
		return []dto.LogResponse{}
	}
	return LogsToResponses(page.Logs)
}
