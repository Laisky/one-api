package openai_compatible

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	relayadaptor "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/streaming"
)

// joinedCauseBoundary observes a native body's post-fixture read and original
// error. Read and Close delegate unchanged; this wrapper injects no error.
type joinedCauseBoundary struct {
	io.ReadCloser
	expected []byte
	seen     []byte
	entered  chan struct{}
	readErr  chan error
	once     sync.Once
	closes   atomic.Int32
}

// Read delegates to the native body and records its unmodified read error.
func (b *joinedCauseBoundary) Read(p []byte) (int, error) {
	if bytes.Equal(b.seen, b.expected) {
		b.once.Do(func() { close(b.entered) })
	}
	n, err := b.ReadCloser.Read(p)
	b.seen = append(b.seen, p[:n]...)
	if err != nil {
		select {
		case b.readErr <- err:
		default:
		}
	}
	return n, err
}

// Close delegates to the native body while counting cleanup invocations.
func (b *joinedCauseBoundary) Close() error {
	b.closes.Add(1)
	return b.ReadCloser.Close()
}

// Unwrap exposes the unchanged native reader for dispatch ownership verification.
func (b *joinedCauseBoundary) Unwrap() io.ReadCloser { return b.ReadCloser }

// TestIssue599JoinedCancellationCausePreservesIndependentErrors dispatches a real
// native HTTP request and cancels it with a cause containing cancellation plus
// independent failures. net/http propagates context.Cause; the wrapper only
// observes it, so no replacement-body or forged-ownership seam is involved.
func TestIssue599JoinedCancellationCausePreservesIndependentErrors(t *testing.T) {
	independent := errors.New("independent stream failure")
	for _, tc := range []struct {
		name        string
		cause       error
		independent []error
	}{
		{
			name:        "closed_connection_control",
			cause:       errors.Join(context.Canceled, net.ErrClosed),
			independent: []error{net.ErrClosed},
		},
		{
			name:        "unexpected_eof_control",
			cause:       errors.Join(context.Canceled, io.ErrUnexpectedEOF),
			independent: []error{io.ErrUnexpectedEOF},
		},
		{
			name:        "eof_joined_with_unexpected_eof",
			cause:       errors.Join(context.Canceled, io.EOF, io.ErrUnexpectedEOF),
			independent: []error{io.ErrUnexpectedEOF},
		},
		{
			name:        "eof_joined_with_short_buffer",
			cause:       errors.Join(context.Canceled, io.EOF, io.ErrShortBuffer),
			independent: []error{io.ErrShortBuffer},
		},
		{
			name: "eof_joined_with_arbitrary_failure",
			cause: errors.Wrap(errors.Join(
				context.Canceled,
				errors.Wrap(errors.Join(io.EOF, independent), "mixed EOF branch"),
			), "outer EOF cause"),
			independent: []error{independent},
		},
		{
			name:        "short_buffer",
			cause:       errors.Join(context.Canceled, io.ErrShortBuffer),
			independent: []error{io.ErrShortBuffer},
		},
		{
			name:        "short_write",
			cause:       errors.Join(context.Canceled, io.ErrShortWrite),
			independent: []error{io.ErrShortWrite},
		},
		{
			name:        "arbitrary_failure",
			cause:       errors.Join(context.Canceled, independent),
			independent: []error{independent},
		},
		{
			name: "nested_multiple_failures",
			cause: errors.Wrap(errors.Join(
				errors.Wrap(context.Canceled, "cancellation branch"),
				errors.Wrap(errors.Join(io.ErrShortBuffer, independent), "independent branches"),
			), "outer transport cause"),
			independent: []error{io.ErrShortBuffer, independent},
		},
		{
			name: "duplicate_cancellation_with_nested_failure",
			cause: errors.Join(
				context.Canceled,
				errors.Wrap(errors.Join(context.Canceled, independent), "nested cause"),
			),
			independent: []error{independent},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runIssue599JoinedCause(t, tc.cause, tc.independent)
		})
	}
}

// TestIssue599PureCancellationCauseStillCompletes retains successful terminal
// behavior for bare, wrapped, and duplicated cancellation-only error trees.
func TestIssue599PureCancellationCauseStillCompletes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause error
	}{
		{"default_cancellation", nil},
		{"bare_cancellation", context.Canceled},
		{"wrapped_cancellation", errors.Wrap(context.Canceled, "native cancellation cause")},
		{"duplicate_cancellation", errors.Join(context.Canceled, context.Canceled)},
		{
			"nested_duplicate_cancellation",
			errors.Wrap(errors.Join(
				errors.Wrap(context.Canceled, "first cancellation"),
				errors.Wrap(errors.Join(context.Canceled, context.Canceled), "other cancellations"),
			), "outer cancellation"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runIssue599JoinedCause(t, tc.cause, nil)
		})
	}
}

