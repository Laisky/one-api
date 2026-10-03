package relayctx

import (
	"context"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestDetach_DoesNotRetainGinContext is the load-bearing invariant: a goroutine
// holding the detached context must not be able to reach the recycled *gin.Context.
// gmw.BackgroundCtx fails this (it stores c under CtxKeyGin); Detach must pass it.
func TestDetach_DoesNotRetainGinContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/anything", nil)

	detached := Detach(c)

	_, ok := gmw.GetGinCtxFromStdCtx(detached)
	require.False(t, ok,
		"Detach must not retain *gin.Context; otherwise a background goroutine can "+
			"read c.Keys after gin recycles the context via sync.Pool")

	// Sanity: gmw.BackgroundCtx, the unsafe alternative, DOES retain c — proving the
	// distinction this primitive exists to make.
	_, bgOK := gmw.GetGinCtxFromStdCtx(gmw.BackgroundCtx(c))
	require.True(t, bgOK, "gmw.BackgroundCtx is expected to retain c (the hazard Detach avoids)")
}

// TestDetach_SnapshotsLogger verifies the request logger is carried by value, so
// gmw.GetLogger(detached) returns the request logger without touching c.
func TestDetach_SnapshotsLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/anything", nil)

	marker := glog.Shared.Named("relayctx-test-marker")
	gmw.SetLogger(c, marker)

	detached := Detach(c)
	require.Equal(t, gmw.GetLogger(c), gmw.GetLogger(detached),
		"Detach must snapshot the request logger by value")
}

// TestDetach_SurvivesRequestCancellation verifies the detached context is not
// cancelled when the request context is — the property that prevents refund DB
// writes from being aborted after the handler returns.
func TestDetach_SurvivesRequestCancellation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest("POST", "/v1/anything", nil)
	reqCtx, cancel := context.WithCancel(context.Background())
	c.Request = req.WithContext(reqCtx)

	detached := Detach(c)
	require.NoError(t, detached.Err(), "detached context should start live")

	cancel() // simulate the handler returning / client disconnecting
	require.Error(t, gmw.Ctx(c).Err(), "request context must be cancelled")
	require.NoError(t, detached.Err(),
		"Detach must survive request-context cancellation so post-return DB writes are not aborted")
}

// TestDetach_CarriesTraceID verifies the trace id is snapshotted under the key
// gin-middlewares uses, so downstream logging/tracing can still resolve it.
func TestDetach_CarriesTraceID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/anything", nil)

	tid, err := gmw.TraceID(c)
	require.NoError(t, err)
	require.NotEmpty(t, tid.String())

	detached := Detach(c)
	require.Equal(t, tid.String(), detached.Value(gutils.TracingKey),
		"Detach must carry the request trace id by value")
}

// TestDetach_NilContext must not panic and returns a usable background context.
func TestDetach_NilContext(t *testing.T) {
	detached := Detach(nil)
	require.NotNil(t, detached)
	require.NoError(t, detached.Err())
}
