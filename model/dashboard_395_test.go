package model

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/benchdb"
)

// legacyDashboard395 executes the six production queries used before issue 395.
// It is an uncached executable control, not a simplified substitute query.
func legacyDashboard395(ctx context.Context, userID, start, end int) (*DashboardLogAggregates, error) {
	r := &DashboardLogAggregates{}
	var err error
	if r.Logs, err = SearchLogsByDayAndModelWithContext(ctx, userID, start, end); err != nil {
		return nil, err
	}
	if r.UserLogs, err = SearchLogsByDayAndUserWithContext(ctx, userID, start, end); err != nil {
		return nil, err
	}
	if r.TokenLogs, err = SearchLogsByDayAndTokenWithContext(ctx, userID, start, end); err != nil {
		return nil, err
	}
	if r.ToolLogs, err = SearchToolLogsByDayAndToolWithContext(ctx, userID, start, end); err != nil {
		return nil, err
	}
	if r.ToolUserLogs, err = SearchToolLogsByDayAndUserWithContext(ctx, userID, start, end); err != nil {
		return nil, err
	}
	if r.ToolTokenLogs, err = SearchToolLogsByDayAndTokenWithContext(ctx, userID, start, end); err != nil {
		return nil, err
	}
	return r, nil
}

// openDashboard395 creates the full GORM log schema in an isolated SQLite file
// or an explicitly opted-in benchmark database and restores global handles.
// It returns the log database; callers must not run these fixtures in parallel.
func openDashboard395(tb testing.TB, target benchdb.Target) *gorm.DB {
	tb.Helper()
	db, restore := benchdb.Open(tb, target)
	tb.Cleanup(restore)
	require.NoError(tb, db.AutoMigrate(&Log{}))
	benchdb.Reset(tb, db, "logs")
	prev := LOG_DB
	LOG_DB = db
	tb.Cleanup(func() { LOG_DB = prev })
	return db
}

// equalDashboard395 compares every field and preserves duplicate SQL groups.
// Undefined ordering within equal SQL sort keys is intentionally not asserted.
func equalDashboard395(tb testing.TB, want, got *DashboardLogAggregates) {
	tb.Helper()
	require.NotNil(tb, got)
	require.ElementsMatch(tb, want.Logs, got.Logs)
	require.ElementsMatch(tb, want.UserLogs, got.UserLogs)
	require.ElementsMatch(tb, want.TokenLogs, got.TokenLogs)
	require.ElementsMatch(tb, want.ToolLogs, got.ToolLogs)
	require.ElementsMatch(tb, want.ToolUserLogs, got.ToolUserLogs)
	require.ElementsMatch(tb, want.ToolTokenLogs, got.ToolTokenLogs)
}

// TestDashboard395Differential checks complete chart results against the old
// implementation across scopes, boundaries, nullable identities and log types.
// The optional database targets exercise real PostgreSQL/MySQL SQL semantics.
func TestDashboard395Differential(t *testing.T) {
	for _, target := range benchdb.Targets() {
		t.Run(string(target.Engine), func(t *testing.T) {
			db := openDashboard395(t, target)
			base := int(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC).Unix())
			for i := 0; i < 210; i++ {
				var uuid any = fmt.Sprintf("user-%d", i%3+1)
				if i%5 == 0 {
					uuid = nil
				}
				if i%11 == 0 {
					uuid = ""
				}
				var token any = fmt.Sprintf("token-%d", i%4)
				if i%13 == 0 {
					token = nil
				}
				if i%17 == 0 {
					token = ""
				}
				var modelName any = fmt.Sprintf("model-%d", i%5)
				if i%19 == 0 {
					modelName = nil
				}
				if i%23 == 0 {
					modelName = ""
				}
				// Include zero/negative adjustments, cache hits and every log type.
				cached := 0
				if i%2 == 0 {
					cached = 8
				}
				require.NoError(t, db.Exec(`INSERT INTO logs
				 (user_id,user_uuid,username,token_name,model_name,type,created_at,
				 quota,prompt_tokens,completion_tokens,cached_prompt_tokens)
				 VALUES (?,?,?,?,?,?,?,?,?,?,?)`, i%3+1, uuid, fmt.Sprintf("user-%d", i%3+1),
					token, modelName, i%8, base-1+i*997, i-100, i*2, i, cached).Error)
			}
			for _, scope := range []int{0, 1, 2, 999} {
				for _, window := range [][2]int{{base, base + 86400}, {base + 37, base + 2*86400}, {base, base}, {base + 2, base + 1}} {
					t.Run(fmt.Sprintf("user%d/%d-%d", scope, window[0], window[1]), func(t *testing.T) {
						want, err := legacyDashboard395(context.Background(), scope, window[0], window[1])
						require.NoError(t, err)
						got, err := SearchDashboardLogAggregatesWithContext(context.Background(), scope, window[0], window[1])
						require.NoError(t, err)
						equalDashboard395(t, want, got)
						if len(want.Logs) == 0 && len(want.ToolLogs) == 0 {
							wantJSON, err := json.Marshal(want)
							require.NoError(t, err)
							gotJSON, err := json.Marshal(got)
							require.NoError(t, err)
							require.JSONEq(t, string(wantJSON), string(gotJSON))
						}
					})
				}
			}
			// Raw bounds are half-open: include the last second, exclude midnight.
			benchdb.Reset(t, db, "logs")
			for _, ts := range []int{base - 1, base, base + 86399, base + 86400} {
				require.NoError(t, db.Exec("INSERT INTO logs (type,created_at,user_id,username,model_name,quota) VALUES (2,?,1,'boundary','m',10)", ts).Error)
			}
			got, err := SearchDashboardLogAggregatesWithContext(context.Background(), 1, base, base+86400)
			require.NoError(t, err)
			require.Len(t, got.Logs, 1)
			require.Equal(t, 2, got.Logs[0].RequestCount)
			require.Equal(t, 20, got.Logs[0].Quota)
			require.Equal(t, "2026-08-01", got.Logs[0].Day)
		})
	}
}

