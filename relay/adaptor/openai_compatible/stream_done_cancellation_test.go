package openai_compatible

import (
	"bytes"
	"context"
	"fmt"
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
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/render"
	relayadaptor "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/streaming"
)

// doneCancellationWriter cancels at the actual rendered protocol boundary.
type doneCancellationWriter struct {
	gin.ResponseWriter
	cancel context.CancelFunc
}

// Write cancels only after a complete, successfully written terminal SSE frame.
// Literal [DONE] within generated content must not end the request.
func (w *doneCancellationWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if err == nil && n == len(p) && strings.TrimSpace(string(p)) == "data: [DONE]" {
		w.cancel()
	}
	return n, err
}

// WriteString follows the same byte-oriented renderer contract.
func (w *doneCancellationWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// issue599ShortWriteWriter simulates a terminal event not fully delivered.
type issue599ShortWriteWriter struct{ gin.ResponseWriter }

func (w *issue599ShortWriteWriter) Write(p []byte) (int, error) {
	return len(p) - 1, nil
}

// TestDoneCancellationWriterRejectsNonterminalFrames ensures that text containing
// [DONE] cannot cancel the request before the actual protocol terminal frame.
func TestDoneCancellationWriterRejectsNonterminalFrames(t *testing.T) {
	c, rec := newBridgeTestCtx(t)
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	c.Writer = &doneCancellationWriter{ResponseWriter: c.Writer, cancel: cancel}

	render.StringData(c, `data: {"choices":[{"delta":{"content":"literal [DONE] token"}}]}`)
	require.NoError(t, ctx.Err(), "literal generated text is not the terminal frame")
	render.StringData(c, "data: [DONE]garbage")
	require.NoError(t, ctx.Err(), "malformed DONE suffix is not a terminal frame")
	render.StringData(c, "data: [DONE]")
	require.ErrorIs(t, ctx.Err(), context.Canceled, "real terminal frame must cancel")
	require.Equal(t, 1, strings.Count(rec.Body.String(), "data: [DONE]\n\n"))
}

// TestDoneCancellationWriterRejectsShortWrites ensures a truncated write does not
// count as delivered protocol completion even if the writer returned nil error.
func TestDoneCancellationWriterRejectsShortWrites(t *testing.T) {
	c, _ := newBridgeTestCtx(t)
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	w := &doneCancellationWriter{
		ResponseWriter: &issue599ShortWriteWriter{ResponseWriter: c.Writer},
		cancel:         cancel,
	}
	data := []byte("data: [DONE]")
	n, err := w.Write(data)
	require.NoError(t, err)
	require.Equal(t, len(data)-1, n)
	require.NoError(t, ctx.Err(), "short writes must not imply delivered completion")
}

// streamTerminalReadFailure retains a genuine non-EOF transport error for the control.
type streamTerminalReadFailure struct{ cause error }

// Read returns the injected independent failure, preserving the historical
// truncated-response default for existing zero-value fixtures.
func (r streamTerminalReadFailure) Read([]byte) (int, error) {
	if r.cause != nil {
		return 0, r.cause
	}
	return 0, io.ErrUnexpectedEOF
}

// TestSharedStreamDoneCancellation distinguishes a completed protocol followed by
// clean EOF from pre-terminal cancellation and genuine trailing transport failure.
func TestSharedStreamDoneCancellation(t *testing.T) {
	for _, tc := range []struct {
		name         string
		trailingErr  error
		cancelOnDone bool
	}{
		{"completed_eof", nil, true},
		{"transport_error_control", io.ErrUnexpectedEOF, true},
		{"independent_closed_without_cancel", net.ErrClosed, false},
		{"independent_closed_after_cancel", net.ErrClosed, true},
		{"independent_truncated_without_cancel", io.ErrUnexpectedEOF, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newBridgeTestCtx(t)
			gmw.SetLogger(c, createTestLogger())
			ctx, cancel := context.WithCancel(c.Request.Context())
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			if tc.cancelOnDone {
				c.Writer = &doneCancellationWriter{ResponseWriter: c.Writer, cancel: cancel}
			}
			c.Set(ctxkey.RequestModel, "gpt-4")
			tracker := streaming.NewQuotaTracker(streaming.QuotaTrackerParams{Ctx: ctx, ModelName: "gpt-4", PromptTokens: 11, PreConsumedQuota: 10000, TokenCounter: func(s, _ string) int { return len(s) }})
			streaming.StoreTracker(c, tracker)
			wire := "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":7,\"total_tokens\":18}}\n\ndata: [DONE]\n\n"
			var source io.Reader = strings.NewReader(wire)
			if tc.trailingErr != nil {
				source = io.MultiReader(source, streamTerminalReadFailure{cause: tc.trailingErr})
			}
			resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(source)}
			apiErr, usage := UnifiedStreamProcessing(c, resp, 11, "gpt-4", false)
			if tc.cancelOnDone {
				require.ErrorIs(t, ctx.Err(), context.Canceled)
			} else {
				require.NoError(t, ctx.Err(), "independent error must not require a canceled request")
			}
			require.Equal(t, 1, strings.Count(rec.Body.String(), "data: [DONE]\n\n"))
			require.NotNil(t, usage)
			require.Equal(t, 18, usage.TotalTokens)
			require.Empty(t, usage.BillingEstimateReason)
			if tc.trailingErr != nil {
				require.NotNil(t, apiErr, "an independent trailing failure must remain a failure")
				require.Equal(t, "read_stream_failed", apiErr.Code)
				require.ErrorIs(t, apiErr.RawError, tc.trailingErr, "retain the independent read cause")
			} else {
				require.Nil(t, apiErr, "normal EOF after processed DONE must remain successful")
			}
		})
	}
}