// runIssue599JoinedCause returns a real transport error after measured receipt
// and DONE, then checks handler behavior, unchanged causes, usage and one cleanup.
func runIssue599JoinedCause(t *testing.T, cause error, independent []error) {
	t.Helper()
	configureIssue599TraceFixture(t)
	wire := issue599Receipt + issue599DONE
	var requests atomic.Int32
	releaseServer := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, wire)
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-releaseServer:
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(releaseServer) })

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	oldClient := client.HTTPClient
	client.HTTPClient = &http.Client{Transport: transport}
	t.Cleanup(func() {
		client.HTTPClient = oldClient
		transport.CloseIdleConnections()
	})

	c, rec := newBridgeTestCtx(t)
	gmw.SetLogger(c, createTestLogger())
	timeoutCtx, stopTimeout := context.WithTimeout(c.Request.Context(), 5*time.Second)
	t.Cleanup(stopTimeout)
	ctx, cancel := context.WithCancelCause(timeoutCtx)
	t.Cleanup(func() { cancel(context.Canceled) })
	c.Request = c.Request.WithContext(ctx)
	c.Set(ctxkey.RequestModel, "gpt-4")
	streaming.StoreTracker(c, streaming.NewQuotaTracker(streaming.QuotaTrackerParams{
		Ctx: ctx, ModelName: "gpt-4", PromptTokens: 11, PreConsumedQuota: 10000,
		TokenCounter: func(s, _ string) int { return len(s) },
	}))

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	resp, err := relayadaptor.DoRequest(c, request)
	require.NoError(t, err)
	require.True(t, relayadaptor.ResponseTransportOwnsCancellation(c, resp))
	boundary := &joinedCauseBoundary{
		ReadCloser: resp.Body, expected: []byte(wire),
		entered: make(chan struct{}), readErr: make(chan error, 1),
	}
	resp.Body = boundary
	require.True(t, relayadaptor.ResponseTransportOwnsCancellation(c, resp),
		"the recording wrapper delegates native Read and Close unchanged")

	type result struct {
		failure *model.ErrorWithStatusCode
		usage   *model.Usage
	}
	finished := make(chan result, 1)
	go func() {
		failure, usage := UnifiedStreamProcessing(c, resp, 11, "gpt-4", false)
		finished <- result{failure, usage}
	}()
	select {
	case <-boundary.entered:
	case <-time.After(3 * time.Second):
		cancel(context.Canceled)
		t.Fatal("did not reach the native read after receipt and DONE")
	}
	cancel(cause)
	var got result
	select {
	case got = <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not release the native HTTP body read")
	}

	require.ErrorIs(t, ctx.Err(), context.Canceled)
	var nativeErr error
	select {
	case nativeErr = <-boundary.readErr:
	default:
		t.Fatal("the native body did not expose its original cancellation error")
	}
	require.ErrorIs(t, nativeErr, context.Canceled)
	require.Equal(t, int32(1), requests.Load(), "accepted work must never be retried")
	require.Equal(t, int32(1), boundary.closes.Load())
	require.Equal(t, 1, strings.Count(rec.Body.String(), "data: [DONE]"))
	require.NotNil(t, got.usage)
	require.Equal(t, 11, got.usage.PromptTokens)
	require.Equal(t, 7, got.usage.CompletionTokens)
	require.Equal(t, 18, got.usage.TotalTokens)
	require.Empty(t, got.usage.BillingEstimateReason)

	for _, failure := range independent {
		require.ErrorIs(t, nativeErr, failure,
			"the real transport must expose the independent branch before handler assertions")
	}
	if len(independent) == 0 {
		require.Nil(t, got.failure, "a cancellation-only error tree after receipt and DONE may complete")
		return
	}
	require.NotNil(t, got.failure, "an independent joined cause must not become terminal success")
	require.Equal(t, http.StatusInternalServerError, got.failure.StatusCode)
	require.ErrorIs(t, got.failure.RawError, context.Canceled)
	for _, failure := range independent {
		require.ErrorIs(t, got.failure.RawError, failure,
			"all original independent causes must remain observable")
	}
}
