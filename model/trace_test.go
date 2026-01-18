package model

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/logger"
)

// TestCreateTraceWithLongURL verifies that trace creation succeeds even when the request URL includes very long query strings.
func TestCreateTraceWithLongURL(t *testing.T) {
	setupTestDatabase(t)

	require.NoError(t, DB.Exec("DELETE FROM traces WHERE trace_id LIKE 'test-trace-long-url%'").Error)

	longURL := "/api/verification?token=" + strings.Repeat("abc123", 1000)
	require.Greater(t, len(longURL), maxTraceURLLength)

	ctx := gmw.SetLogger(context.Background(), logger.Logger)

	traceID := "test-trace-long-url"
	trace, err := CreateTrace(ctx, traceID, longURL, "GET", 0)
	require.NoError(t, err)
	require.NotNil(t, trace)
	require.Equal(t, maxTraceURLLength, len(trace.URL))

	var stored Trace
	err = DB.Where("trace_id = ?", traceID).First(&stored).Error
	require.NoError(t, err)
	require.Equal(t, maxTraceURLLength, len(stored.URL))
	require.Equal(t, trace.URL, stored.URL)
}

func TestCreateTraceURLWithinLimit(t *testing.T) {
	setupTestDatabase(t)
	require.NoError(t, DB.Exec("DELETE FROM traces WHERE trace_id = 'test-trace-within-limit'").Error)

	url := "/api/status"
	require.LessOrEqual(t, len(url), maxTraceURLLength)
	ctx := gmw.SetLogger(context.Background(), logger.Logger)
	trace, err := CreateTrace(ctx, "test-trace-within-limit", url, "GET", 0)
	require.NoError(t, err)
	require.Equal(t, url, trace.URL)

	var stored Trace
	err = DB.Where("trace_id = ?", "test-trace-within-limit").First(&stored).Error
	require.NoError(t, err)
	require.Equal(t, url, stored.URL)
}

func TestTraceDBSessionDisablesPreparedStatementsOnPostgres(t *testing.T) {
	setupTestDatabase(t)
	prev := common.UsingPostgreSQL.Load()
	common.UsingPostgreSQL.Store(true)
	t.Cleanup(func() { common.UsingPostgreSQL.Store(prev) })

	session := traceDBWithContext(context.Background())
	require.False(t, session.Config.PrepareStmt)

	sessionGin := traceDBWithGin(nil)
	require.False(t, sessionGin.Config.PrepareStmt)
}

func TestUpdateTraceTimestampWithPostgresSession(t *testing.T) {
	setupTestDatabase(t)
	prev := common.UsingPostgreSQL.Load()
	common.UsingPostgreSQL.Store(true)
	t.Cleanup(func() { common.UsingPostgreSQL.Store(prev) })

	const traceID = "test-trace-postgres-session"
	require.NoError(t, DB.Exec("DELETE FROM traces WHERE trace_id = ?", traceID).Error)

	trace := &Trace{
		TraceId:    traceID,
		URL:        "/api/test",
		Method:     "GET",
		Timestamps: `{"request_received": 1}`,
	}
	require.NoError(t, DB.Create(trace).Error)

	require.NoError(t, UpdateTraceTimestamp(nil, traceID, TimestampRequestCompleted))

	var stored Trace
	require.NoError(t, DB.Where("trace_id = ?", traceID).First(&stored).Error)

	var parsed TraceTimestamps
	require.NoError(t, json.Unmarshal([]byte(stored.Timestamps), &parsed))
	require.NotNil(t, parsed.RequestCompleted)
}

func TestUpdateTraceStatusWithCanceledContext(t *testing.T) {
	setupTestDatabase(t)

	const traceID = "test-trace-status-canceled"
	require.NoError(t, DB.Exec("DELETE FROM traces WHERE trace_id = ?", traceID).Error)

	baseCtx := gmw.SetLogger(context.Background(), logger.Logger)
	_, err := CreateTrace(baseCtx, traceID, "/api/test", "GET", 0)
	require.NoError(t, err)

	canceledCtx, cancel := context.WithCancel(baseCtx)
	cancel()

	err = UpdateTraceStatus(canceledCtx, traceID, 207)
	require.NoError(t, err)

	var stored Trace
	require.NoError(t, DB.Where("trace_id = ?", traceID).First(&stored).Error)
	require.Equal(t, 207, stored.Status)
}

// TestAppendTraceExternalCall verifies external call entries are appended to trace timestamps.
func TestAppendTraceExternalCall(t *testing.T) {
	setupTestDatabase(t)

	const traceID = "test-trace-external-call"
	require.NoError(t, DB.Exec("DELETE FROM traces WHERE trace_id = ?", traceID).Error)

	trace := &Trace{
		TraceId:    traceID,
		URL:        "/api/test",
		Method:     "GET",
		Timestamps: `{"request_received": 1}`,
	}
	require.NoError(t, DB.Create(trace).Error)

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest("GET", "/api/test", nil)

	call := TraceExternalCall{
		Source:     "mcp",
		Tool:       "web_search",
		ServerID:   7,
		StartedAt:  100,
		EndedAt:    220,
		DurationMs: 120,
	}
	require.NoError(t, AppendTraceExternalCall(ginCtx, traceID, call))

	var stored Trace
	require.NoError(t, DB.Where("trace_id = ?", traceID).First(&stored).Error)

	var parsed TraceTimestamps
	require.NoError(t, json.Unmarshal([]byte(stored.Timestamps), &parsed))
	require.Len(t, parsed.ExternalCalls, 1)
	require.Equal(t, "mcp", parsed.ExternalCalls[0].Source)
	require.Equal(t, "web_search", parsed.ExternalCalls[0].Tool)
}
