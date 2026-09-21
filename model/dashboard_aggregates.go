package model

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/dto"
)

// DashboardLogAggregates contains the six existing dashboard chart series.
// Nil slices intentionally retain the existing empty-window JSON contract.
type DashboardLogAggregates struct {
	Logs          []*dto.LogStatistic            `json:"logs"`
	UserLogs      []*dto.LogStatisticByUser      `json:"user_logs"`
	TokenLogs     []*dto.LogStatisticByToken     `json:"token_logs"`
	ToolLogs      []*dto.ToolLogStatistic        `json:"tool_logs"`
	ToolUserLogs  []*dto.ToolLogStatisticByUser  `json:"tool_user_logs"`
	ToolTokenLogs []*dto.ToolLogStatisticByToken `json:"tool_token_logs"`
}

// dashboardDayBucket computes UTC midnight without formatting every source row.
// The normalized remainder also floors pre-epoch timestamps correctly. It uses
// integer arithmetic supported by SQLite, PostgreSQL and MySQL.
const dashboardDayBucket = "created_at - ((created_at % 86400 + 86400) % 86400)"

const dashboardMetricSums = `COUNT(*) AS request_count,
 COALESCE(SUM(quota), 0) AS quota,
 COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
 COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
 COALESCE(SUM(cached_prompt_tokens), 0) AS cached_prompt_tokens,
 SUM(CASE WHEN cached_prompt_tokens > 0 THEN 1 ELSE 0 END) AS cache_hit_count,
 COALESCE(SUM(CASE WHEN cached_prompt_tokens > 0 THEN quota ELSE 0 END), 0) AS cache_hit_quota`

const dashboardMetricColumns = `request_count, quota, prompt_tokens, completion_tokens,
 cached_prompt_tokens, cache_hit_count, cache_hit_quota`

// dashboardAggregateQueries builds two parameterized reads for the supplied
// user and half-open Unix-second window. It returns the SQL and shared bindings.
// The first read groups models/tools. The second materializes token groups once
// and derives user groups in SQL, preserving database collation and NULL groups.
// This deliberately avoids a day/model/user/token cross-product and avoids
// persistent rollups, which can become stale after billing reconciliation.
func dashboardAggregateQueries(userID, start, endExclusive int) (string, string, []any) {
	where := "type IN (?, ?) AND created_at >= ? AND created_at < ?"
	args := []any{LogTypeConsume, LogTypeTool, start, endExclusive}
	if userID != 0 {
		where += " AND user_id = ?"
		args = append(args, userID)
	}
	models := `SELECT type, ` + dashboardDayBucket + ` AS day_bucket,
 CASE WHEN type = ` + strconv.Itoa(LogTypeTool) + ` THEN COALESCE(model_name, '') ELSE model_name END AS name,
 ` + dashboardMetricSums + ` FROM logs WHERE ` + where + `
 GROUP BY type, day_bucket, name ORDER BY type, day_bucket, name`

	// Keep the raw nullable identity columns inside the CTE. Coalescing them
	// before regrouping would merge NULL and empty-string historical groups.
	// GROUP BY prevents MySQL from merging this CTE; PostgreSQL and SQLite also
	// reuse this multiply referenced aggregate rather than rescanning logs.
	tokens := `WITH token_rows AS (
 SELECT type, ` + dashboardDayBucket + ` AS day_bucket,
 username, user_id, user_uuid, token_name, ` + dashboardMetricSums + `
 FROM logs WHERE ` + where + `
 GROUP BY type, day_bucket, username, user_id, user_uuid, token_name
 )
 SELECT * FROM (
 SELECT 0 AS by_token, type, day_bucket, username, user_id, user_uuid,
 NULL AS token_name,
 SUM(request_count) AS request_count, SUM(quota) AS quota,
 SUM(prompt_tokens) AS prompt_tokens, SUM(completion_tokens) AS completion_tokens,
 SUM(cached_prompt_tokens) AS cached_prompt_tokens,
 SUM(cache_hit_count) AS cache_hit_count, SUM(cache_hit_quota) AS cache_hit_quota
 FROM token_rows GROUP BY type, day_bucket, username, user_id, user_uuid
 UNION ALL
 SELECT 1 AS by_token, type, day_bucket, username, user_id, user_uuid, token_name,
 ` + dashboardMetricColumns + ` FROM token_rows
 ) AS dashboard_rows
 ORDER BY by_token, type, day_bucket, username,
 CASE WHEN type = ` + strconv.Itoa(LogTypeTool) + ` THEN user_id ELSE 0 END, token_name, user_id, user_uuid`
	return models, tokens, args
}

// dashboardAggregateMetrics holds one aggregate's numeric values. Quota remains
// int64 so tool costs retain their existing wire precision.
type dashboardAggregateMetrics struct {
	requests, prompt, completion, cached, hits, hitQuota int
	quota                                                int64
}

// scanDestinations returns destinations for the SQL metric columns in order.
func (m *dashboardAggregateMetrics) scanDestinations() []any {
	return []any{&m.requests, &m.quota, &m.prompt, &m.completion, &m.cached, &m.hits, &m.hitQuota}
}

