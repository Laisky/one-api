package model

import (
	"context"
	_ "embed"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/dto"
)

// dashboardAggregateSQL preaggregates the selected window once, retaining raw
// nullable keys and database collations before deriving the six dashboard views.
//
//go:embed dashboard_aggregate.sql
var dashboardAggregateSQL string

// DashboardAggregates contains the existing six dashboard result sets. Quota
// balances and account status are intentionally not part of this cached bundle.
type DashboardAggregates struct {
	Logs          []*dto.LogStatistic            `json:"logs"`
	UserLogs      []*dto.LogStatisticByUser      `json:"user_logs"`
	TokenLogs     []*dto.LogStatisticByToken     `json:"token_logs"`
	ToolLogs      []*dto.ToolLogStatistic        `json:"tool_logs"`
	ToolUserLogs  []*dto.ToolLogStatisticByUser  `json:"tool_user_logs"`
	ToolTokenLogs []*dto.ToolLogStatisticByToken `json:"tool_token_logs"`
}

// dashboardAggregateRow is one grouped result, never an individual audit log.
// Quota remains int64 because tool DTOs deliberately support that full range.
type dashboardAggregateRow struct {
	Kind               int
	DayStart           int64
	ModelName          string
	Username           string
	UserID             int    `gorm:"column:user_id"`
	UserUUID           string `gorm:"column:user_uuid"`
	TokenName          string
	RequestCount       int
	Quota              int64
	PromptTokens       int
	CompletionTokens   int
	CachedPromptTokens int
	CacheHitCount      int
	CacheHitQuota      int
}

// SearchDashboardAggregatesWithContext returns all six views for the authorized
// user and half-open Unix-second window. A zero user ID means site-wide. It
// preserves the existing SQL grouping semantics and returns a wrapped error,
// never a partial bundle. Cancellation is propagated to the database.
func SearchDashboardAggregatesWithContext(ctx context.Context, userID, start, endExclusive int) (*DashboardAggregates, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "collect dashboard aggregates")
	}
	if LOG_DB == nil {
		return nil, errors.WithStack(errors.New("dashboard log database is unavailable"))
	}
	if !dashboardSupportsPreaggregation(LOG_DB) {
		return searchDashboardAggregatesLegacy(ctx, userID, start, endExclusive)
	}

	query, args := dashboardAggregateQuery(LOG_DB, userID, start, endExclusive)
	var rows []dashboardAggregateRow
	if err := LOG_DB.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, errors.Wrap(err, "collect dashboard aggregates from one log scan")
	}
	return dashboardAggregateResult(ctx, rows)
}

// dashboardSupportsPreaggregation reports whether db supports the shared CTE
// plan. Unknown MySQL versions, MySQL 5.x, and MariaDB keep the existing queries:
// their syntax or CTE reuse cannot be assumed from the MySQL dialector name.
func dashboardSupportsPreaggregation(db *gorm.DB) bool {
	switch dialectName(db) {
	case "sqlite", "postgres":
		return true
	case "mysql":
		dialector, ok := db.Dialector.(*mysql.Dialector)
		if !ok || dialector.Config == nil || strings.Contains(strings.ToLower(dialector.ServerVersion), "mariadb") {
			return false
		}
		major, err := strconv.Atoi(strings.SplitN(dialector.ServerVersion, ".", 2)[0])
		return err == nil && major >= 8
	default:
		return false
	}
}

// dashboardAggregateQuery returns the SQL and bound values for a dashboard
// window. Only fixed SQL fragments are substituted; no caller value becomes
// SQL text. The dialect is taken from the log handle, not global engine flags.
func dashboardAggregateQuery(db *gorm.DB, userID, start, endExclusive int) (string, []any) {
	materialized := ""
	if dialectName(db) == "sqlite" {
		materialized = "MATERIALIZED"
	}
	query := strings.ReplaceAll(dashboardAggregateSQL, "/*MATERIALIZED*/", materialized)
	filter := ""
	args := []any{start, endExclusive}
	if userID != 0 {
		filter = "AND user_id = ?"
		args = append(args, userID)
	}
	return strings.ReplaceAll(query, "/*USER_FILTER*/", filter), args
}

