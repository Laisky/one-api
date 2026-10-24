package controller

import (
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

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	relayadaptor "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/model"
)

const portable599Receipt = "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":7,\"total_tokens\":18}}\n\n"

// portable599Boundary delegates Read and Close while exposing the post-payload read barrier.
type portable599Boundary struct {
	io.ReadCloser
	expected int
	seen     int
	entered  chan struct{}
	once     sync.Once
	closes   atomic.Int32
}

// Read signals the read after all synthetic wire bytes have reached the parser.
func (b *portable599Boundary) Read(p []byte) (int, error) {
	if b.seen >= b.expected {
		b.once.Do(func() { close(b.entered) })
	}
	n, err := b.ReadCloser.Read(p)
	b.seen += n
	return n, err
}

// Close delegates unchanged and counts calls made by the stream handler.
func (b *portable599Boundary) Close() error {
	b.closes.Add(1)
	return b.ReadCloser.Close()
}

// Unwrap advertises this wrapper's exact Read/Close delegation contract.
func (b *portable599Boundary) Unwrap() io.ReadCloser { return b.ReadCloser }

// portable599Opaque keeps an independent reader blocked even when Close is called.
type portable599Opaque struct {
	prefix      *strings.Reader
	entered     chan struct{}
	closed      chan struct{}
	release     chan struct{}
	enterOnce   sync.Once
	closeOnce   sync.Once
	releaseOnce sync.Once
	closes      atomic.Int32
}

// Read drains its own bytes then reports an independently released transport error.
func (b *portable599Opaque) Read(p []byte) (int, error) {
	if b.prefix.Len() > 0 {
		return b.prefix.Read(p)
	}
	b.enterOnce.Do(func() { close(b.entered) })
	<-b.release
	return 0, net.ErrClosed
}

// Close acknowledges cancellation cleanup without causing the independent read error.
func (b *portable599Opaque) Close() error {
	b.closes.Add(1)
	b.closeOnce.Do(func() { close(b.closed) })
	return nil
}

// unblock releases the independent error exactly once during observation or rescue cleanup.
func (b *portable599Opaque) unblock() { b.releaseOnce.Do(func() { close(b.release) }) }

// portable599Result carries completed handler state across the join boundary.
type portable599Result struct {
	failure *model.ErrorWithStatusCode
	usage   *model.Usage
}

