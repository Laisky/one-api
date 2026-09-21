package model

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/benchdb"
)

// dashboard395Fixture describes a reproducible issue-scale workload.
type dashboard395Fixture struct {
	rows, days int
}

// seedDashboard395 writes a deterministic, chronological, wide-row workload
// through batched SQL into the real indexed Log schema. Seeding and verification
// are outside benchmark timing. It returns the half-open timestamp window.
func seedDashboard395(tb testing.TB, db *gorm.DB, fixture dashboard395Fixture) (int, int) {
	tb.Helper()
	start := int(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC).Unix())
	end := start + fixture.days*86400
	const batchSize = 500
	content := strings.Repeat("synthetic usage ", 24)
	for first := 0; first < fixture.rows; first += batchSize {
		last := min(first+batchSize, fixture.rows)
		var query strings.Builder
		query.WriteString(`INSERT INTO logs (uuid,user_id,user_uuid,created_at,type,username,token_name,
		 model_name,quota,prompt_tokens,completion_tokens,cached_prompt_tokens,content,metadata) VALUES `)
		args := make([]any, 0, (last-first)*14)
		for i := first; i < last; i++ {
			if i != first {
				query.WriteByte(',')
			}
			query.WriteString("(?,?,?,?,?,?,?,?,?,?,?,?,?,?)")
			// Mix billing and tool rows with unfinalized/nonbilling traffic. User,
			// token, model and type periods differ to avoid accidental correlation.
			user := (i*17+i/97)%40 + 1
			logType := LogTypeConsume
			if i%13 == 0 {
				logType = LogTypeTool
			}
			if i%101 == 0 {
				logType = LogTypeProvisional
			}
			if i%211 == 0 {
				logType = LogTypeTest
			}
			cached := 0
			if i%3 == 0 {
				cached = 31
			}
			args = append(args, fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1), user,
				fmt.Sprintf("10000000-0000-4000-8000-%012d", user),
				start+int(int64(i)*int64(end-start)/int64(fixture.rows)), logType,
				fmt.Sprintf("user-%02d", user), fmt.Sprintf("token-%d", (i/7)%4),
				fmt.Sprintf("model-%02d", (i/11)%12), 17+i%97, 80+i%200, 11+i%31, cached,
				content, `{"user_api_format":"chat","upstream_api_format":"openai"}`)
		}
		require.NoError(tb, db.Exec(query.String(), args...).Error)
	}
	var count int64
	require.NoError(tb, db.Model(&Log{}).Count(&count).Error)
	require.EqualValues(tb, fixture.rows, count, "the fixture must be complete before timing")
	// Let each optimizer see the fixture, without forcing an index or changing
	// production engine settings for either comparison arm.
	analyze := "ANALYZE logs"
	if db.Dialector.Name() == "mysql" {
		analyze = "ANALYZE TABLE logs"
	}
	require.NoError(tb, db.Exec(analyze).Error)
	return start, end
}

// verifyDashboard395Totals independently reconciles all three model chart totals
// and tool totals with direct filtered SUM/COUNT queries. It rejects fast but
// incomplete data, separately from the differential comparison with legacy SQL.
func verifyDashboard395Totals(tb testing.TB, db *gorm.DB, userID, start, end int, got *DashboardLogAggregates) {
	tb.Helper()
	var counts []struct {
		Type     int
		Requests int
		Quota    int64
	}
	query := "SELECT type, COUNT(*) AS requests, COALESCE(SUM(quota),0) AS quota FROM logs WHERE created_at >= ? AND created_at < ?"
	args := []any{start, end}
	if userID != 0 {
		query += " AND user_id = ?"
		args = append(args, userID)
	}
	require.NoError(tb, db.Raw(query+" GROUP BY type", args...).Scan(&counts).Error)
	for _, count := range counts {
		var requests [3]int
		var quotas [3]int64
		switch count.Type {
		case LogTypeConsume:
			for _, r := range got.Logs {
				requests[0] += r.RequestCount
				quotas[0] += int64(r.Quota)
			}
			for _, r := range got.UserLogs {
				requests[1] += r.RequestCount
				quotas[1] += int64(r.Quota)
			}
			for _, r := range got.TokenLogs {
				requests[2] += r.RequestCount
				quotas[2] += int64(r.Quota)
			}
		case LogTypeTool:
			for _, r := range got.ToolLogs {
				requests[0] += r.RequestCount
				quotas[0] += r.Quota
			}
			for _, r := range got.ToolUserLogs {
				requests[1] += r.RequestCount
				quotas[1] += r.Quota
			}
			for _, r := range got.ToolTokenLogs {
				requests[2] += r.RequestCount
				quotas[2] += r.Quota
			}
		default:
			continue
		}
		for i := range requests {
			require.Equal(tb, count.Requests, requests[i])
			require.Equal(tb, count.Quota, quotas[i])
		}
	}
}

// BenchmarkDashboard395 measures the actual old/new cold aggregate computation
// at the reported 290k/7-day and 1m/30-day scales. Both arms bypass Redis and
// include SQL execution, transfer, scanning and all six DTO result sets. The DB
// buffer cache is warm, not artificially flushed, and seeding is never timed.
// Use -benchtime=3x -count=3 -benchmem with dedicated optional benchmark DSNs.
func BenchmarkDashboard395(b *testing.B) {
	for _, target := range benchdb.Targets() {
		for _, fixture := range []dashboard395Fixture{{290_000, 7}, {1_000_000, 30}} {
			b.Run(fmt.Sprintf("%s/%drows_%ddays", target.Engine, fixture.rows, fixture.days), func(b *testing.B) {
				db := openDashboard395(b, target)
				start, end := seedDashboard395(b, db, fixture)
				for _, userID := range []int{0, 1} {
					b.Run(fmt.Sprintf("user%d", userID), func(b *testing.B) {
						want, err := legacyDashboard395(context.Background(), userID, start, end)
						require.NoError(b, err)
						got, err := SearchDashboardLogAggregatesWithContext(context.Background(), userID, start, end)
						require.NoError(b, err)
						equalDashboard395(b, want, got)
						verifyDashboard395Totals(b, db, userID, start, end, got)
						groups := len(got.Logs) + len(got.UserLogs) + len(got.TokenLogs) + len(got.ToolLogs) + len(got.ToolUserLogs) + len(got.ToolTokenLogs)
						for _, optimized := range []bool{false, true} {
							name, queries := "legacy", 6
							collect := legacyDashboard395
							if optimized {
								name, queries, collect = "optimized", 3, SearchDashboardLogAggregatesWithContext
							}
							b.Run(name, func(b *testing.B) {
								b.ReportAllocs()
								b.ResetTimer()
								for range b.N {
									if _, err := collect(context.Background(), userID, start, end); err != nil {
										b.Fatal(err)
									}
								}
								b.ReportMetric(float64(queries), "queries/op")
								b.ReportMetric(float64(groups), "groups/op")
							})
						}
					})
				}
			})
		}
	}
}
