package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// rawReceiptLifecycleTimeout bounds every wait so a lifecycle regression fails
// the test instead of hanging the package.
const rawReceiptLifecycleTimeout = 10 * time.Second

// rawReceiptSignalWriter closes seen once the client has been sent a write
// containing marker, so a test can cancel exactly after a delivered delta.
type rawReceiptSignalWriter struct {
	gin.ResponseWriter
	marker string
	once   sync.Once
	seen   chan struct{}
}

// Write delivers p and signals when it carries the marker. Parameters: p is
// the client payload. Returns: the underlying writer's result.
func (w *rawReceiptSignalWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if strings.Contains(string(p), w.marker) {
		w.once.Do(func() { close(w.seen) })
	}
	return n, err
}

// WriteString follows the same signalling boundary as Write. Parameters: s is
// the client payload. Returns: the underlying writer's result.
func (w *rawReceiptSignalWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// rawReceiptDoResult carries one DoResponse result across goroutines.
type rawReceiptDoResult struct {
	usage  *model.Usage
	apiErr *model.ErrorWithStatusCode
}

// rawReceiptRunDoResponse runs DoResponse on its own goroutine, as the relay
// handler does, and returns the channel that receives its result. Parameters:
// c, resp and info are the attempt. Returns: a buffered result channel.
func rawReceiptRunDoResponse(c *gin.Context, resp *http.Response, info *metalib.Meta) <-chan rawReceiptDoResult {
	done := make(chan rawReceiptDoResult, 1)
	go func() {
		usage, apiErr := (&Adaptor{ChannelType: channeltype.OpenAI}).DoResponse(c, resp, info)
		done <- rawReceiptDoResult{usage: usage, apiErr: apiErr}
	}()
	return done
}

// rawReceiptAwait waits for a DoResponse result or fails after the lifecycle
// timeout. Parameters: t asserts and done is the result channel. Returns: the result.
func rawReceiptAwait(t *testing.T, done <-chan rawReceiptDoResult) rawReceiptDoResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(rawReceiptLifecycleTimeout):
		require.FailNow(t, "DoResponse did not return after the client cancelled")
		return rawReceiptDoResult{}
	}
}