const issue599Receipt = "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":7,\"total_tokens\":18}}\n\n"
const issue599DONE = "data: [DONE]\n\n"

// issue599Boundary signals the next Body.Read only after every fixture byte
// arrives. It preserves the HTTP transport's original cancellation behavior.
type issue599Boundary struct {
	io.ReadCloser
	expected []byte
	seen     []byte
	entered  chan struct{}
	once     sync.Once
	closes   atomic.Int32
}

func (b *issue599Boundary) Read(p []byte) (int, error) {
	if bytes.Equal(b.seen, b.expected) {
		b.once.Do(func() { close(b.entered) })
	}
	n, err := b.ReadCloser.Read(p)
	b.seen = append(b.seen, p[:n]...)
	return n, err
}

func (b *issue599Boundary) Close() error {
	b.closes.Add(1)
	return b.ReadCloser.Close()
}

// Unwrap declares this fixture's Read/Close delegation to the native body.
func (b *issue599Boundary) Unwrap() io.ReadCloser { return b.ReadCloser }

// TestIssue599NativeHTTPReadAfterDONE is a loopback HTTP/1.1 regression. With
// the original double-cancellation ownership, the post-DONE read may return
// net.ErrClosed rather than context.Canceled and produce a spurious HTTP 500.
func TestIssue599NativeHTTPReadAfterDONE(t *testing.T) {
	configureIssue599TraceFixture(t)
	tests := []struct {
		name       string
		wire       string
		iterations int
		bridge     bool
		success    bool
		doneCount  int
		total      int
		estimate   string
	}{
		{"measured_done", issue599Receipt + issue599DONE, 12, false, true, 1, 18, ""},
		{"responses_bridge", issue599Receipt + issue599DONE, 6, true, true, 0, 18, ""},
		{"before_done", issue599Receipt, 3, false, false, 0, 18, ""},
		{"done_missing_receipt", "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}]}\n\n" + issue599DONE, 3, false, false, 1, 0, ""},
		{"output_after_receipt", issue599Receipt + issue599DONE + "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"later output\"}}]}\n\n", 3, false, false, 1, 30, "stream_output_after_last_receipt"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < tc.iterations; i++ {
				t.Run(fmt.Sprintf("%02d", i), func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, tc.wire)
						w.(http.Flusher).Flush()
						<-r.Context().Done()
					}))
					defer server.Close()

					c, rec := newBridgeTestCtx(t)
					gmw.SetLogger(c, createTestLogger())
					ctx, cancel := context.WithCancel(c.Request.Context())
					defer cancel()
					c.Request = c.Request.WithContext(ctx)
					c.Set(ctxkey.RequestModel, "gpt-4")
					tracker := streaming.NewQuotaTracker(streaming.QuotaTrackerParams{
						Ctx: ctx, ModelName: "gpt-4", PromptTokens: 11,
						PreConsumedQuota: 10000,
						TokenCounter:     func(s, _ string) int { return len(s) },
					})
					streaming.StoreTracker(c, tracker)
					var bridge *bridgeRecorder
					if tc.bridge {
						bridge = &bridgeRecorder{}
						c.Set(ctxkey.ResponseStreamRewriteHandler, bridge)
					}

					request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
					require.NoError(t, err)
					resp, err := relayadaptor.DoRequest(c, request)
					require.NoError(t, err)
					require.NotNil(t, resp.Request)
					require.True(t, relayadaptor.ResponseTransportOwnsCancellation(c, resp))
					boundary := &issue599Boundary{
						ReadCloser: resp.Body, expected: []byte(tc.wire),
						entered: make(chan struct{}),
					}
					resp.Body = boundary
					require.True(t, relayadaptor.ResponseTransportOwnsCancellation(c, resp),
						"transparent read barrier must preserve proven native cancellation")

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
						cancel()
						t.Fatal("did not reach extra HTTP Body.Read")
					}
					cancel()
					var got result
					select {
					case got = <-finished:
					case <-time.After(3 * time.Second):
						t.Fatal("cancel failed to release blocked HTTP read")
					}
					require.Equal(t, int32(1), boundary.closes.Load())
					require.Equal(t, tc.doneCount, strings.Count(rec.Body.String(), "data: [DONE]\n\n"))
					require.NotNil(t, got.usage)
					if tc.success {
						require.Nil(t, got.failure, "completed receipt/DONE must not become read_stream_failed")
					} else {
						require.NotNil(t, got.failure, "incomplete/unmeasured work must not become success")
					}
					if tc.total > 0 {
						require.Equal(t, tc.total, got.usage.TotalTokens)
					}
					if tc.estimate != "" {
						require.Equal(t, tc.estimate, got.usage.BillingEstimateReason)
					}
					if tc.bridge {
						require.Equal(t, 1, bridge.doneCount)
						require.True(t, bridge.usageSet)
					}
				})
			}
		})
	}
}

