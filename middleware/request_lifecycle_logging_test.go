package middleware

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/helper"
)

// cancellationLogFixture captures a real abort log and its unchanged wire response.
type cancellationLogFixture struct {
	response *httptest.ResponseRecorder
	event    map[string]any
	aborted  bool
}

// captureCancellationAbort runs either shared abort entry point with lifecycle provenance.
func captureCancellationAbort(t *testing.T, root context.Context, lifecycle bool, token bool, status int, err error, before gin.HandlerFunc) cancellationLogFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "abort.json")
	logger, loggerErr := glog.New(glog.WithName("request-lifecycle-test"), glog.WithLevel(glog.LevelInfo), glog.WithEncoding(glog.EncodingJSON), glog.WithOutputPaths([]string{path}), glog.WithErrorOutputPaths([]string{path}))
	require.NoError(t, loggerErr)
	engine := gin.New()
	if lifecycle {
		engine.Use(RequestLifecycle())
	}
	engine.Use(func(c *gin.Context) {
		gmw.SetLogger(c, logger)
		c.Set(helper.RequestIdKey, "synthetic-cancel-request")
		c.Next()
	})
	if before != nil {
		engine.Use(before)
	}
	aborted := false
	engine.GET("/synthetic", func(c *gin.Context) {
		if token {
			AbortWithTokenError(c, status, err, &TokenInfo{MaskedKey: "synthetic-masked", RequestedAt: "synthetic-model"})
		} else {
			AbortWithError(c, status, err)
		}
		aborted = c.IsAborted()
	})
	request := httptest.NewRequest(http.MethodGet, "/synthetic", nil).WithContext(root)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	require.NoError(t, logger.Sync())
	raw, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	var event map[string]any
	require.NoError(t, json.Unmarshal(raw, &event), "exactly one structured abort event is required")
	return cancellationLogFixture{response: response, event: event, aborted: aborted}
}

// TestRequestCancellationAbortClassification guards both abort funnels and wire compatibility.
func TestRequestCancellationAbortClassification(t *testing.T) {
	gin.SetMode(gin.TestMode)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	custom, cancelCustom := context.WithCancelCause(context.Background())
	cancelCustom(errors.New("synthetic internal shutdown"))
	deadline, cancelDeadline := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelDeadline()
	operational := errors.New("synthetic Redis operational failure")
	for _, tc := range []struct {
		name        string
		root        context.Context
		marker      bool
		status      int
		err         error
		level, kind string
	}{
		{"transport_cancel", canceled, true, 500, context.Canceled, "WARN", "request_canceled"},
		{"wrapped_transport_cancel", canceled, true, 500, errors.Wrap(context.Canceled, "failed to get list length"), "WARN", "request_canceled"},
		{"pure_join_transport_cancel", canceled, true, 500, stderrors.Join(context.Canceled, errors.WithStack(context.Canceled)), "WARN", "request_canceled"},
		{"active_request_canceled_result", context.Background(), true, 500, context.Canceled, "ERROR", "unknown"},
		{"active_request_canceled_result_401", context.Background(), true, 401, context.Canceled, "ERROR", "unknown"},
		{"missing_origin", canceled, false, 500, context.Canceled, "ERROR", "unknown"},
		{"custom_root_cause", custom, true, 500, context.Canceled, "ERROR", "unknown"},
		{"root_deadline", deadline, true, 500, context.DeadlineExceeded, "ERROR", "unknown"},
		{"deadline_401", context.Background(), true, 401, context.DeadlineExceeded, "ERROR", "unknown"},
		{"downstream_close_during_deadline", canceled, true, 500, context.DeadlineExceeded, "ERROR", "unknown"},
		{"unrelated_failure_after_close", canceled, true, 500, operational, "ERROR", "unknown"},
		{"joined_operational_failure", canceled, true, 500, stderrors.Join(context.Canceled, operational), "ERROR", "unknown"},
		{"joined_deadline", canceled, true, 500, stderrors.Join(context.Canceled, context.DeadlineExceeded), "ERROR", "unknown"},
		{"marked_server_cancel", canceled, true, 500, errkind.ServerErr(errors.WithStack(context.Canceled)), "ERROR", "server"},
		{"marked_upstream_cancel", canceled, true, 500, errkind.UpstreamErr(errors.WithStack(context.Canceled)), "ERROR", "upstream"},
		{"joined_client_then_server", canceled, true, 500, stderrors.Join(errkind.UnauthorizedErr(context.Canceled), errkind.ServerErr(operational)), "ERROR", "server"},
		{"joined_server_then_client", canceled, true, 500, stderrors.Join(errkind.ServerErr(operational), errkind.UnauthorizedErr(context.Canceled)), "ERROR", "server"},
		{"joined_client_then_upstream", canceled, true, 500, stderrors.Join(errkind.UnauthorizedErr(context.Canceled), errkind.UpstreamErr(operational)), "ERROR", "upstream"},
		{"joined_client_then_unmarked_fault", canceled, true, 500, stderrors.Join(errkind.UnauthorizedErr(context.Canceled), operational), "ERROR", "unknown"},
		{"joined_unmarked_fault_then_client", canceled, true, 500, stderrors.Join(operational, errkind.UnauthorizedErr(context.Canceled)), "ERROR", "unknown"},
		{"deliberate_outer_client_override", canceled, true, 500, errkind.UnauthorizedErr(errkind.ServerErr(context.Canceled)), "WARN", "unauthorized"},
		{"deliberate_outer_join_override", canceled, true, 500, errkind.UnauthorizedErr(stderrors.Join(context.Canceled, errkind.ServerErr(operational))), "WARN", "unauthorized"},
		{"existing_client_attribution", canceled, true, 401, errkind.UnauthorizedErr(errors.WithStack(context.Canceled)), "WARN", "unauthorized"},
		{"text_only_false_positive", canceled, true, 500, errors.New("context canceled"), "ERROR", "unknown"},
		{"ordinary_401_compatibility", context.Background(), true, 401, errors.New("synthetic invalid credential"), "WARN", "unknown"},
	} {
		for _, token := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/token=%t", tc.name, token), func(t *testing.T) {
				got := captureCancellationAbort(t, tc.root, tc.marker, token, tc.status, tc.err, nil)
				require.True(t, got.aborted)
				require.Equal(t, tc.status, got.response.Code)
				require.Equal(t, "application/json; charset=utf-8", got.response.Header().Get("Content-Type"))
				expected := map[string]any{"error": map[string]any{"message": tc.err.Error() + " (request id: synthetic-cancel-request)", "type": "one_api_error"}}
				raw, err := json.Marshal(expected)
				require.NoError(t, err)
				require.JSONEq(t, string(raw), got.response.Body.String(), "cancellation classification must preserve legacy wire behavior")
				require.Equal(t, tc.level, got.event["level"])
				require.Equal(t, tc.kind, got.event["error_kind"])
				require.Equal(t, "server abort", got.event["message"])
				require.EqualValues(t, tc.status, got.event["status_code"])
				require.Equal(t, tc.err.Error(), got.event["error"])
				if tc.level == "WARN" {
					require.NotContains(t, got.event, "errorVerbose", "nonpaging outcomes must not carry a stack")
				}
				if token {
					require.Equal(t, "synthetic-masked", got.event["api_key"])
					require.Equal(t, "synthetic-model", got.event["requested_model"])
				}
			})
		}
	}
}

