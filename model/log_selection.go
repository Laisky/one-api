package model

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/errkind"
)

// logSelectionQuery combines the authenticated owner scope with the same predicates as the displayed log list.
func logSelectionQuery(ctx context.Context, scope LogListScope, filter LogListFilter, keyword string) (*gorm.DB, error) {
	if scope.PrincipalUserID <= 0 || scope.Kind == LogListScopeAll && scope.Role < RoleAdminUser ||
		scope.Kind == LogListScopeSelf && scope.SubjectUserID != scope.PrincipalUserID ||
		scope.Kind != LogListScopeSelf && scope.Kind != LogListScopeAll {
		return nil, errkind.InvalidRequestErr(errors.New("Invalid log selection scope."))
	}
	if len(keyword) > 255 {
		return nil, errkind.InvalidRequestErr(errors.New("Log search is too long."))
	}
	// Keyword search is a separate existing route, not an extra form filter.
	// Keep its universe identical to SearchAllLogs/SearchUserLogs.
	if keyword != "" {
		query := excludeProvisionalScope(LOG_DB.WithContext(ctx).Model(&Log{}))
		if scope.Kind == LogListScopeSelf {
			query = query.Where("user_id = ?", scope.SubjectUserID)
		}
		return applyLogKeyword(query, keyword), nil
	}
	where, args := BuildLogListPredicate(scope, filter.Normalize(scope))
	return LOG_DB.WithContext(ctx).Model(&Log{}).Where(where, args...), nil
}

// applyLogKeyword is shared by keyword lists and selection resolution, including exact public UUID lookups.
func applyLogKeyword(query *gorm.DB, keyword string) *gorm.DB {
	if keyword == "" {
		return query
	}
	if scoped, matched := applyUUIDKeyword(query, keyword, "uuid", "user_uuid", "token_uuid", "channel_uuid"); matched {
		return scoped
	}
	return query.Where("(content LIKE ?)", "%"+keyword+"%")
}

// SelectLogs returns the authorized selected rows in the requested list order, with an explicit size bound.
func SelectLogs(ctx context.Context, scope LogListScope, filter LogListFilter, keyword string, selection ListSelection, sortBy, sortOrder string) ([]*Log, error) {
	normalized, err := selection.Normalize()
	if err != nil {
		return nil, errors.Wrap(err, "validate log selection")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	query, err := logSelectionQuery(ctx, scope, filter, keyword)
	if err != nil {
		return nil, errors.Wrap(err, "scope selected logs")
	}
	rows, err := applyListSelection(query, normalized).Order(GetLogOrderClause(sortBy, sortOrder)).Limit(MaxListSelection + 1).Rows()
	if err != nil {
		return nil, errors.Wrap(err, "read selected logs")
	}
	defer rows.Close()
	var logs []*Log
	bytesRead := 0
	for rows.Next() {
		var row Log
		if err := query.ScanRows(rows, &row); err != nil {
			return nil, errors.Wrap(err, "scan selected log")
		}
		encoded, err := json.Marshal(row.ToResponse())
		if err != nil {
			return nil, errors.Wrap(err, "measure selected log")
		}
		bytesRead += len(encoded)
		if len(logs) >= MaxListSelection || bytesRead > 16<<20 {
			return nil, errkind.InvalidRequestErr(errors.New("Selection exceeds 10000 records or 16 MiB; narrow the filters. No partial selection was returned."))
		}
		logs = append(logs, &row)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "read selected log rows")
	}
	if err := rows.Close(); err != nil {
		return nil, errors.Wrap(err, "close selected log rows")
	}
	if err := fillLogChannelNames(logs); err != nil {
		return nil, errors.Wrap(err, "fill selected log channel names")
	}
	return logs, nil
}

// DeleteSelectedLogs deletes an explicit confirmed snapshot in the log database; implicit-all requests are rejected.
func DeleteSelectedLogs(ctx context.Context, selection ListSelection) (int64, error) {
	normalized, err := selection.Normalize()
	if err != nil {
		return 0, errors.Wrap(err, "validate selected log deletion")
	}
	if normalized.Mode != "ids" {
		return 0, errkind.InvalidRequestErr(errors.New("Confirm explicit log UUIDs before deletion."))
	}
	result := applyListSelection(excludeProvisionalScope(LOG_DB.WithContext(ctx).Model(&Log{})), normalized).Delete(&Log{})
	if result.Error != nil {
		return 0, errors.Wrap(result.Error, "delete selected logs")
	}
	return result.RowsAffected, nil
}