// TestDashboard395FreshLogState proves reconciliations, corrections and deletes
// are immediately visible without backfills or persistent summary invalidation.
func TestDashboard395FreshLogState(t *testing.T) {
	db := openDashboard395(t, benchdb.Target{Engine: benchdb.EngineSQLite})
	// The primary handle is deliberately unavailable: all reads must use LOG_DB.
	prev := DB
	DB = nil
	t.Cleanup(func() { DB = prev })
	require.NoError(t, db.Exec(`INSERT INTO logs (id,type,user_id,created_at,model_name,quota)
	 VALUES (1,6,1,1,'m',900),(2,7,1,1,'tool',31)`).Error)
	got, err := SearchDashboardLogAggregatesWithContext(context.Background(), 1, 0, 2)
	require.NoError(t, err)
	require.Empty(t, got.Logs)
	require.EqualValues(t, 31, got.ToolLogs[0].Quota)
	require.NoError(t, db.Exec("UPDATE logs SET type=2,quota=42,prompt_tokens=20,cached_prompt_tokens=7 WHERE id=1").Error)
	got, err = SearchDashboardLogAggregatesWithContext(context.Background(), 1, 0, 2)
	require.NoError(t, err)
	require.Equal(t, 42, got.Logs[0].Quota)
	require.Equal(t, 1, got.Logs[0].CacheHitCount)
	require.Equal(t, 42, got.Logs[0].CacheHitQuota)
	require.Equal(t, 7, got.TokenLogs[0].CachedPromptTokens)
	require.NoError(t, db.Exec("UPDATE logs SET quota=-5,cached_prompt_tokens=0 WHERE id=1").Error)
	got, err = SearchDashboardLogAggregatesWithContext(context.Background(), 1, 0, 2)
	require.NoError(t, err)
	require.Equal(t, -5, got.UserLogs[0].Quota)
	require.Zero(t, got.Logs[0].CacheHitQuota)
	require.NoError(t, db.Exec("DELETE FROM logs").Error)
	got, err = SearchDashboardLogAggregatesWithContext(context.Background(), 1, 0, 2)
	require.NoError(t, err)
	require.Equal(t, &DashboardLogAggregates{}, got)
}

// TestDashboard395CancellationAndFailure ensures failed requests return neither
// partial data nor success, and cancellation reaches the actual SQL driver.
func TestDashboard395CancellationAndFailure(t *testing.T) {
	db := openDashboard395(t, benchdb.Target{Engine: benchdb.EngineSQLite})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := SearchDashboardLogAggregatesWithContext(ctx, 0, 0, 1)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, got)
	ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	got, err = SearchDashboardLogAggregatesWithContext(ctx, 0, 0, 1)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, got)
	require.NoError(t, db.Exec("DROP TABLE logs").Error)
	got, err = SearchDashboardLogAggregatesWithContext(context.Background(), 0, 0, 1)
	require.Error(t, err)
	require.Nil(t, got)
}

// TestDashboard395UTCBeforeEpoch verifies integer day bucketing is floor-based
// and independent of the process local timezone, including negative timestamps.
func TestDashboard395UTCBeforeEpoch(t *testing.T) {
	db := openDashboard395(t, benchdb.Target{Engine: benchdb.EngineSQLite})
	prev := time.Local
	time.Local = time.FixedZone("test-offset", 9*3600)
	t.Cleanup(func() { time.Local = prev })
	for _, ts := range []int{-86401, -86400, -1, 0, 86399, 86400} {
		require.NoError(t, db.Exec("INSERT INTO logs (type,user_id,created_at,model_name,quota) VALUES (2,1,?,'m',1)", ts).Error)
	}
	want, err := legacyDashboard395(context.Background(), 0, -90000, 90000)
	require.NoError(t, err)
	got, err := SearchDashboardLogAggregatesWithContext(context.Background(), 0, -90000, 90000)
	require.NoError(t, err)
	equalDashboard395(t, want, got)
	require.Len(t, got.Logs, 4)
}