// TestRequestLifecycleRetainsTransportOrigin rejects internally canceled replacement contexts.
func TestRequestLifecycleRetainsTransportOrigin(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(fmt.Sprintf("duplicate=%t", duplicate), func(t *testing.T) {
			before := func(c *gin.Context) {
				child, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(child)
				if duplicate {
					RequestLifecycle()(c)
				} else {
					c.Next()
				}
			}
			got := captureCancellationAbort(t, context.Background(), true, false, 500, context.Canceled, before)
			require.Equal(t, "ERROR", got.event["level"])
			require.Equal(t, "unknown", got.event["error_kind"])
		})
	}
}

// cyclicAbortError models a malformed dependency chain without recursive rendering.
type cyclicAbortError struct{}

// Error returns a fixed synthetic diagnostic for the malformed chain.
func (*cyclicAbortError) Error() string { return "synthetic cyclic dependency error" }

// Unwrap returns itself to exercise the classifier's traversal bound.
func (e *cyclicAbortError) Unwrap() error { return e }

// TestRequestCancellationProofFailsClosed keeps malformed trees and provenance actionable.
func TestRequestCancellationProofFailsClosed(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/synthetic", nil).WithContext(canceled)
	RequestLifecycle()(c)
	wide := make([]error, 257)
	for i := range wide {
		wide[i] = context.Canceled
	}
	deep := error(context.Canceled)
	for range 65 {
		deep = fmt.Errorf("synthetic dependency: %w", deep)
	}
	for _, err := range []error{&cyclicAbortError{}, stderrors.Join(wide...), deep} {
		plan := classifyAbortLog(c, 401, err)
		require.False(t, plan.warning)
		require.Equal(t, "unknown", plan.kind)
	}
	c.Set(requestLifecycleContextKey, context.Background())
	require.False(t, classifyAbortLog(c, 500, context.Canceled).warning, "an untyped/corrupt marker cannot prove transport origin")
}
