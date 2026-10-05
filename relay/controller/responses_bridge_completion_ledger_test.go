package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// bridgeLifecycleBody observes actual HTTP read outcomes without substituting provider bytes.
type bridgeLifecycleBody struct {
	io.ReadCloser
	eof, truncated *atomic.Bool
}

// Read records the real transport outcome while returning its original bytes and error.
func (b *bridgeLifecycleBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if errors.Is(err, io.EOF) {
		b.eof.Store(true)
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		b.truncated.Store(true)
	}
	return n, err
}

// bridgeLifecycleTransport delegates to the real local TLS provider transport.
type bridgeLifecycleTransport struct {
	base           http.RoundTripper
	eof, truncated *atomic.Bool
}

// RoundTrip instruments only body observations on an otherwise genuine HTTP response.
func (t *bridgeLifecycleTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(r)
	if err != nil {
		return resp, err
	}
	resp.Body = &bridgeLifecycleBody{ReadCloser: resp.Body, eof: t.eof, truncated: t.truncated}
	return resp, nil
}

// bridgeLifecycleWriter cancels after a selected actual Responses event reaches the client.
type bridgeLifecycleWriter struct {
	gin.ResponseWriter
	cancel context.CancelFunc
	event  string
	once   sync.Once
	notify chan bool
	seen   bool
}

// CloseNotify preserves Gin's explicit downstream notification boundary.
func (w *bridgeLifecycleWriter) CloseNotify() <-chan bool { return w.notify }

// Write first delivers the complete real bridge frame, then cancels at the chosen boundary.
func (w *bridgeLifecycleWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if err == nil && w.event != "" && strings.Contains(string(p), "event: "+w.event+"\n") {
		w.once.Do(func() { w.seen = true; w.cancel(); close(w.notify) })
	}
	return n, err
}