// SearchDashboardLogAggregatesWithContext returns all six chart series from two
// log-table reads. Parameters select a user (zero means site-wide) and the
// half-open Unix-second window. Database, scan and cancellation errors return no
// partial bundle. No response cache is consulted here.
func SearchDashboardLogAggregatesWithContext(ctx context.Context, userID, start, endExclusive int) (*DashboardLogAggregates, error) {
	started := time.Now()
	modelsQuery, tokensQuery, args := dashboardAggregateQueries(userID, start, endExclusive)
	result := &DashboardLogAggregates{}
	days := make(map[int64]string)
	if err := readDashboardModelGroups(ctx, modelsQuery, args, result, days); err != nil {
		return nil, errors.Wrap(err, "read dashboard model and tool groups")
	}
	if err := readDashboardTokenGroups(ctx, tokensQuery, args, result, days); err != nil {
		return nil, errors.Wrap(err, "read dashboard token and user groups")
	}
	logger.FromContext(ctx).Debug("collected dashboard aggregates",
		zap.Duration("duration", time.Since(started)),
		zap.Int("queries", 2),
		zap.Int("groups", len(result.Logs)+len(result.UserLogs)+len(result.TokenLogs)+
			len(result.ToolLogs)+len(result.ToolUserLogs)+len(result.ToolTokenLogs)))
	return result, nil
}

// dashboardDay formats one UTC bucket, reusing labels across all chart groups.
// Parameters are the request-local label map and bucket; the result is a date.
func dashboardDay(days map[int64]string, bucket int64) string {
	if day, ok := days[bucket]; ok {
		return day
	}
	day := time.Unix(bucket, 0).UTC().Format("2006-01-02")
	days[bucket] = day
	return day
}

// readDashboardModelGroups streams the first grouped query into result using
// the request-local date cache. It returns a wrapped query or scan failure.
func readDashboardModelGroups(ctx context.Context, query string, args []any, result *DashboardLogAggregates, days map[int64]string) error {
	rows, err := LOG_DB.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return errors.Wrap(err, "query model groups")
	}
	defer rows.Close()
	for rows.Next() {
		var logType int
		var bucket int64
		var name sql.NullString
		var m dashboardAggregateMetrics
		dest := append([]any{&logType, &bucket, &name}, m.scanDestinations()...)
		if err := rows.Scan(dest...); err != nil {
			return errors.Wrap(err, "scan model group")
		}
		day := dashboardDay(days, bucket)
		if logType == LogTypeTool {
			result.ToolLogs = append(result.ToolLogs, &dto.ToolLogStatistic{
				Day: day, ToolName: name.String, RequestCount: m.requests, Quota: m.quota,
			})
			continue
		}
		if int64(int(m.quota)) != m.quota {
			return errors.WithStack(errors.New("dashboard model quota exceeds int range"))
		}
		result.Logs = append(result.Logs, &dto.LogStatistic{
			Day: day, ModelName: name.String, RequestCount: m.requests, Quota: int(m.quota),
			PromptTokens: m.prompt, CompletionTokens: m.completion, CachedPromptTokens: m.cached,
			CacheHitCount: m.hits, CacheHitQuota: m.hitQuota,
		})
	}
	return errors.Wrap(rows.Err(), "iterate model groups")
}

// readDashboardTokenGroups streams token/user groups, preserving the SQL order
// and identity grouping. It returns a wrapped failure and never caches results.
func readDashboardTokenGroups(ctx context.Context, query string, args []any, result *DashboardLogAggregates, days map[int64]string) error {
	rows, err := LOG_DB.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return errors.Wrap(err, "query token and user groups")
	}
	defer rows.Close()
	for rows.Next() {
		var byToken, logType int
		var bucket int64
		var userID sql.NullInt64
		var username, userUUID, tokenName sql.NullString
		var m dashboardAggregateMetrics
		dest := append([]any{&byToken, &logType, &bucket, &username, &userID, &userUUID, &tokenName}, m.scanDestinations()...)
		if err := rows.Scan(dest...); err != nil {
			return errors.Wrap(err, "scan token or user group")
		}
		day := dashboardDay(days, bucket)
		if logType == LogTypeTool {
			if byToken == 1 {
				result.ToolTokenLogs = append(result.ToolTokenLogs, &dto.ToolLogStatisticByToken{
					Day: day, Username: username.String, UserId: int(userID.Int64), UserUUID: userUUID.String,
					TokenName: tokenName.String, RequestCount: m.requests, Quota: m.quota,
				})
			} else {
				result.ToolUserLogs = append(result.ToolUserLogs, &dto.ToolLogStatisticByUser{
					Day: day, Username: username.String, UserId: int(userID.Int64), UserUUID: userUUID.String,
					RequestCount: m.requests, Quota: m.quota,
				})
			}
			continue
		}
		if int64(int(m.quota)) != m.quota {
			return errors.WithStack(errors.New("dashboard user quota exceeds int range"))
		}
		if byToken == 1 {
			result.TokenLogs = append(result.TokenLogs, &dto.LogStatisticByToken{
				Day: day, Username: username.String, UserId: int(userID.Int64), UserUUID: userUUID.String,
				TokenName: tokenName.String, RequestCount: m.requests, Quota: int(m.quota),
				PromptTokens: m.prompt, CompletionTokens: m.completion, CachedPromptTokens: m.cached,
				CacheHitCount: m.hits, CacheHitQuota: m.hitQuota,
			})
		} else {
			result.UserLogs = append(result.UserLogs, &dto.LogStatisticByUser{
				Day: day, Username: username.String, UserId: int(userID.Int64), UserUUID: userUUID.String,
				RequestCount: m.requests, Quota: int(m.quota), PromptTokens: m.prompt,
				CompletionTokens: m.completion, CachedPromptTokens: m.cached,
				CacheHitCount: m.hits, CacheHitQuota: m.hitQuota,
			})
		}
	}
	return errors.Wrap(rows.Err(), "iterate token and user groups")
}
