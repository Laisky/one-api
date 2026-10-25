package adaptor

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/meta"
)

const transportObservationEvent = "upstream transport completed"
const transportPrivatePayload = "synthetic-private-transport-payload"

// transportObservationFixture installs an isolated request logger and client without SQL tracing.
func transportObservationFixture(t *testing.T, transport http.RoundTripper) (*gin.Context, *observer.ObservedLogs, context.CancelFunc) {
	t.Helper()
	priorClient, priorSinks := client.HTTPClient, config.TraceSinks
	client.HTTPClient = &http.Client{Transport: transport, Timeout: 3 * time.Second}
	config.TraceSinks = []string{config.TraceSinkNone}
	core, logs := observer.New(zapcore.DebugLevel)
	logger, err := glog.NewConsoleWithName("transport-observation-test", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	c.Set(ctxkey.ContentType, "application/json")
	gmw.SetLogger(c, logger)
	t.Cleanup(func() {
		cancel()
		client.HTTPClient.CloseIdleConnections()
		client.HTTPClient, config.TraceSinks = priorClient, priorSinks
	})
	return c, logs, cancel
}

// transportObservationFields checks the public diagnostic contract and absence of raw transport data.
func transportObservationFields(t *testing.T, logs *observer.ObservedLogs, wantLevel zapcore.Level) map[string]interface{} {
	t.Helper()
	entries := logs.FilterMessage(transportObservationEvent).All()
	require.Len(t, entries, 1, "one completion event must cover one Client.Do invocation")
	require.Equal(t, wantLevel, entries[0].Level)
	fields := entries[0].ContextMap()
	require.Equal(t, "transport_attempt", fields["log_stage"])
	for _, forbidden := range []string{"error", "errorVerbose", "remote_addr", "local_addr", "url", "headers", "payload", "accepted_work"} {
		require.NotContains(t, fields, forbidden)
	}
	raw, err := json.Marshal(fields)
	require.NoError(t, err)
	for _, secret := range []string{transportPrivatePayload, "synthetic-private-header", "synthetic-private-credential"} {
		require.NotContains(t, string(raw), secret)
	}
	return fields
}

// TestTransportObservationNativeCompletion records actual reset/header/context outcomes and composes existing traces.
func TestTransportObservationNativeCompletion(t *testing.T) {
	for _, mode := range []string{"reset", "success", "http_error", "cancel_after_write", "joined_reset_cancel"} {
		t.Run(mode, func(t *testing.T) {
			writeComplete := make(chan struct{}, 1)
			received := make(chan struct{}, 1)
			var requests, oldGot, oldWrote, oldFirst atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error("synthetic request body read failed")
					return
				}
				if string(raw) != transportPrivatePayload {
					t.Error("synthetic request payload changed")
					return
				}
				requests.Add(1)
				select {
				case <-writeComplete:
				case <-r.Context().Done():
					return
				}
				received <- struct{}{}
				if mode == "cancel_after_write" || mode == "joined_reset_cancel" {
					<-r.Context().Done()
					return
				}
				if mode == "reset" {
					conn, _, hijackErr := w.(http.Hijacker).Hijack()
					if hijackErr != nil {
						t.Error("fixture hijack failed")
						return
					}
					tcp, ok := conn.(*net.TCPConn)
					if !ok {
						t.Error("fixture was not TCP")
						_ = conn.Close()
						return
					}
					if lingerErr := tcp.SetLinger(0); lingerErr != nil {
						t.Error("fixture reset setup failed")
					}
					_ = conn.Close()
					return
				}
				if mode == "http_error" {
					w.WriteHeader(http.StatusInternalServerError)
				}
				_, _ = io.WriteString(w, "synthetic response")
			}))
			defer server.Close()
			transport := &http.Transport{Proxy: nil}
			c, logs, cancel := transportObservationFixture(t, transport)
			var cancelCause context.CancelCauseFunc
			if mode == "joined_reset_cancel" {
				joinedCtx, stop := context.WithCancelCause(c.Request.Context())
				cancelCause = stop
				defer stop(nil)
				c.Request = c.Request.WithContext(joinedCtx)
			}
			c.Request.Header.Set("X-Synthetic-Private", "synthetic-private-header")
			originalDone := c.Request.Context().Done()
			trace := &httptrace.ClientTrace{
				GotConn: func(httptrace.GotConnInfo) { oldGot.Add(1) },
				WroteRequest: func(info httptrace.WroteRequestInfo) {
					oldWrote.Add(1)
					if info.Err == nil {
						select {
						case writeComplete <- struct{}{}:
						default:
						}
					}
				},
				GotFirstResponseByte: func() { oldFirst.Add(1) },
			}
			c.Request = c.Request.WithContext(httptrace.WithClientTrace(c.Request.Context(), trace))
			type outcome struct {
				response *http.Response
				err      error
			}
			result := make(chan outcome, 1)
			go func() {
				response, err := DoRequestHelper(&stubAdaptor{defaultURL: server.URL}, c, &meta.Meta{ActualModelName: "fixture-model", APIKey: "synthetic-private-credential"}, bytes.NewBufferString(transportPrivatePayload))
				result <- outcome{response, err}
			}()
			select {
			case <-received:
			case <-time.After(2 * time.Second):
				cancel()
				t.Fatal("fixture request did not reach the write barrier")
			}
			if mode == "cancel_after_write" {
				cancel()
			} else if mode == "joined_reset_cancel" {
				cancelCause(errors.Join(context.Canceled, syscall.ECONNRESET))
			}
			var got outcome
			select {
			case got = <-result:
			case <-time.After(2 * time.Second):
				cancel()
				t.Fatal("transport did not complete within its fixture bound")
			}
			if got.response != nil {
				t.Cleanup(func() { _ = got.response.Body.Close() })
			}
			require.EqualValues(t, 1, requests.Load())
			require.Equal(t, originalDone, c.Request.Context().Done(), "trace observation must preserve cancellation identity")
			require.EqualValues(t, 1, oldGot.Load(), "existing GotConn callback must remain composed")
			require.EqualValues(t, 1, oldWrote.Load(), "existing write callback must remain composed")
			level := zapcore.DebugLevel
			if got.err != nil {
				level = zapcore.WarnLevel
			} else if mode == "http_error" {
				level = zapcore.InfoLevel
			}
			fields := transportObservationFields(t, logs, level)
			require.Equal(t, "observed", fields["transport_trace_state"])
			require.EqualValues(t, 1, fields["transport_got_conn_count"])
			require.EqualValues(t, 0, fields["transport_reused_conn_count"])
			require.EqualValues(t, 1, fields["transport_write_completed_count"])
			require.EqualValues(t, 0, fields["transport_write_failed_count"])
			encoded, marshalErr := json.Marshal(fields)
			require.NoError(t, marshalErr)
			require.NotContains(t, string(encoded), server.Listener.Addr().String())
			if mode == "reset" || mode == "cancel_after_write" || mode == "joined_reset_cancel" {
				require.Nil(t, got.response)
				require.Error(t, got.err)
				require.Equal(t, false, fields["response_headers_received"])
				require.NotContains(t, fields, "upstream_http_status", "local failure status must not be published as upstream HTTP status")
				require.Equal(t, "before_response_headers", fields["transport_failure_phase"])
				require.EqualValues(t, 0, fields["transport_first_response_byte_count"])
				require.Zero(t, oldFirst.Load())
				if mode == "reset" {
					require.ErrorIs(t, got.err, syscall.ECONNRESET)
					require.Equal(t, "connection_reset", fields["transport_failure_class"])
					require.Equal(t, "active", fields["request_context_state"])
				} else if mode == "joined_reset_cancel" {
					require.ErrorIs(t, got.err, syscall.ECONNRESET)
					require.ErrorIs(t, got.err, context.Canceled)
					require.Equal(t, "connection_reset", fields["transport_failure_class"], "independent reset evidence cannot disappear behind cancellation")
					require.Equal(t, "canceled", fields["request_context_state"])
				} else {
					require.ErrorIs(t, got.err, context.Canceled)
					require.Equal(t, "canceled", fields["transport_failure_class"])
					require.Equal(t, "canceled", fields["request_context_state"])
				}
			} else {
				require.NoError(t, got.err)
				require.NotNil(t, got.response)
				require.True(t, ResponseTransportOwnsCancellation(c, got.response), "composed trace must retain native cancellation provenance")
				require.NoError(t, got.response.Body.Close())
				require.Equal(t, true, fields["response_headers_received"])
				require.EqualValues(t, got.response.StatusCode, fields["upstream_http_status"])
				require.Equal(t, "none", fields["transport_failure_class"])
				require.Equal(t, "none", fields["transport_failure_phase"])
				require.EqualValues(t, 1, fields["transport_first_response_byte_count"])
				require.EqualValues(t, 1, oldFirst.Load())
			}
		})
	}
}