// dashboardAggregateResult converts grouped rows into the unchanged public DTOs.
// UTC date formatting happens only on output day changes, not on every raw log.
// It returns an error for an invalid view or a quota that cannot fit a consume DTO.
func dashboardAggregateResult(ctx context.Context, rows []dashboardAggregateRow) (*DashboardAggregates, error) {
	result := &DashboardAggregates{}
	var day string
	var previousDay int64
	for i, row := range rows {
		if i%256 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, errors.Wrap(err, "convert dashboard aggregates")
			}
		}
		if day == "" || row.DayStart != previousDay {
			day = time.Unix(row.DayStart, 0).UTC().Format("2006-01-02")
			previousDay = row.DayStart
		}
		quota := int(row.Quota)
		if row.Kind < 3 && int64(quota) != row.Quota {
			return nil, errors.WithStack(errors.New("dashboard consume quota exceeds integer range"))
		}
		switch row.Kind {
		case 0:
			result.Logs = append(result.Logs, &dto.LogStatistic{
				Day: day, ModelName: row.ModelName, RequestCount: row.RequestCount,
				Quota: quota, PromptTokens: row.PromptTokens, CompletionTokens: row.CompletionTokens,
				CachedPromptTokens: row.CachedPromptTokens, CacheHitCount: row.CacheHitCount, CacheHitQuota: row.CacheHitQuota,
			})
		case 1:
			result.UserLogs = append(result.UserLogs, &dto.LogStatisticByUser{
				Day: day, Username: row.Username, UserId: row.UserID, UserUUID: row.UserUUID,
				RequestCount: row.RequestCount, Quota: quota, PromptTokens: row.PromptTokens,
				CompletionTokens: row.CompletionTokens, CachedPromptTokens: row.CachedPromptTokens,
				CacheHitCount: row.CacheHitCount, CacheHitQuota: row.CacheHitQuota,
			})
		case 2:
			result.TokenLogs = append(result.TokenLogs, &dto.LogStatisticByToken{
				Day: day, Username: row.Username, UserId: row.UserID, UserUUID: row.UserUUID,
				TokenName: row.TokenName, RequestCount: row.RequestCount, Quota: quota,
				PromptTokens: row.PromptTokens, CompletionTokens: row.CompletionTokens,
				CachedPromptTokens: row.CachedPromptTokens, CacheHitCount: row.CacheHitCount, CacheHitQuota: row.CacheHitQuota,
			})
		case 3:
			result.ToolLogs = append(result.ToolLogs, &dto.ToolLogStatistic{
				Day: day, ToolName: row.ModelName, RequestCount: row.RequestCount, Quota: row.Quota,
			})
		case 4:
			result.ToolUserLogs = append(result.ToolUserLogs, &dto.ToolLogStatisticByUser{
				Day: day, Username: row.Username, UserId: row.UserID, UserUUID: row.UserUUID,
				RequestCount: row.RequestCount, Quota: row.Quota,
			})
		case 5:
			result.ToolTokenLogs = append(result.ToolTokenLogs, &dto.ToolLogStatisticByToken{
				Day: day, Username: row.Username, UserId: row.UserID, UserUUID: row.UserUUID,
				TokenName: row.TokenName, RequestCount: row.RequestCount, Quota: row.Quota,
			})
		default:
			return nil, errors.WithStack(errors.Errorf("unknown dashboard aggregate view %d", row.Kind))
		}
	}
	return result, nil
}

// searchDashboardAggregatesLegacy retains the six independent query functions
// for unsupported engines and as the differential benchmark's before arm.
// It returns the complete bundle or the first wrapped query error.
func searchDashboardAggregatesLegacy(ctx context.Context, userID, start, endExclusive int) (*DashboardAggregates, error) {
	a := &DashboardAggregates{}
	var err error
	if a.Logs, err = SearchLogsByDayAndModelWithContext(ctx, userID, start, endExclusive); err != nil {
		return nil, errors.Wrap(err, "get dashboard model usage")
	}
	if a.UserLogs, err = SearchLogsByDayAndUserWithContext(ctx, userID, start, endExclusive); err != nil {
		return nil, errors.Wrap(err, "get dashboard user usage")
	}
	if a.TokenLogs, err = SearchLogsByDayAndTokenWithContext(ctx, userID, start, endExclusive); err != nil {
		return nil, errors.Wrap(err, "get dashboard token usage")
	}
	if a.ToolLogs, err = SearchToolLogsByDayAndToolWithContext(ctx, userID, start, endExclusive); err != nil {
		return nil, errors.Wrap(err, "get dashboard tool usage")
	}
	if a.ToolUserLogs, err = SearchToolLogsByDayAndUserWithContext(ctx, userID, start, endExclusive); err != nil {
		return nil, errors.Wrap(err, "get dashboard tool user usage")
	}
	if a.ToolTokenLogs, err = SearchToolLogsByDayAndTokenWithContext(ctx, userID, start, endExclusive); err != nil {
		return nil, errors.Wrap(err, "get dashboard tool token usage")
	}
	return a, nil
}
