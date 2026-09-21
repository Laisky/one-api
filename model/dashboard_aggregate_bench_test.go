package model

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/benchdb"
)

// dashboard395Seed is the raw-log fixture projection, including a 1 KiB audit
// payload that the aggregation must not transfer to the application.
type dashboard395Seed struct {
	UserID             int    `gorm:"column:user_id"`
	UserUUID           string `gorm:"column:user_uuid"`
	CreatedAt          int64
	Type               int
	ModelName          string
	Username           string
	TokenName          string
	Quota              int64
	PromptTokens       int
	CompletionTokens   int
	CachedPromptTokens int
	Content            string
	Metadata           string
}

// seedDashboard395 fills the isolated fixture table with exact-size deterministic
// workloads. Most traffic belongs to user 7; the tail belongs to 100 other users.
// The distinct arm assigns a unique token per row to expose materialization cost.
func seedDashboard395(tb testing.TB, db *gorm.DB, count, days int, distinct bool) {
	tb.Helper()
	const base = int64(1767225600)
	const batchSize = 500
	payload := strings.Repeat("x", 1024)
	for offset := 0; offset < count; offset += batchSize {
		batch := make([]dashboard395Seed, 0, batchSize)
		for i := offset; i < min(offset+batchSize, count); i++ {
			user := 7
			if i%10 == 0 {
				user = 100 + (i/10)%100
			}
			kind := LogTypeConsume
			if i%23 == 0 {
				kind = LogTypeTool
			}
			token := fmt.Sprintf("token-%02d", (i/11)%11)
			if distinct {
				token = fmt.Sprintf("token-%09d", i)
			}
			cached := 0
			if i%3 == 0 {
				cached = 25
			}
			batch = append(batch, dashboard395Seed{
				UserID: user, UserUUID: fmt.Sprintf("00000000-0000-4000-8000-%012d", user),
				CreatedAt: base + int64(i)*int64(days)*86400/int64(count), Type: kind,
				ModelName: fmt.Sprintf("model-or-tool-%02d", (i/19)%17), Username: fmt.Sprintf("user-%03d", user),
				TokenName: token, Quota: int64(50 + i%100), PromptTokens: 100 + i%200,
				CompletionTokens: 10 + i%20, CachedPromptTokens: cached, Content: payload, Metadata: "{}",
			})
		}
		require.NoError(tb, db.Table(dashboard395Table(db)).Create(&batch).Error)
	}
	var seeded int64
	require.NoError(tb, db.Table(dashboard395Table(db)).Count(&seeded).Error)
	require.EqualValues(tb, count, seeded, "a partial fixture must never produce a false speedup")
}

// BenchmarkDashboardAggregate395 compares the real six-query baseline and the
// production optimized path on the same fixture, excluding seeding and equality
// checks. Run explicitly with -run '^$' -bench '^BenchmarkDashboardAggregate395$'
// -benchmem -benchtime=3x -count=3. Native engines use ONEAPI_BENCH_*_DSN.
func BenchmarkDashboardAggregate395(b *testing.B) {
	for _, target := range benchdb.Targets() {
		for _, fixture := range []struct {
			rows, days int
			distinct   bool
		}{
			{290000, 7, false}, {1000000, 30, false}, {100000, 30, true},
		} {
			name := fmt.Sprintf("%s/rows_%d_days_%d_distinct_%t", target.Engine, fixture.rows, fixture.days, fixture.distinct)
			b.Run(name, func(b *testing.B) {
				db := dashboard395Database(b, target, false)
				require.True(b, dashboardSupportsPreaggregation(db), "do not benchmark a fallback as an optimization")
				seedDashboard395(b, db, fixture.rows, fixture.days, fixture.distinct)
				const start = 1767225600
				end := start + fixture.days*86400
				for _, user := range []int{0, 7} {
					want, err := searchDashboardAggregatesLegacy(context.Background(), user, start, end)
					require.NoError(b, err)
					got, err := SearchDashboardAggregatesWithContext(context.Background(), user, start, end)
					require.NoError(b, err)
					require.NotEmpty(b, want.Logs)
					require.NotEmpty(b, want.ToolLogs)
					requireDashboard395Equal(b, want, got)
					for _, optimized := range []bool{false, true} {
						b.Run(fmt.Sprintf("user_%d/optimized_%t", user, optimized), func(b *testing.B) {
							collect := searchDashboardAggregatesLegacy
							queries := 6.0
							if optimized {
								collect = SearchDashboardAggregatesWithContext
								queries = 1
							}
							b.ReportAllocs()
							b.ResetTimer()
							for range b.N {
								if _, err := collect(context.Background(), user, start, end); err != nil {
									b.Fatal(err)
								}
							}
							b.StopTimer()
							b.ReportMetric(queries, "queries/op")
						})
					}
				}
			})
		}
	}
}