// rawReceiptCancellableContext prepares a native Cyber stream attempt whose
// client request can be cancelled. Parameters: mode is the client relay mode
// and baseURL the provider. Returns: the context, recorder, metadata and cancel.
func rawReceiptCancellableContext(t *testing.T, mode int, baseURL string) (*gin.Context, *httptest.ResponseRecorder, *metalib.Meta, context.CancelFunc) {
	t.Helper()
	c, w := newDirectStreamContext()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{}`)).WithContext(ctx)
	info := &metalib.Meta{ChannelType: channeltype.OpenAI, APIKey: "fixture", BaseURL: baseURL,
		ActualModelName: "gpt-6-cyber", OriginModelName: "gpt-6-cyber", Mode: mode, IsStream: true,
		RequestURLPath: "/v1/responses", StartTime: time.Now().UTC(), PromptTokens: rawReceiptPromptTokens}
	metalib.Set2Context(c, info)
	c.Set(ctxkey.ConvertedRequest, &ResponseAPIRequest{Model: "gpt-6-cyber"})
	if mode == relaymode.ClaudeMessages {
		c.Set(ctxkey.ClaudeMessagesConversion, true)
	}
	return c, w, info, cancel
}

// TestRawUsageReceiptClientCancelMidStreamHTTP dispatches a real native HTTP
// request, lets the provider deliver lifecycle frames and two deltas, and then
// cancels the client request (a client disconnect) while the provider is still
// generating. For every client format, DoResponse must return promptly without
// a panic or race, release the provider connection, deliver exactly the deltas
// sent before the disconnect with no fabricated completion, and settle a
// flagged estimate that counts the delivered output.
func TestRawUsageReceiptClientCancelMidStreamHTTP(t *testing.T) {
	previous := config.TraceSinks
	config.TraceSinks = []string{config.TraceSinkNone}
	t.Cleanup(func() { config.TraceSinks = previous })

	for _, mode := range rawReceiptModes {
		t.Run(mode.name, func(t *testing.T) {
			disconnected := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, rawReceiptProvisional()+rawReceiptDelta("before cancel ")+rawReceiptDelta("client left"))
				w.(http.Flusher).Flush()
				select {
				case <-r.Context().Done():
					close(disconnected)
				case <-time.After(rawReceiptLifecycleTimeout):
				}
			}))
			t.Cleanup(server.Close)

			c, w, info, cancel := rawReceiptCancellableContext(t, mode.mode, server.URL)
			signal := &rawReceiptSignalWriter{ResponseWriter: c.Writer, marker: "client left", seen: make(chan struct{})}
			c.Writer = signal
			resp, err := adaptor.DoRequestHelper(&Adaptor{ChannelType: channeltype.OpenAI}, c, info,
				strings.NewReader(`{"model":"gpt-6-cyber","stream":true}`))
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })

			done := rawReceiptRunDoResponse(c, resp, info)
			select {
			case <-signal.seen:
			case <-time.After(rawReceiptLifecycleTimeout):
				require.FailNow(t, "the second delta never reached the client")
			}
			cancel()
			result := rawReceiptAwait(t, done)
			select {
			case <-disconnected:
			case <-time.After(rawReceiptLifecycleTimeout):
				require.FailNow(t, "the provider connection was not released after the client left")
			}

			require.ErrorIs(t, c.Request.Context().Err(), context.Canceled)
			requireRawReceiptTerminalError(t, mode.mode, result.apiErr)
			requireRawReceiptDelivery(t, w.Body.String(), "before cancel ", "client left")
			requireRawReceiptEstimate(t, result.usage, 20, "default")
			if mode.mode == relaymode.ClaudeMessages {
				requireRawReceiptClaudeStored(t, c, result.usage)
			}
		})
	}
}

// rawReceiptStubbornBody is a provider body whose pending Read ignores Close,
// like a stalled transport, so a reader goroutine abandoned by the heartbeat
// line reader keeps reading after DoResponse has settled the receipt. parked
// counts Reads in progress, delivered the bytes Reads returned, and closes the
// Close calls.
type rawReceiptStubbornBody struct {
	*io.PipeReader
	parked    atomic.Int32
	delivered atomic.Int64
	closes    atomic.Int32
}

// Read records the call while it blocks on the pipe. Parameters: p is the
// destination. Returns: the pipe's result.
func (b *rawReceiptStubbornBody) Read(p []byte) (int, error) {
	b.parked.Add(1)
	n, err := b.PipeReader.Read(p)
	b.delivered.Add(int64(n))
	b.parked.Add(-1)
	return n, err
}

// Close records the release without unblocking a pending Read. It takes no
// parameters and returns nil.
func (b *rawReceiptStubbornBody) Close() error {
	b.closes.Add(1)
	return nil
}

// rawReceiptObserverOf unwraps the stream-handler wrappers DoResponse leaves on
// resp.Body and returns the installed raw usage receipt. Parameters: t asserts
// and body is the response body after DoResponse. Returns: the observer.
func rawReceiptObserverOf(t *testing.T, body io.ReadCloser) *rawUsageReceipt {
	t.Helper()
	for range 4 {
		switch wrapped := body.(type) {
		case *rawUsageReceipt:
			return wrapped
		case *responseStreamBody:
			body = wrapped.ReadCloser
		case *webSearchStreamCounter:
			body = wrapped.body
		default:
			require.FailNowf(t, "unexpected response body wrapper", "%T", body)
		}
	}
	require.FailNow(t, "raw usage receipt not found")
	return nil
}

// rawReceiptWaitPendingRead waits until every written byte was returned by a
// Read and a new Read is parked on the provider body. The stream reader issues
// that Read only after its previous Read returned up the whole wrapper chain,
// so every byte was observed and every complete line handled. Parameters: t
// asserts, body is the provider body and written the bytes written so far.
func rawReceiptWaitPendingRead(t *testing.T, body *rawReceiptStubbornBody, written int) {
	t.Helper()
	require.Eventually(t, func() bool { return body.delivered.Load() == int64(written) && body.parked.Load() == 1 },
		rawReceiptLifecycleTimeout, time.Millisecond, "the stream reader never parked a read on the provider body")
}

// TestRawUsageReceiptLateReadAfterFinishThroughDoResponse cancels the client
// while the heartbeat reader's goroutine is parked in a provider Read, so that
// goroutine outlives DoResponse and completes its Read after the deferred
// receipt finish. Late provider bytes (a delta, then a huge terminal receipt)
// race concurrent finish calls and Close. The settled snapshot must be frozen:
// every later finish returns the snapshot taken when DoResponse returned, and
// the late receipt never rewrites the settled estimate.
func TestRawUsageReceiptLateReadAfterFinishThroughDoResponse(t *testing.T) {
	lateReceipt := `{"input_tokens":999999,"output_tokens":999999,"total_tokens":1999998}`
	for _, mode := range rawReceiptModes {
		t.Run(mode.name, func(t *testing.T) {
			c, w, info, cancel := rawReceiptCancellableContext(t, mode.mode, "")
			reader, writer := io.Pipe()
			t.Cleanup(func() { _ = writer.Close() })
			body := &rawReceiptStubbornBody{PipeReader: reader}
			resp := rawReceiptStreamResponse(body)
			done := rawReceiptRunDoResponse(c, resp, info)

			written, err := io.WriteString(writer, rawReceiptProvisional()+rawReceiptDelta("settled before ")+rawReceiptDelta("the cancel"))
			require.NoError(t, err)
			rawReceiptWaitPendingRead(t, body, written)
			cancel()
			result := rawReceiptAwait(t, done)
			requireRawReceiptTerminalError(t, mode.mode, result.apiErr)
			requireRawReceiptDelivery(t, w.Body.String(), "settled before ", "the cancel")
			requireRawReceiptEstimate(t, result.usage, 20, "default")
			if mode.mode == relaymode.ClaudeMessages {
				requireRawReceiptClaudeStored(t, c, result.usage)
			}
			require.Positive(t, body.closes.Load(), "the handler must release the provider body")

			receipt := rawReceiptObserverOf(t, resp.Body)
			frozen := receipt.finish(c, nil, false)
			require.NotNil(t, frozen)
			require.NotEmpty(t, frozen.BillingEstimateReason)

			// The abandoned goroutine's parked Read now completes concurrently
			// with repeated finish calls and both Close paths.
			var wg sync.WaitGroup
			snapshots := make([]*model.Usage, 16)
			wg.Go(func() { _, _ = io.WriteString(writer, rawReceiptDelta("late delta after settlement")) })
			for i := range snapshots {
				wg.Go(func() { snapshots[i] = receipt.finish(c, nil, false) })
			}
			wg.Go(func() { _ = resp.Body.Close() })
			wg.Go(func() { _ = receipt.Close() })
			wg.Wait()
			for _, snapshot := range snapshots {
				require.Equal(t, frozen, snapshot, "a late read must not change the settled snapshot")
			}

			// A sequenced late read of a complete terminal receipt is observed by
			// the wrapper after finish and must still be ignored.
			go func() { _, _ = io.WriteString(writer, rawReceiptCompleted("late", lateReceipt)) }()
			buffer := make([]byte, 64<<10)
			n, readErr := receipt.Read(buffer)
			require.NoError(t, readErr)
			require.Positive(t, n)
			require.Contains(t, string(buffer[:n]), "999999", "control: the late receipt bytes were read through the observer")
			require.Equal(t, frozen, receipt.finish(c, nil, false), "late provider bytes must not rewrite a settled receipt")
			require.Less(t, result.usage.PromptTokens, 999999)
			require.Less(t, result.usage.CompletionTokens, 999999)
		})
	}
}