// TestTransportObservationReusedConnection distinguishes observed reuse from a guessed stale-pool cause.
func TestTransportObservationReusedConnection(t *testing.T) {
	writes := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-writes:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, "synthetic success")
	}))
	defer server.Close()
	c, logs, _ := transportObservationFixture(t, &http.Transport{Proxy: nil})
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(httptrace.WithClientTrace(c.Request.Context(), &httptrace.ClientTrace{
			WroteRequest: func(info httptrace.WroteRequestInfo) {
				if info.Err == nil {
					writes <- struct{}{}
				}
			},
		}), http.MethodGet, server.URL, nil)
		require.NoError(t, err)
		response, err := DoRequest(c, req)
		require.NoError(t, err)
		_, err = io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
	}
	entries := logs.FilterMessage(transportObservationEvent).All()
	require.Len(t, entries, 2)
	require.EqualValues(t, 0, entries[0].ContextMap()["transport_reused_conn_count"])
	require.EqualValues(t, 1, entries[1].ContextMap()["transport_reused_conn_count"])
	require.EqualValues(t, 1, entries[1].ContextMap()["transport_got_conn_count"])
}

// transportObservationOpaque is a callback-free custom transport with an explicit fixture result.
type transportObservationOpaque struct {
	response *http.Response
	failure  error
	calls    atomic.Int32
}

