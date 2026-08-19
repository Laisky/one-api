package controller

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/common/relayctx"
	"github.com/Laisky/one-api/relay/model"
)

// TestGoProcessChannelRelayError_RunsOnDetachedContext is the reproduce-first guard for
// the async channel-error path fix (relay.go: `ctx := relayctx.Detach(c)` instead of
// `ctx := gmw.Ctx(c)`).
//
// Relay used to alias `ctx := gmw.Ctx(c)` and pass it through goProcessChannelRelayError
// into graceful.GoCritical(ctx, ...), reaching dbmodel.SuspendAbility / monitor.Emit. That
// context (a) embeds the *gin.Context — gin recycles it via sync.Pool the instant the
// handler returns, so the error goroutine racing a recycled c could suspend the WRONG
// channel — and (b) inherits request cancellation, so a client disconnect could abort the
// SuspendAbility DB write. The ast-grep guardrail could not catch this aliased form
// (data-flow), so this behavioral test is the regression guard.
//
// The test mirrors what Relay now does (detach on the request goroutine, hand the detached
// ctx to the async processor), parks the spawned goroutine on a gate, cancels the request
// context, then releases and asserts the goroutine observed a context that is NEITHER
// cancelled NOR carrying the gin context. With the old gmw.Ctx(c) the observed ctx.Err()
// would be context.Canceled and gmw.GetGinCtxFromStdCtx would succeed; with
// relayctx.Detach(c) both are false. The assertion is on the observed context
// (deterministic), not on the -race detector firing.
func TestGoProcessChannelRelayError_RunsOnDetachedContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gate := make(chan struct{})
	processChannelRelayErrorGateForTest = gate
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(gate) }) }

	type observation struct {
		ctxErr   error
		carriesC bool
	}
	observed := make(chan observation, 1)
	processChannelRelayErrorForTest = func(ctx context.Context, _ processChannelRelayErrorParams) {
		_, carriesC := gmw.GetGinCtxFromStdCtx(ctx)
		observed <- observation{ctxErr: ctx.Err(), carriesC: carriesC}
	}

	t.Cleanup(func() {
		release()
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = graceful.Drain(drainCtx)
		processChannelRelayErrorGateForTest = nil
		processChannelRelayErrorForTest = nil
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	reqCtx, cancelReq := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil).WithContext(reqCtx)

	// Mirror Relay: detach on the request goroutine, then hand the detached context to the
	// async error processor. All params are already snapshotted by value.
	detached := relayctx.Detach(c)
	bizErr := &model.ErrorWithStatusCode{
		Error:      model.Error{Message: "upstream 429"},
		StatusCode: 429,
	}
	goProcessChannelRelayError(detached, bizErr, processChannelRelayErrorParams{
		RequestID: "req_detach_errproc",
		ChannelId: 7,
	})

	// Simulate the handler returning / client disconnecting while the error goroutine is
	// parked: the request context is cancelled and gin would recycle c.
	cancelReq()

	release()

	select {
	case got := <-observed:
		require.NoError(t, got.ctxErr,
			"channel-error processing must run on a NON-CANCELLED (detached) context so a "+
				"client disconnect cannot abort SuspendAbility; a cancelled context here means "+
				"the path regressed to gmw.Ctx(c)/the request context")
		require.False(t, got.carriesC,
			"channel-error processing must run on a c-free context (relayctx.Detach); carrying "+
				"the *gin.Context lets the goroutine race gin's sync.Pool recycle and suspend the wrong channel")
	case <-time.After(5 * time.Second):
		t.Fatal("async processChannelRelayError did not run")
	}
}
