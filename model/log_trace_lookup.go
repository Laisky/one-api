package model

import (
	"context"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/idresolve"
)

// GetLogForTraceWithContext reads only the authoritative identity, ownership,
// correlation, and response fields needed to inspect a billing log's trace.
// Provider metadata is deliberately not decoded: it is unrelated to trace
// access and malformed historical metadata must not hide an intact trace.
// Parameters:
//   - ctx: request cancellation/deadline scope for every database operation.
//   - ref: public UUID reference, validated by the existing UUID-only boundary.
//
// Return values:
//   - *Log: a narrow log projection; never a complete billing record.
//   - error: wrapped invalid-reference, not-found, cancellation, or database error.
func GetLogForTraceWithContext(ctx context.Context, ref string) (*Log, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "read log trace correlation")
	}
	if LOG_DB == nil {
		return nil, errors.New("log database is unavailable")
	}
	id, err := idresolve.Resolve(func(value string) (int, error) {
		return resolvePublicIDByUUID(ctx, LOG_DB, "logs", value)
	}, ref)
	if err != nil {
		return nil, errors.Wrap(err, "resolve log trace reference")
	}
	var rows []Log
	if err := LOG_DB.WithContext(ctx).Raw(`SELECT id, uuid, user_id, user_uuid,
		channel_id, channel_uuid, username, content, type, trace_id
		FROM logs WHERE id = ? LIMIT 1`, id).Scan(&rows).Error; err != nil {
		return nil, errors.Wrap(err, "read log trace projection")
	}
	if len(rows) == 0 {
		return nil, errors.WithStack(gorm.ErrRecordNotFound)
	}
	return &rows[0], nil
}
