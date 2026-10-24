package openai_compatible

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	relayadaptor "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/model"
)

// newIssue599NativeResponse keeps the provider connection open after all
// fixture bytes have been delivered, so cancellation happens at a real read.
func newIssue599NativeResponse(t *testing.T, wire string) (*gin.Context, *httptest.ResponseRecorder, *http.Response, context.CancelFunc) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, wire)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	c, rec := newBridgeTestCtx(t)
	gmw.SetLogger(c, createTestLogger())
	ctx, cancel := context.WithCancel(c.Request.Context())
	t.Cleanup(cancel)
	c.Request = c.Request.WithContext(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	resp, err := relayadaptor.DoRequest(c, req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.True(t, relayadaptor.ResponseTransportOwnsCancellation(c, resp))
	return c, rec, resp, cancel
}

// A response pointer surviving dispatch is not sufficient to establish that
// the currently installed reader is still canceled by net/http.Transport.
func TestIssue599OpaqueReplacementRetainsCancelWatcher(t *testing.T) {
	c, _, resp, cancel := newIssue599NativeResponse(t, ": ready\n\n")
	original := resp.Body
	defer original.Close()

	body := &issue599IndependentFailure{
		reader:   strings.NewReader(issue599Receipt + issue599DONE),
		failure:  net.ErrClosed,
		entered:  make(chan struct{}),
		closed:   make(chan struct{}),
		released: make(chan struct{}),
	}
	resp.Body = body
	type result struct {
		err   *model.ErrorWithStatusCode
		usage *model.Usage
	}
	finished := make(chan result, 1)
	go func() {
		failure, usage := UnifiedStreamProcessing(c, resp, 11, "gpt-4", false)
		finished <- result{failure, usage}
	}()

	select {
	case <-body.entered:
	case <-time.After(3 * time.Second):
		cancel()
		close(body.released)
		t.Fatal("replacement body did not enter a blocked read")
	}
	cancel()
	select {
	case <-body.closed:
	case <-time.After(500 * time.Millisecond):
		close(body.released)
		<-finished // unblock a faulty implementation before failing
		t.Fatal("native response pointer incorrectly disabled the replacement body's cancellation watcher")
	}
	close(body.released)
	select {
	case got := <-finished:
		require.NotNil(t, got.err, "an independent closed-connection read must remain an error")
		require.ErrorIs(t, got.err.RawError, net.ErrClosed)
		require.NotNil(t, got.usage)
		require.Equal(t, 18, got.usage.TotalTokens)
	case <-time.After(3 * time.Second):
		t.Fatal("replacement body did not finish after cancellation")
	}
	require.False(t, relayadaptor.ResponseTransportOwnsCancellation(c, resp))
	require.Equal(t, int32(1), body.closeCount.Load())
}

// Rebinding the handler to a different request context after dispatch must
// re-enable the ordinary watcher: the original transport has no way to
// observe cancellation of the replacement context.
func TestIssue599ReboundContextRetainsCancelWatcher(t *testing.T) {
	wire := issue599Receipt + issue599DONE
	c, _, resp, cancelOriginal := newIssue599NativeResponse(t, wire)
	rebound, cancelRebound := context.WithCancel(context.Background())
	defer cancelRebound()
	c.Request = c.Request.WithContext(rebound)

	boundary := &issue599Boundary{
		ReadCloser: resp.Body,
		expected:   []byte(wire),
		entered:    make(chan struct{}),
	}
	resp.Body = boundary
	finished := make(chan *model.ErrorWithStatusCode, 1)
	go func() {
		failure, _ := UnifiedStreamProcessing(c, resp, 11, "gpt-4", false)
		finished <- failure
	}()
	select {
	case <-boundary.entered:
	case <-time.After(3 * time.Second):
		cancelOriginal()
		t.Fatal("did not reach the post-DONE network read")
	}
	cancelRebound()
	select {
	case failure := <-finished:
		require.NotNil(t, failure, "rebound cancellation cannot be attributed to the original native request")
	case <-time.After(500 * time.Millisecond):
		cancelOriginal()
		<-finished
		t.Fatal("rebound request cancellation left the native transport reader blocked")
	}
	require.False(t, relayadaptor.ResponseTransportOwnsCancellation(c, resp))
	require.Equal(t, int32(1), boundary.closes.Load())
}

// A malformed prefix of [DONE] must not authorize successful completion after
// a previously complete usage receipt. Whitespace-only suffixes remain valid.
func TestIssue599MalformedDoneCannotAuthorizeCancellationSuccess(t *testing.T) {
	for _, tc := range []struct {
		name         string
		marker       string
		wantComplete bool
	}{
		{"suffix", "[DONE]garbage", false},
		{"extra_token", "[DONE] [DONE]", false},
		{"whitespace", "[DONE] \t", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := issue599Receipt + "data: " + tc.marker + "\n\n"
			c, rec, resp, cancel := newIssue599NativeResponse(t, wire)
			boundary := &issue599Boundary{
				ReadCloser: resp.Body,
				expected:   []byte(wire),
				entered:    make(chan struct{}),
			}
			resp.Body = boundary
			finished := make(chan *model.ErrorWithStatusCode, 1)
			go func() {
				failure, _ := UnifiedStreamProcessing(c, resp, 11, "gpt-4", false)
				finished <- failure
			}()
			select {
			case <-boundary.entered:
			case <-time.After(3 * time.Second):
				cancel()
				t.Fatal("did not reach the post-frame read")
			}
			cancel()
			select {
			case failure := <-finished:
				if tc.wantComplete {
					require.Nil(t, failure)
					require.Contains(t, rec.Body.String(), "data: [DONE]")
				} else {
					require.NotNil(t, failure, "malformed DONE must not turn a canceled stream into success")
					require.NotContains(t, rec.Body.String(), "data: [DONE]")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("cancellation failed to release the HTTP read")
			}
		})
	}
}