// TestIssue599PortableDispatchProvenance uses APIs shared by PR600 and PR601.
// It checks delivered behavior rather than either proposal's provenance helper.
func TestIssue599PortableDispatchProvenance(t *testing.T) {
	tests := []struct {
		name           string
		mode           string
		marker         string
		success        bool
		done           int
		cancelIdentity bool
	}{
		{"custom_replaced_before_done", "custom", "", false, 0, true},
		{"bound_before_done", "bound", "", false, 0, true},
		{"opaque_replaced_body", "opaque", "[DONE]", false, 1, false},
		{"rebound_handler_context", "rebound", "[DONE]", false, 1, false},
		{"logging_wrapper_cancel", "bound", "[DONE]", true, 1, false},
		{"logging_wrapper_happy_eof", "eof", "[DONE]", true, 1, false},
		{"malformed_done_suffix", "bound", "[DONE]garbage", false, 0, false},
		{"malformed_done_double_marker", "bound", "[DONE] [DONE]", false, 0, false},
		{"done_whitespace_control", "bound", "[DONE] \t", true, 1, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wire := portable599Receipt
			if tc.marker != "" {
				wire += "data: " + tc.marker + "\n\n"
			}
			providerRelease := make(chan struct{})
			var providerReleaseOnce sync.Once
			releaseProvider := func() { providerReleaseOnce.Do(func() { close(providerRelease) }) }
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, wire)
				w.(http.Flusher).Flush()
				select {
				case <-r.Context().Done():
				case <-providerRelease:
				}
			}))
			transport := &http.Transport{Proxy: nil, DisableKeepAlives: true}
			oldClient, oldSinks := client.HTTPClient, config.TraceSinks
			client.HTTPClient = &http.Client{Transport: transport}
			config.TraceSinks = []string{config.TraceSinkNone}
			originalCtx, cancelOriginal := context.WithCancel(context.Background())
			cancelHandler := cancelOriginal
			var originalBody io.ReadCloser
			var response *http.Response
			var opaque *portable599Opaque
			finished := make(chan portable599Result, 1)
			launched, joined := false, false
			t.Cleanup(func() {
				cancelHandler()
				cancelOriginal()
				releaseProvider()
				if opaque != nil {
					opaque.unblock()
				}
				if launched && !joined {
					select {
					case <-finished:
						joined = true
					case <-time.After(3 * time.Second):
						t.Error("handler remained blocked after all owned rescue signals")
					}
				}
				// Read the handler-mutated response only after joining its goroutine.
				if !launched || joined {
					if response != nil && response.Body != nil {
						_ = response.Body.Close()
					} else if originalBody != nil {
						_ = originalBody.Close()
					}
				}
				server.CloseClientConnections()
				server.Close()
				transport.CloseIdleConnections()
				client.HTTPClient, config.TraceSinks = oldClient, oldSinks
			})
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(originalCtx)
			logger, err := glog.New(glog.WithName("issue599-portable-provenance"))
			require.NoError(t, err)
			gmw.SetLogger(c, logger)
			req, err := http.NewRequestWithContext(originalCtx, http.MethodGet, server.URL, nil)
			require.NoError(t, err)
			resp, err := relayadaptor.DoRequest(c, req)
			require.NoError(t, err)
			originalBody = resp.Body
			response = resp
			if tc.mode == "rebound" {
				rebound, reboundCancel := context.WithCancel(context.WithoutCancel(originalCtx))
				cancelHandler = reboundCancel
				c.Request = c.Request.WithContext(rebound)
			}
			if tc.mode == "opaque" || tc.mode == "custom" {
				opaque = &portable599Opaque{prefix: strings.NewReader(wire), entered: make(chan struct{}), closed: make(chan struct{}), release: make(chan struct{})}
				resp.Body = opaque
			}
			if tc.mode == "custom" {
				resp.Request = nil
			}
			// This is the actual controller logging wrapper, not a fixture substitute.
			capture := wrapUpstreamResponse(resp)
			require.NotNil(t, capture)
			boundary := &portable599Boundary{ReadCloser: resp.Body, expected: len(wire), entered: make(chan struct{})}
			resp.Body = boundary
			launched = true
			go func() {
				failure, usage := openai_compatible.UnifiedStreamProcessing(c, resp, 11, "gpt-4", false)
				finished <- portable599Result{failure: failure, usage: usage}
			}()
			select {
			case <-boundary.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("handler did not enter the post-payload body read")
			}
			timely := true
			var got portable599Result
			if tc.mode == "eof" {
				releaseProvider()
			} else {
				cancelHandler()
			}
			if opaque != nil {
				select {
				case <-opaque.closed:
				case <-time.After(750 * time.Millisecond):
					timely = false
				}
				// The read failure remains independent of Close; both must stay visible.
				opaque.unblock()
			}
			select {
			case got = <-finished:
				joined = true
			case <-time.After(750 * time.Millisecond):
				timely = false
				cancelOriginal()
				releaseProvider()
				_ = originalBody.Close()
				select {
				case got = <-finished:
					joined = true
				case <-time.After(3 * time.Second):
					t.Fatal("handler failed to return after bounded rescue cleanup")
				}
			}
			require.NotNil(t, got.usage)
			require.Equal(t, 18, got.usage.TotalTokens, "measured receipt must survive terminal errors and cancellation")
			require.Equal(t, int32(1), boundary.closes.Load(), "handler must close the installed body exactly once")
			_, _, total := capture.Snapshot()
			require.Equal(t, len(wire), total, "logging wrapper must account every upstream fixture byte")
			if opaque != nil {
				require.Equal(t, int32(1), opaque.closes.Load())
				require.NotNil(t, got.failure)
				require.ErrorIs(t, got.failure.RawError, net.ErrClosed, "an independently released failure cannot become cancellation success")
			}
			require.True(t, timely, "cancellation must reach the currently installed reader without cancelling a different original context or independently releasing its Read")
			require.Equal(t, tc.done, strings.Count(rec.Body.String(), "data: [DONE]"))
			if tc.success {
				require.Nil(t, got.failure)
			} else {
				require.NotNil(t, got.failure)
			}
			if tc.cancelIdentity {
				require.NotNil(t, got.failure)
				require.ErrorIs(t, got.failure.RawError, context.Canceled, "pre-terminal interruption must preserve cancellation together with the independent read cause")
			}
			if tc.mode == "eof" {
				require.NoError(t, originalCtx.Err())
			}
		})
	}
}
