package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/model"
)

// traceFlushSpy records calls to the process-wide sink flush operation. It
// proves trace lookup endpoints cannot make one caller flush another caller's
// buffered telemetry.
type traceFlushSpy struct {
	flushCalls atomic.Int32
}

// Submit implements tracing.TraceSink by accepting rows without storing them.
// Parameters are unused because this test double only observes Flush calls.
// Return values: nil.
func (s *traceFlushSpy) Submit(context.Context, *model.Trace) error { return nil }

// Flush implements tracing.TraceSink by recording that a global flush was
// requested. Parameters are unused because this test double does no I/O.
// Return values: nil.
func (s *traceFlushSpy) Flush(context.Context) error {
	s.flushCalls.Add(1)
	return nil
}

// Close implements tracing.TraceSink as a no-op for the test double.
// Parameters are unused because this test double owns no resources.
// Return values: nil.
func (s *traceFlushSpy) Close(context.Context) error { return nil }

// traceAccessTestRouter builds trace routes whose request context represents
// the viewer selected by X-Test-Viewer.
// Parameters:
//   - ownerID: id of the user that owns the fixture log.
//   - otherID: id of a different ordinary user.
//
// Return values:
//   - *gin.Engine: test router that invokes the production trace handlers.
func traceAccessTestRouter(ownerID, otherID int) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		gmw.SetLogger(c, logger.Logger)
		switch c.GetHeader("X-Test-Viewer") {
		case "owner":
			c.Set(ctxkey.Id, ownerID)
			c.Set(ctxkey.Role, model.RoleCommonUser)
		case "other":
			c.Set(ctxkey.Id, otherID)
			c.Set(ctxkey.Role, model.RoleCommonUser)
		case "admin":
			c.Set(ctxkey.Id, 0)
			c.Set(ctxkey.Role, model.RoleAdminUser)
		case "root":
			c.Set(ctxkey.Id, 0)
			c.Set(ctxkey.Role, model.RoleRootUser)
		}
		c.Next()
	})
	router.GET("/api/trace/:trace_id", GetTraceByTraceId)
	router.GET("/api/trace/log/:log_id", GetTraceByLogId)
	return router
}

// performTraceAccessRequest invokes a trace endpoint as the selected test
// viewer.
// Parameters:
//   - router: production-handler test router.
//   - path: requested endpoint path.
//   - viewer: identity selector consumed by traceAccessTestRouter.
//
// Return values:
//   - *httptest.ResponseRecorder: response from the production handler.
func performTraceAccessRequest(router *gin.Engine, path, viewer string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("X-Test-Viewer", viewer)
	router.ServeHTTP(recorder, request)
	return recorder
}

// TestTraceEndpointsEnforceOwnershipAndExposeRetentionState verifies ordinary
// users cannot read another user's trace or log correlation and that a missing
// locally retained trace does not trigger a process-wide writer flush.
// Parameters:
//   - t: active test handle.
//
// Return values: none.
func TestTraceEndpointsEnforceOwnershipAndExposeRetentionState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)

	other := &model.User{
		Username:    "trace-access-other",
		Password:    "hashed",
		AccessToken: "trace-access-other-token",
		AffCode:     "trace-access-other-aff-code",
		Group:       "default",
		Status:      model.UserStatusEnabled,
		Role:        model.RoleCommonUser,
	}
	require.NoError(t, model.DB.Create(other).Error)

	missingTraceLog := &model.Log{
		UserId:   fixture.user.Id,
		UserUUID: &fixture.user.UUID,
		Username: fixture.user.Username,
		TraceId:  "trace-not-retained-locally",
		Type:     model.LogTypeConsume,
		Content:  "trace was sampled out or exported externally",
	}
	require.NoError(t, model.LOG_DB.Create(missingTraceLog).Error)
	require.NotEmpty(t, missingTraceLog.UUID)

	flushSpy := &traceFlushSpy{}
	restoreSink := tracing.SetSinkForTest(flushSpy)
	t.Cleanup(restoreSink)

	router := traceAccessTestRouter(fixture.user.Id, other.Id)

	for _, testCase := range []struct {
		name       string
		path       string
		viewer     string
		statusCode int
	}{
		{name: "owner reads trace by trace id", path: "/api/trace/" + fixture.trace.TraceId, viewer: "owner", statusCode: http.StatusOK},
		{name: "other user cannot read trace by trace id", path: "/api/trace/" + fixture.trace.TraceId, viewer: "other", statusCode: http.StatusNotFound},
		{name: "administrator can read trace by trace id", path: "/api/trace/" + fixture.trace.TraceId, viewer: "admin", statusCode: http.StatusOK},
		{name: "root can read trace by trace id", path: "/api/trace/" + fixture.trace.TraceId, viewer: "root", statusCode: http.StatusOK},
		{name: "owner reads trace by log id", path: "/api/trace/log/" + fixture.log.UUID, viewer: "owner", statusCode: http.StatusOK},
		{name: "other user cannot read trace by log id", path: "/api/trace/log/" + fixture.log.UUID, viewer: "other", statusCode: http.StatusNotFound},
		{name: "administrator can read trace by log id", path: "/api/trace/log/" + fixture.log.UUID, viewer: "admin", statusCode: http.StatusOK},
		{name: "root can read trace by log id", path: "/api/trace/log/" + fixture.log.UUID, viewer: "root", statusCode: http.StatusOK},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := performTraceAccessRequest(router, testCase.path, testCase.viewer)
			require.Equal(t, testCase.statusCode, recorder.Code, recorder.Body.String())
		})
	}

	recorder := performTraceAccessRequest(router, "/api/trace/log/"+missingTraceLog.UUID, "owner")
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var payload struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Equal(t, "not_retained_locally", payload.Data["availability"])
	require.Equal(t, missingTraceLog.TraceId, payload.Data["trace_id"])
	require.Zero(t, flushSpy.flushCalls.Load(), "trace lookups must not flush the process-wide sink")
}