// RoundTrip returns its fixture result unchanged without implying a network callback or accepted work.
func (r *transportObservationOpaque) RoundTrip(*http.Request) (*http.Response, error) {
	r.calls.Add(1)
	return r.response, r.failure
}

// TestTransportObservationOpaqueAndContextControls keeps absent custom callbacks unknown and preserves context classes.
func TestTransportObservationOpaqueAndContextControls(t *testing.T) {
	for _, mode := range []string{"opaque_error", "opaque_success", "opaque_unexpected_eof", "canceled", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			opaque := &transportObservationOpaque{failure: errors.New("synthetic opaque transport failure")}
			if mode == "opaque_unexpected_eof" {
				opaque.failure = io.ErrUnexpectedEOF
			}
			var transport http.RoundTripper = opaque
			if mode == "opaque_success" {
				opaque.failure = nil
				opaque.response = &http.Response{StatusCode: http.StatusCreated, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("synthetic response"))}
			}
			if mode == "canceled" || mode == "deadline" {
				transport = &http.Transport{Proxy: nil}
			}
			c, logs, cancel := transportObservationFixture(t, transport)
			if mode == "canceled" {
				cancel()
			}
			if mode == "deadline" {
				ctx, deadlineCancel := context.WithDeadline(c.Request.Context(), time.Unix(1, 0))
				defer deadlineCancel()
				c.Request = c.Request.WithContext(ctx)
			}
			request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, "http://127.0.0.1:1/fixture", nil)
			require.NoError(t, err)
			response, err := DoRequest(c, request)
			level := zapcore.WarnLevel
			if mode == "opaque_success" {
				require.NoError(t, err)
				require.NoError(t, response.Body.Close())
				level = zapcore.DebugLevel
			} else {
				require.Error(t, err)
			}
			fields := transportObservationFields(t, logs, level)
			require.Equal(t, "unknown", fields["transport_trace_state"])
			require.EqualValues(t, 0, fields["transport_got_conn_count"])
			require.EqualValues(t, 0, fields["transport_write_completed_count"])
			if mode == "canceled" || mode == "deadline" {
				want := context.Canceled
				label := "canceled"
				if mode == "deadline" {
					want = context.DeadlineExceeded
					label = "deadline_exceeded"
				}
				require.ErrorIs(t, err, want)
				require.Equal(t, label, fields["transport_failure_class"])
				require.Equal(t, label, fields["request_context_state"])
			} else {
				require.EqualValues(t, 1, opaque.calls.Load())
				if mode == "opaque_unexpected_eof" {
					require.ErrorIs(t, err, io.ErrUnexpectedEOF)
					require.Equal(t, "other", fields["transport_failure_class"], "a callback-free truncated response is not evidence of a connection reset")
					require.Equal(t, "active", fields["request_context_state"])
				}
			}
			if response == nil {
				require.Equal(t, false, fields["response_headers_received"])
				require.NotContains(t, fields, "upstream_http_status")
				require.Equal(t, "unknown", fields["transport_failure_phase"])
			} else {
				require.EqualValues(t, http.StatusCreated, fields["upstream_http_status"])
			}
		})
	}
}