// issue599IndependentFailure has an ordinary Close callback, but its blocked
// Read is independently released with a genuine error, not by that Close.
type issue599IndependentFailure struct {
	reader     *strings.Reader
	failure    error
	entered    chan struct{}
	closed     chan struct{}
	released   chan struct{}
	enterOnce  sync.Once
	closeOnce  sync.Once
	closeCount atomic.Int32
}

func (b *issue599IndependentFailure) Read(p []byte) (int, error) {
	if b.reader.Len() > 0 {
		return b.reader.Read(p)
	}
	b.enterOnce.Do(func() { close(b.entered) })
	<-b.released
	return 0, b.failure
}

func (b *issue599IndependentFailure) Close() error {
	b.closeCount.Add(1)
	b.closeOnce.Do(func() { close(b.closed) })
	return nil
}

// TestIssue599IndependentFailurePreserved rejects the false positive
// "context canceled + Close called + net.ErrClosed means successful DONE".
func TestIssue599IndependentFailurePreserved(t *testing.T) {
	for _, readFailure := range []error{net.ErrClosed, io.ErrUnexpectedEOF} {
		for _, withDONE := range []bool{false, true} {
			t.Run(fmt.Sprintf("%v/done=%v", readFailure, withDONE), func(t *testing.T) {
				c, rec := newBridgeTestCtx(t)
				gmw.SetLogger(c, createTestLogger())
				ctx, cancel := context.WithCancel(c.Request.Context())
				defer cancel()
				c.Request = c.Request.WithContext(ctx)
				wire := issue599Receipt
				if withDONE {
					wire += issue599DONE
				}
				body := &issue599IndependentFailure{
					reader: strings.NewReader(wire), failure: readFailure,
					entered: make(chan struct{}), closed: make(chan struct{}),
					released: make(chan struct{}),
				}
				// Release even on t.Fatal before Close: a blocked test reader must
				// not outlive its subtest. Normal and cleanup paths may both invoke it.
				releaseRead := sync.OnceFunc(func() { close(body.released) })
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": {"text/event-stream"}},
					Body:       body,
				}
				type streamResult struct {
					failure *model.ErrorWithStatusCode
					usage   *model.Usage
				}
				finished := make(chan streamResult, 1)
				completed := make(chan struct{})
				defer func() {
					releaseRead()
					select {
					case <-completed:
					case <-time.After(3 * time.Second):
						t.Error("independent reader goroutine did not exit during cleanup")
					}
				}()
				go func() {
					defer close(completed)
					failure, usage := UnifiedStreamProcessing(c, resp, 11, "gpt-4", false)
					finished <- streamResult{failure: failure, usage: usage}
				}()
				select {
				case <-body.entered:
				case <-time.After(3 * time.Second):
					t.Fatal("independent reader never blocked")
				}
				cancel()
				select {
				case <-body.closed:
				case <-time.After(3 * time.Second):
					t.Fatal("fallback watcher did not close custom body")
				}
				releaseRead()
				select {
				case got := <-finished:
					require.NotNil(t, got.failure)
					require.ErrorIs(t, got.failure.RawError, readFailure)
					require.Equal(t, "read_stream_failed", got.failure.Code)
					require.NotNil(t, got.usage, "measured receipt must survive independent failure")
					require.Equal(t, 11, got.usage.PromptTokens)
					require.Equal(t, 7, got.usage.CompletionTokens)
					require.Equal(t, 18, got.usage.TotalTokens)
					require.Empty(t, got.usage.BillingEstimateReason)
					if !withDONE && errors.Is(readFailure, net.ErrClosed) {
						require.ErrorIs(t, got.failure.RawError, context.Canceled,
							"pre-DONE cancellation must preserve both observed causes")
					}
				case <-time.After(3 * time.Second):
					t.Fatal("independent error did not complete")
				}
				require.Equal(t, int32(1), body.closeCount.Load())
				if withDONE {
					require.Equal(t, 1, strings.Count(rec.Body.String(), "data: [DONE]\n\n"))
				} else {
					require.NotContains(t, rec.Body.String(), "[DONE]")
				}
			})
		}
	}
}
