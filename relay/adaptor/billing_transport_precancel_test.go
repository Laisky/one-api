package adaptor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/meta"
)

// billingPrecancelBody observes the actual outgoing body, including a secondary cleanup failure.
type billingPrecancelBody struct {
	reader        io.Reader
	reads, closes int
	closeErr      error
}

// Read counts payload access and delegates to the synthetic reader.
func (b *billingPrecancelBody) Read(p []byte) (int, error) {
	b.reads++
	return b.reader.Read(p)
}

// Close records ownership cleanup and returns the configured fixture failure.
func (b *billingPrecancelBody) Close() error {
	b.closes++
	return b.closeErr
}

// billingPrecancelOpaque deliberately ignores request cancellation; the helper must prevent invoking it.
type billingPrecancelOpaque struct {
	calls int
	err   error
}

// RoundTrip ignores the context, closes any dispatched body and returns its synthetic failure.
func (r *billingPrecancelOpaque) RoundTrip(req *http.Request) (*http.Response, error) {
	r.calls++
	if req.Body != nil {
		if err := req.Body.Close(); err != nil {
			return nil, errors.Wrap(err, "close dispatched fixture body")
		}
	}
	return nil, errors.WithStack(r.err)
}

// billingPrecancelContext creates a logger-backed request using the existing stub adaptor and trace fixture.
func billingPrecancelContext(t *testing.T, canceled, forwarded bool) (*gin.Context, *observer.ObservedLogs) {
	t.Helper()
	withTraceDB(t)
	core, logs := observer.New(zapcore.DebugLevel)
	logger, err := glog.NewWithName("billing-precancel-helper", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	gmw.SetLogger(c, logger)
	if canceled {
		ctx, cancel := context.WithCancel(c.Request.Context())
		cancel()
		c.Request = c.Request.WithContext(ctx)
	}
	c.Set(ctxkey.ContentType, "application/json")
	if forwarded {
		c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
	}
	return c, logs
}

// TestBillingTransportHelperPrecancelNeverDispatches proves cancellation prevents an opaque transport invocation and closes only owned outgoing bodies.
func TestBillingTransportHelperPrecancelNeverDispatches(t *testing.T) {
	for _, forwarded := range []bool{false, true} {
		for _, kind := range []string{"body", "close_error", "nil_body"} {
			name := kind + "/initially_unforwarded"
			if forwarded {
				name = kind + "/already_forwarded"
			}
			t.Run(name, func(t *testing.T) {
				c, logs := billingPrecancelContext(t, true, forwarded)
				transport := &billingPrecancelOpaque{err: errors.New("synthetic opaque failure")}
				previous := client.HTTPClient
				client.HTTPClient = &http.Client{Transport: transport}
				t.Cleanup(func() { client.HTTPClient = previous })
				var body io.Reader
				var owned *billingPrecancelBody
				var closeErr error
				if kind != "nil_body" {
					if kind == "close_error" {
						closeErr = errors.New("synthetic body cleanup failure")
					}
					owned = &billingPrecancelBody{reader: strings.NewReader(`{"fixture":true}`), closeErr: closeErr}
					body = owned
				}
				resp, err := DoRequestHelper(&stubAdaptor{defaultURL: "http://billing-precancel.invalid/v1/chat/completions"}, c,
					&meta.Meta{ChannelId: 95003, ActualModelName: "fixture-model"}, body)
				require.Nil(t, resp)
				require.ErrorIs(t, err, context.Canceled)
				require.Zero(t, transport.calls, "an opaque transport may ignore context once called")
				require.Equal(t, forwarded, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded), "never clear proof of an earlier possibly paid write")
				require.Empty(t, logs.FilterMessage("forwarding request to upstream channel").All())
				if owned != nil {
					require.Zero(t, owned.reads)
					require.Equal(t, 1, owned.closes)
				}
				if closeErr != nil {
					require.ErrorIs(t, err, closeErr, "cleanup failure must not replace or disappear behind cancellation")
				}
			})
		}
	}
}

// TestBillingTransportHelperOpaqueErrorRemainsForwarded preserves conservative attribution for an uncanceled custom transport failure.
func TestBillingTransportHelperOpaqueErrorRemainsForwarded(t *testing.T) {
	c, logs := billingPrecancelContext(t, false, false)
	cause := errors.New("synthetic opaque transport failure")
	transport := &billingPrecancelOpaque{err: cause}
	previous := client.HTTPClient
	client.HTTPClient = &http.Client{Transport: transport}
	t.Cleanup(func() { client.HTTPClient = previous })
	body := &billingPrecancelBody{reader: strings.NewReader(`{"fixture":true}`)}
	resp, err := DoRequestHelper(&stubAdaptor{defaultURL: "http://billing-opaque.invalid/v1/chat/completions"}, c,
		&meta.Meta{ChannelId: 95003, ActualModelName: "fixture-model"}, body)
	require.Nil(t, resp)
	require.ErrorIs(t, err, cause)
	require.Equal(t, 1, transport.calls)
	require.Equal(t, 1, body.closes)
	require.True(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
	require.Len(t, logs.FilterMessage("forwarding request to upstream channel").All(), 1)
}
