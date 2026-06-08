package controller

import (
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// TestDetachForBilling_SnapshotsIdentifiers reproduces the indirect use-after-return
// bug in the post-billing path: postConsume* used to resolve request id / provisional
// log id / tool summary via gmw.GetGinCtxFromStdCtx(ctx) INSIDE the billing goroutine,
// which reads c.Keys after gin recycles the *gin.Context via sync.Pool. The migration
// snapshots those identifiers on the request goroutine (detachForBilling) so the
// goroutine reads them from the snapshot, immune to c being recycled/mutated.
//
// The test snapshots c, then mutates every captured key (simulating the next request's
// c.reset()/Set after recycling) and asserts billingIdentityFromContext still returns
// the spawn-time values. Against the buggy gmw.BackgroundCtx(c) path, the embedded c
// would be resolved by the fallback and return the mutated values; with the snapshot it
// returns the spawn-time values.
func TestDetachForBilling_SnapshotsIdentifiers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	summary := &model.ToolUsageSummary{}
	c.Set(ctxkey.RequestId, "req_spawn_time")
	c.Set(ctxkey.ProvisionalLogId, 1234)
	c.Set(ctxkey.ToolInvocationSummary, summary)

	ctx := detachForBilling(c)

	// The detached context must not retain the gin context.
	_, ok := gmw.GetGinCtxFromStdCtx(ctx)
	require.False(t, ok, "detachForBilling must not retain *gin.Context")

	// Simulate gin recycling c for the next request and mutating its keys.
	c.Set(ctxkey.RequestId, "req_recycled")
	c.Set(ctxkey.ProvisionalLogId, 9999)
	c.Set(ctxkey.ToolInvocationSummary, &model.ToolUsageSummary{})

	id := billingIdentityFromContext(ctx)
	require.Equal(t, "req_spawn_time", id.requestID,
		"request id must come from the spawn-time snapshot, not the recycled *gin.Context")
	require.Equal(t, 1234, id.provisionalLogID,
		"provisional log id must come from the spawn-time snapshot")
	require.Same(t, summary, id.toolSummary,
		"tool summary must be the spawn-time pointer, not the mutated one")
	require.NotEmpty(t, id.traceID, "trace id must be snapshotted")
}

// TestBillingIdentityFromContext_SyncFallback verifies that a synchronous caller that
// still passes an embedded *gin.Context (gmw.Ctx/BackgroundCtx) — i.e. NOT a
// detachForBilling context — resolves identifiers off the gin context. This keeps the
// legacy synchronous path working while the goroutine path uses the snapshot.
func TestBillingIdentityFromContext_SyncFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set(ctxkey.RequestId, "req_sync")
	c.Set(ctxkey.ProvisionalLogId, 77)

	// gmw.BackgroundCtx embeds c (the legacy path) and carries no billing snapshot, so
	// billingIdentityFromContext falls back to reading the gin context.
	id := billingIdentityFromContext(gmw.BackgroundCtx(c))
	require.Equal(t, "req_sync", id.requestID)
	require.Equal(t, 77, id.provisionalLogID)
}

// TestBillingIdentityFromGinContext_Resolves verifies the explicit SYNCHRONOUS resolver
// extracts the billing identifiers directly off the request *gin.Context, and that the
// detachForBilling snapshot agrees with it (detachForBilling is defined in terms of it).
func TestBillingIdentityFromGinContext_Resolves(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	summary := &model.ToolUsageSummary{}
	c.Set(ctxkey.RequestId, "req_explicit")
	c.Set(ctxkey.ProvisionalLogId, 555)
	c.Set(ctxkey.ToolInvocationSummary, summary)

	id := billingIdentityFromGinContext(c)
	require.Equal(t, "req_explicit", id.requestID)
	require.Equal(t, 555, id.provisionalLogID)
	require.Same(t, summary, id.toolSummary)
	require.NotEmpty(t, id.traceID)

	// The async snapshot path (detachForBilling) must produce the same identity as the
	// explicit synchronous resolver — they share billingIdentityFromGinContext.
	snap := billingIdentityFromContext(detachForBilling(c))
	require.Equal(t, id, snap, "detachForBilling snapshot must match the explicit gin resolver")
}

// TestBillingIdentityFromGinContext_NilSafe: a nil gin context yields a zero identity and
// never panics (mirrors detachForBilling's nil handling).
func TestBillingIdentityFromGinContext_NilSafe(t *testing.T) {
	require.Equal(t, billingIdentity{}, billingIdentityFromGinContext(nil))
}