// WriteString preserves the same delivery boundary for string renderers.
func (w *bridgeLifecycleWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// TestResponsesBridgeCompletionLedgerHTTP verifies real registered adapters, transport,
// terminal Responses events, receipt chronology and exactly one physical settlement.
func TestResponsesBridgeCompletionLedgerHTTP(t *testing.T) {
	for _, route := range []struct {
		name, actual string
		channel      int
	}{
		{"native_adapter", "gpt-4", channeltype.OpenAICompatible},
		{"shared_adapter", "deepseek-chat", channeltype.DeepSeek},
	} {
		for _, tc := range []struct {
			name, cancelEvent                                       string
			later, done, fresh, failure, truncate, malformed, quota bool
		}{
			{name: "measured_done", done: true},
			{name: "measured_done_late_cancel", done: true, cancelEvent: "response.completed"},
			{name: "stale_receipt_done_late_cancel", later: true, done: true, cancelEvent: "response.completed"},
			{name: "fresh_final_receipt_done_late_cancel", later: true, done: true, fresh: true, cancelEvent: "response.completed"},
			{name: "measured_canceled_ordinary_eof", failure: true, cancelEvent: "response.created"},
			{name: "stale_receipt_canceled_ordinary_eof", later: true, failure: true, cancelEvent: "response.output_text.delta"},
			{name: "truncated_transport", later: true, failure: true, truncate: true},
			{name: "done_then_truncated_transport", later: true, done: true, failure: true, truncate: true},
			{name: "malformed_then_truncated_transport", later: true, failure: true, truncate: true, malformed: true},
			{name: "quota_error", later: true, done: true, failure: true, quota: true},
		} {
			t.Run(route.name+"/"+tc.name, func(t *testing.T) {
				balance := int64(1_000_000)
				if tc.quota {
					balance = 150
				}
				xaiVideoSetup(t, balance, false)
				oldPre := config.PreConsumedQuota
				config.PreConsumedQuota = 20
				t.Cleanup(func() { config.PreConsumedQuota = oldPre })
				later := ""
				if tc.later {
					later = strings.Repeat("later bridge output ", 100)
				}
				expected := int64(18)
				estimated := tc.later && !tc.fresh
				if estimated {
					expected += int64(openai.CountTokenText(later, route.actual))
				}
				receipt := "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":7,\"total_tokens\":18}}\n\n"
				wire := receipt
				if tc.later {
					raw, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": later}}}})
					require.NoError(t, err)
					wire += "data: " + string(raw) + "\n\n"
				}
				if tc.fresh {
					wire += receipt
				}
				if tc.malformed {
					wire += "data: {\"choices\":[malformed\n\n"
				}
				if tc.done {
					wire += "data: [DONE]\n\n"
				}
				var calls atomic.Int32
				var rawEOF, rawTruncated atomic.Bool
				providerBody := make(chan []byte, 1)
				providerPath := make(chan string, 1)
				providerWritten := make(chan int, 1)
				providerFailure := make(chan error, 1)
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					body, err := io.ReadAll(io.LimitReader(r.Body, 16<<10))
					providerBody <- body
					providerPath <- r.URL.Path
					if err != nil {
						providerFailure <- err
						providerWritten <- 0
						return
					}
					length := len(wire)
					if tc.truncate {
						length += 64
					}
					w.Header().Set("Content-Type", "text/event-stream")
					w.Header().Set("Content-Length", strconv.Itoa(length))
					n, err := io.WriteString(w, wire)
					providerWritten <- n
					providerFailure <- err
				}))
				t.Cleanup(server.Close)
				oldClient := client.HTTPClient
				client.HTTPClient = &http.Client{Transport: &bridgeLifecycleTransport{base: server.Client().Transport, eof: &rawEOF, truncated: &rawTruncated}, Timeout: 5 * time.Second}
				t.Cleanup(func() { client.HTTPClient = oldClient })
				c, rec, id := protocolContext(t, route.channel, route.actual, "/v1/responses", `{"model":"alias","input":"hello","max_output_tokens":1,"stream":true}`, server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
				require.False(t, supportsNativeResponseAPI(metalib.GetByContext(c)), "registered adapter must traverse the real Chat-to-Responses bridge")
				ctx, cancel := context.WithCancel(c.Request.Context())
				defer cancel()
				c.Request = c.Request.WithContext(ctx)
				writer := &bridgeLifecycleWriter{ResponseWriter: c.Writer, cancel: cancel, event: tc.cancelEvent, notify: make(chan bool)}
				c.Writer = writer
				apiErr := RelayResponseAPIHelper(c)
				drainCriticalTasks(t)
				require.EqualValues(t, 1, calls.Load(), "one genuine local provider dispatch")
				require.Equal(t, "/v1/chat/completions", <-providerPath)
				require.Equal(t, len(wire), <-providerWritten, "the full configured provider bytes were actually written")
				require.NoError(t, <-providerFailure)
				var sent map[string]any
				require.NoError(t, json.Unmarshal(<-providerBody, &sent))
				require.Equal(t, route.actual, sent["model"])
				if tc.cancelEvent != "" {
					require.True(t, writer.seen)
					require.Error(t, ctx.Err())
				}
				if strings.Contains(tc.name, "ordinary_eof") {
					require.True(t, rawEOF.Load(), "actual bounded HTTP body ended with ordinary EOF")
					require.False(t, rawTruncated.Load(), "ordinary EOF control must not borrow interrupted-body provenance")
				}
				if tc.truncate {
					require.True(t, rawTruncated.Load(), "negative transport control must observe actual unexpected EOF")
				}
				events := rec.Body.String()
				require.Contains(t, events, "event: response.created\n")
				sentinels := strings.Count(events, "data: [DONE]")
				completed := strings.Count(events, "event: response.completed\n")
				failed := strings.Count(events, "event: response.failed\n")
				var terminal map[string]any
				for _, frame := range strings.Split(events, "\n\n") {
					if !strings.HasPrefix(frame, "event: response.completed\n") && !strings.HasPrefix(frame, "event: response.failed\n") {
						continue
					}
					parts := strings.SplitN(frame, "\ndata: ", 2)
					require.Len(t, parts, 2)
					require.NoError(t, json.Unmarshal([]byte(parts[1]), &terminal))
				}
				require.NotNil(t, terminal)
				response, ok := terminal["response"].(map[string]any)
				require.True(t, ok)
				status := "completed"
				if tc.failure {
					status = "failed"
				}
				terminalUsage, ok := response["usage"].(map[string]any)
				require.True(t, ok, "terminal usage must come from actual adapter observations")
				charge := requestCostQuota(t, id)
				var token model.Token
				require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
				var logs []model.Log
				require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", id, model.LogTypeConsume).Find(&logs).Error)
				require.Len(t, logs, 1, "one final consume row")
				var costRows int64
				require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", id).Count(&costRows).Error)
				require.EqualValues(t, 1, costRows)
				t.Logf("BRIDGE_PROTOCOL case=%s expected=%d request=%d owner=%d token=%d log=%d complete=%d failed=%d terminal_status=%v terminal_usage=%v estimated=%v api_error=%v canceled=%v ordinary_eof=%v truncated=%v", tc.name, expected, charge, balance-reloadUserQuota(t), balance-token.RemainQuota, logs[0].Quota, completed, failed, response["status"], terminalUsage["total_tokens"], logs[0].Metadata["billing_estimated"], apiErr != nil, ctx.Err() != nil, rawEOF.Load(), rawTruncated.Load())
				if tc.failure {
					require.NotNil(t, apiErr)
					require.Zero(t, completed, "cancellation before upstream DONE or a real error must not fabricate success")
					require.Zero(t, sentinels, "failed streams must not synthesize a successful sentinel")
					require.Equal(t, 1, failed, "the real bridge emits exactly one failed terminal")
				} else {
					require.Nil(t, apiErr, "late downstream cancellation must preserve actual provider completion")
					require.Equal(t, 1, completed)
					require.Equal(t, 1, sentinels, "successful bridge emits its existing trailing sentinel once")
					require.Zero(t, failed)
					require.Contains(t, events, "event: response.output_text.done\n")
				}
				require.Equal(t, status, response["status"])
				require.EqualValues(t, expected, terminalUsage["total_tokens"])
				require.Equal(t, expected, charge, "independent receipt plus later-text oracle")
				require.Equal(t, expected, balance-reloadUserQuota(t))
				require.Equal(t, expected, balance-token.RemainQuota)
				require.EqualValues(t, expected, logs[0].Quota)
				require.Equal(t, estimated, logs[0].Metadata["billing_estimated"] == true)
				if estimated {
					require.NotEmpty(t, logs[0].Metadata["billing_estimate_reason"])
				}
				require.True(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
				if tc.failure {
					require.False(t, BillingAllowsRetry(c))
				}
			})
		}
	}
}
