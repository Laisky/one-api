package openai_compatible

import (
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
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	relayadaptor "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/streaming"
)

const canceledStreamReceipt = "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":7,\"total_tokens\":18}}\n\n"
const canceledStreamDone = "data: [DONE]\n\n"

// streamReadBarrierBody signals the first read after all fixture bytes were
// consumed, and counts caller-owned closes without changing transport behavior.
type streamReadBarrierBody struct {
	io.ReadCloser
	expected int
	received int
	entered  chan struct{}
	once     sync.Once
	closes   atomic.Int32
}

func (b *streamReadBarrierBody) Read(p []byte) (int, error) {
	if b.received >= b.expected {
		b.once.Do(func() { close(b.entered) })
	}
	n, err := b.ReadCloser.Read(p)
	b.received += n
	return n, err
}

func (b *streamReadBarrierBody) Close() error {
	b.closes.Add(1)
	return b.ReadCloser.Close()
}

// TestRequestBoundSSECancelAfterRead distinguishes cancellation before DONE
// from cancellation after DONE using actual context-bound HTTP/1 traffic.
func TestRequestBoundSSECancelAfterRead(t *testing.T) {
	for _, afterDone := range []bool{false, true} {
		name := "before_done"
		wire := canceledStreamReceipt
		if afterDone {
			name = "after_done"
			wire += canceledStreamDone
		}
		t.Run(name, func(t *testing.T) {
			for i := 0; i < 10; i++ {
				t.Run(fmt.Sprintf("attempt_%d", i), func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, wire)
						w.(http.Flusher).Flush()
						<-req.Context().Done()
					}))
					defer server.Close()

					c, rec := newBridgeTestCtx(t)
					gmw.SetLogger(c, createTestLogger())
					ctx, cancel := context.WithCancel(c.Request.Context())
					defer cancel()
					c.Request = c.Request.WithContext(ctx)
					c.Set(ctxkey.RequestModel, "gpt-4")
					streaming.StoreTracker(c, streaming.NewQuotaTracker(streaming.QuotaTrackerParams{
						Ctx: ctx, ModelName: "gpt-4", PromptTokens: 11,
						PreConsumedQuota: 10000, TokenCounter: func(s, _ string) int { return len(s) },
					}))

					req, err := http.NewRequestWithContext(gmw.Ctx(c), http.MethodGet, server.URL, nil)
					require.NoError(t, err)
					resp, err := relayadaptor.DoRequest(c, req)
					require.NoError(t, err)
					body := &streamReadBarrierBody{ReadCloser: resp.Body, expected: len(wire), entered: make(chan struct{})}
					resp.Body = body

					type streamResult struct {
						apiErr *model.ErrorWithStatusCode
						total  int
					}
					finished := make(chan streamResult, 1)
					go func() {
						apiErr, usage := UnifiedStreamProcessing(c, resp, 11, "gpt-4", false)
						result := streamResult{apiErr: apiErr}
						if usage != nil {
							result.total = usage.TotalTokens
						}
						finished <- result
					}()
					select {
					case <-body.entered:
					case <-time.After(3 * time.Second):
						cancel()
						_ = resp.Body.Close()
						t.Fatal("handler never entered the extra body read")
					}
					cancel()
					select {
					case result := <-finished:
						require.Equal(t, 18, result.total, "authoritative receipt must survive cancellation")
						require.EqualValues(t, 1, body.closes.Load(), "body must be explicitly closed once")
						if afterDone {
							require.Nil(t, result.apiErr, "completed DONE must not become synthetic 500")
							require.Equal(t, 1, strings.Count(rec.Body.String(), "[DONE]"))
						} else {
							require.NotNil(t, result.apiErr, "unfinished stream must not count as success")
							require.ErrorIs(t, result.apiErr.RawError, context.Canceled)
							require.NotContains(t, rec.Body.String(), "[DONE]")
						}
					case <-time.After(3 * time.Second):
						_ = resp.Body.Close()
						t.Fatal("cancellation did not release pending transport read")
					}
				})
			}
		})
	}
}

// independentStreamErrorBody cannot be interrupted by Close. Releasing an
// independent error after Close must never be mistaken for successful DONE.
type independentStreamErrorBody struct {
	source      *strings.Reader
	entered     chan struct{}
	closed      chan struct{}
	release     chan struct{}
	once        sync.Once
	closeOnce   sync.Once
	terminalErr error
	closes      atomic.Int32
}

func (b *independentStreamErrorBody) Read(p []byte) (int, error) {
	if b.source.Len() > 0 {
		return b.source.Read(p)
	}
	b.once.Do(func() { close(b.entered) })
	<-b.release
	return 0, b.terminalErr
}

func (b *independentStreamErrorBody) Close() error {
	b.closes.Add(1)
	b.closeOnce.Do(func() { close(b.closed) })
	return nil
}

// TestIndependentStreamErrorAfterCancellation guards against a false positive
// from a "canceled + Close called" flag: neither proves why a read failed.
func TestIndependentStreamErrorAfterCancellation(t *testing.T) {
	for _, afterDone := range []bool{false, true} {
		for _, tc := range []struct {
			name string
			err  error
		}{
			{"closed", net.ErrClosed},
			{"unexpected_eof", io.ErrUnexpectedEOF},
		} {
			name := "before_done/" + tc.name
			wire := canceledStreamReceipt
			if afterDone {
				name = "after_done/" + tc.name
				wire += canceledStreamDone
			}
			t.Run(name, func(t *testing.T) {
				c, rec := newBridgeTestCtx(t)
				gmw.SetLogger(c, createTestLogger())
				ctx, cancel := context.WithCancel(c.Request.Context())
				defer cancel()
				c.Request = c.Request.WithContext(ctx)
				body := &independentStreamErrorBody{
					source: strings.NewReader(wire), entered: make(chan struct{}),
					closed: make(chan struct{}), release: make(chan struct{}), terminalErr: tc.err,
				}
				defer func() {
					select {
					case <-body.release:
					default:
						close(body.release)
					}
				}()
				resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}
				type streamResult struct {
					apiErr *model.ErrorWithStatusCode
					total  int
				}
				finished := make(chan streamResult, 1)
				go func() {
					apiErr, usage := UnifiedStreamProcessing(c, resp, 11, "gpt-4", false)
					result := streamResult{apiErr: apiErr}
					if usage != nil {
						result.total = usage.TotalTokens
					}
					finished <- result
				}()
				select {
				case <-body.entered:
				case <-time.After(3 * time.Second):
					t.Fatal("post-fixture read not entered")
				}
				cancel()
				select {
				case <-body.closed:
				case <-time.After(3 * time.Second):
					t.Fatal("custom body not closed on cancellation")
				}
				close(body.release)
				select {
				case result := <-finished:
					require.NotNil(t, result.apiErr)
					require.ErrorIs(t, result.apiErr.RawError, tc.err, "independent read error must survive")
					if !afterDone && errors.Is(tc.err, net.ErrClosed) {
						require.ErrorIs(t, result.apiErr.RawError, context.Canceled, "keep cancellation identity and raw cause")
					}
					require.Equal(t, 18, result.total)
					require.EqualValues(t, 1, body.closes.Load())
					if afterDone {
						require.Equal(t, 1, strings.Count(rec.Body.String(), "[DONE]"))
					} else {
						require.NotContains(t, rec.Body.String(), "[DONE]")
					}
				case <-time.After(3 * time.Second):
					t.Fatal("independent failure did not terminate stream")
				}
			})
		}
	}
}
