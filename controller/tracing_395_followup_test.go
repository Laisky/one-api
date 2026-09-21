package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/model"
)

// TestTrace395CorrelationReadContract verifies the correlation-ID route can
// serve the log-details view without another UUID/full-log lookup. Parameters:
// t is the test handle. Returns: none. Ownership and missing-retention semantics
// remain explicit; a database failure must never masquerade as a missing trace.
func TestTrace395CorrelationReadContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)
	router := traceAccessTestRouter(fixture.user.Id, fixture.user.Id+100)
	spy := &traceFlushSpy{}
	t.Cleanup(tracing.SetSinkForTest(spy))

	for _, viewer := range []string{"owner", "admin", "root"} {
		t.Run(viewer+"/retained", func(t *testing.T) {
			recorder := performTraceAccessRequest(router, "/api/trace/"+fixture.trace.TraceId, viewer)
			data := decodeUUIDContractData(t, recorder)
			require.Equal(t, fixture.trace.TraceId, data["trace_id"])
			require.Equal(t, fixture.trace.UUID, data["uuid"])
			require.NotContains(t, data, "id")
			durations, ok := data["durations"].(map[string]any)
			require.True(t, ok, "correlation reads must include the same durations used by log details")
			require.EqualValues(t, 3, durations["total_time"])
		})
	}
	t.Run("other owner stays hidden", func(t *testing.T) {
		recorder := performTraceAccessRequest(router, "/api/trace/"+fixture.trace.TraceId, "other")
		require.Equal(t, http.StatusNotFound, recorder.Code)
		require.NotContains(t, recorder.Body.String(), fixture.trace.URL)
	})

	missing := &model.Log{
		UserId: fixture.user.Id, UserUUID: &fixture.user.UUID,
		TraceId: "issue395-retention-miss", Type: model.LogTypeConsume,
	}
	require.NoError(t, model.LOG_DB.Create(missing).Error)
	for _, viewer := range []string{"owner", "admin", "root"} {
		t.Run(viewer+"/not retained", func(t *testing.T) {
			recorder := performTraceAccessRequest(router, "/api/trace/"+missing.TraceId, viewer)
			data := decodeUUIDContractData(t, recorder)
			require.Equal(t, traceAvailabilityNotRetainedLocally, data["availability"])
			require.Equal(t, missing.TraceId, data["trace_id"])
			require.NotContains(t, data, "timestamps")
		})
	}
	t.Run("not-retained correlation is not an ownership bypass", func(t *testing.T) {
		recorder := performTraceAccessRequest(router, "/api/trace/"+missing.TraceId, "other")
		require.Equal(t, http.StatusNotFound, recorder.Code)
		require.NotContains(t, recorder.Body.String(), missing.TraceId)
	})

	t.Run("ordinary user unknown correlation stays hidden", func(t *testing.T) {
		recorder := performTraceAccessRequest(router, "/api/trace/issue395-no-owned-log", "owner")
		require.Equal(t, http.StatusNotFound, recorder.Code)
	})
	t.Run("real database failure is not a retention miss", func(t *testing.T) {
		failure := errors.New("injected internal database detail must not reach the response")
		require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register("issue395:trace_failure", func(tx *gorm.DB) {
			if tx.Statement.Table == "traces" { tx.AddError(failure) }
		}))
		t.Cleanup(func() { require.NoError(t, model.DB.Callback().Query().Remove("issue395:trace_failure")) })
		recorder := performTraceAccessRequest(router, "/api/trace/"+fixture.trace.TraceId, "root")
		require.Equal(t, http.StatusInternalServerError, recorder.Code, recorder.Body.String())
		require.NotContains(t, recorder.Body.String(), failure.Error())
		require.NotContains(t, recorder.Body.String(), traceAvailabilityNotRetainedLocally)
	})
	require.Zero(t, spy.flushCalls.Load(), "trace inspection must not flush every buffered trace")
}

// TestTrace395AdminCorrelationReadAvoidsLogDependency verifies the already
// authorized admin trace route reads exactly one trace and does not require a
// second read of a billing log shown in the browser. Parameters: t is the test
// handle. Returns: none. The log route remains UUID-only; no numeric-ID bypass
// is introduced to repair a client that already holds the correlation ID.
func TestTrace395AdminCorrelationReadAvoidsLogDependency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)
	router := traceAccessTestRouter(fixture.user.Id, fixture.user.Id+100)
	queries := 0
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register("issue395:query_budget", func(tx *gorm.DB) {
		queries++
		if tx.Statement.Table == "logs" { tx.AddError(errors.New("billing log read is unavailable")) }
	}))
	require.NoError(t, model.DB.Callback().Row().Before("gorm:row").Register("issue395:raw_budget", func(tx *gorm.DB) {
		queries++
		if strings.Contains(tx.Statement.SQL.String(), "logs") { tx.AddError(errors.New("billing log lookup is unavailable")) }
	}))
	legacy := performTraceAccessRequest(router, "/api/trace/log/"+fixture.log.UUID, "root")
	require.NotEqual(t, http.StatusOK, legacy.Code, "negative control must reach the unavailable log dependency")
	queries = 0
	recorder := performTraceAccessRequest(router, "/api/trace/"+fixture.trace.TraceId, "root")
	data := decodeUUIDContractData(t, recorder)
	require.Equal(t, fixture.trace.TraceId, data["trace_id"])
	require.Equal(t, 1, queries, "an administrator needs only the trace-ID lookup")
}

// TestTrace395LogReadCancellation verifies cancelled requests do not start
// uncancelled UUID/log reads before the trace query. Parameters: t is the test
// handle. Returns: none. Tests inspect the contexts at the real GORM boundary.
func TestTrace395LogReadCancellation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)
	router := traceAccessTestRouter(fixture.user.Id, fixture.user.Id+100)
	uncancelled := 0
	observe := func(tx *gorm.DB) {
		if tx.Statement.Context.Err() == nil { uncancelled++ }
	}
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register("issue395:query_context", observe))
	require.NoError(t, model.DB.Callback().Row().Before("gorm:row").Register("issue395:raw_context", observe))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/trace/log/"+fixture.log.UUID, nil).WithContext(ctx)
	request.Header.Set("X-Test-Viewer", "owner")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var response struct { Success bool `json:"success"` }
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.False(t, response.Success)
	require.Zero(t, uncancelled, "cancelled trace inspection must not issue background log queries")
}
