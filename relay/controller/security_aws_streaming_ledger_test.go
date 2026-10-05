package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// awsSecurityFrame encodes the genuine ConverseStream SDK wire protocol.
func awsSecurityFrame(t *testing.T, event string, payload any) []byte {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	var headers eventstream.Headers
	headers.Set(":message-type", eventstream.StringValue("event"))
	headers.Set(":event-type", eventstream.StringValue(event))
	headers.Set(":content-type", eventstream.StringValue("application/json"))
	var out bytes.Buffer
	require.NoError(t, eventstream.NewEncoder().Encode(&out, eventstream.Message{Headers: headers, Payload: raw}))
	return out.Bytes()
}

// awsSecurityWriter models downstream close/write failures without replacing
// the actual AWS SDK client, controller admission or settlement machinery.
type awsSecurityWriter struct {
	gin.ResponseWriter
	notify         chan bool
	cancel         context.CancelFunc
	once           sync.Once
	mode           string
	idleNotify     chan struct{}
	firstStep      chan struct{}
	idleFlushCount int
}

// CloseNotify is the explicit Gin streaming lifecycle boundary.
func (w *awsSecurityWriter) CloseNotify() <-chan bool { return w.notify }

// Flush signals that the first real SDK event has been processed. The idle
// regression then closes the client while the next event is not forthcoming.
func (w *awsSecurityWriter) Flush() {
	w.ResponseWriter.Flush()
	w.idleFlushCount++
	if w.idleFlushCount == 2 {
		if w.firstStep != nil {
			close(w.firstStep)
		}
		if w.mode == "cancel_while_idle" {
			w.once.Do(func() { close(w.idleNotify) })
		}
	}
}

// Write triggers cancellation only after the selected real SDK event is delivered.
func (w *awsSecurityWriter) Write(p []byte) (int, error) {
	if w.mode == "write_failure" && strings.Contains(string(p), "data:") {
		return 0, io.ErrClosedPipe
	}
	n, err := w.ResponseWriter.Write(p)
	match := w.mode == "cancel_after_content" && strings.Contains(string(p), "partial_sdk") || w.mode == "cancel_after_metadata" && strings.Contains(string(p), `"total_tokens":18`)
	if match {
		w.once.Do(func() { w.cancel(); close(w.notify) })
	}
	return n, err
}

// WriteString uses the same observable client-delivery boundary.
func (w *awsSecurityWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// TestSecurityAWSStreamingReceiptLedger traverses actual SDK binary decoding,
// both relay entry points, finite/unlimited balances and one final consume row.
func TestSecurityAWSStreamingReceiptLedger(t *testing.T) {
	for _, actual := range []string{"deepseek-r1", "qwen3-coder-480b"} {
		for _, fallback := range []bool{false, true} {
			for _, scenario := range []string{"complete", "missing", "reasoning_only", "malformed", "sdk_exception", "cancel_before_content", "cancel_before_content_delayed", "cancel_while_idle", "cancel_after_content", "cancel_after_metadata", "write_failure", "large_delta", "overdraft_receipt", "known_admission", "unknown_500", "complete_unlimited", "missing_unlimited"} {
				t.Run(fmt.Sprintf("%s/fallback=%v/%s", actual, fallback, scenario), func(t *testing.T) {
					unlimited := strings.HasSuffix(scenario, "_unlimited")
					scenario = strings.TrimSuffix(scenario, "_unlimited")
					balance := int64(1_000_000)
					if scenario == "large_delta" || scenario == "overdraft_receipt" || scenario == "known_admission" || scenario == "unknown_500" {
						balance = 150
					}
					xaiVideoSetup(t, balance, unlimited)
					oldPre := config.PreConsumedQuota
					config.PreConsumedQuota = 20
					t.Cleanup(func() { config.PreConsumedQuota = oldPre })
					plain := "partial_sdk text"
					reason := ""
					if scenario == "missing" {
						plain = strings.Repeat("partial_sdk text ", 200)
					}
					if scenario == "reasoning_only" {
						plain = ""
						reason = strings.Repeat("private reasoning ", 200)
					}
					if scenario == "large_delta" {
						plain = strings.Repeat("partial_sdk token ", 500)
					}
					start := awsSecurityFrame(t, "messageStart", map[string]any{"role": "assistant"})
					data := append([]byte{}, start...)
					if plain != "" {
						data = append(data, awsSecurityFrame(t, "contentBlockDelta", map[string]any{"contentBlockIndex": 0, "delta": map[string]any{"text": plain}})...)
					}
					if reason != "" {
						data = append(data, awsSecurityFrame(t, "contentBlockDelta", map[string]any{"contentBlockIndex": 0, "delta": map[string]any{"reasoningContent": map[string]any{"text": reason}}})...)
					}
					if scenario == "cancel_before_content" || scenario == "cancel_before_content_delayed" || scenario == "cancel_while_idle" {
						data = start
					}
					measured := scenario == "complete" || scenario == "cancel_after_metadata" || scenario == "overdraft_receipt"
					if measured || scenario == "malformed" {
						data = append(data, awsSecurityFrame(t, "messageStop", map[string]any{"stopReason": "end_turn"})...)
						receipt := map[string]any{"inputTokens": 11, "outputTokens": 7, "totalTokens": 18}
						if scenario == "overdraft_receipt" {
							receipt = map[string]any{"inputTokens": 5, "outputTokens": 500, "totalTokens": 505}
						}
						if scenario == "malformed" {
							receipt = map[string]any{"inputTokens": -1, "outputTokens": 0, "totalTokens": -1}
						}
						data = append(data, awsSecurityFrame(t, "metadata", map[string]any{"usage": receipt})...)
					}
					if scenario == "sdk_exception" {
						var out bytes.Buffer
						var headers eventstream.Headers
						headers.Set(":message-type", eventstream.StringValue("exception"))
						headers.Set(":exception-type", eventstream.StringValue("internalServerException"))
						headers.Set(":content-type", eventstream.StringValue("application/json"))
						require.NoError(t, eventstream.NewEncoder().Encode(&out, eventstream.Message{Headers: headers, Payload: []byte(`{"message":"synthetic SDK failure"}`)}))
						data = append(data, out.Bytes()...)
					}
					var calls atomic.Int32
					paths := make(chan string, 1)
					notify := make(chan bool)
					idleNotify := make(chan struct{})
					firstStep := make(chan struct{})
					cancelDownstream := make(chan context.CancelFunc, 1)
					stopped := make(chan bool, 1)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						select {
						case paths <- r.URL.Path:
						default:
						}
						_, _ = io.Copy(io.Discard, r.Body)
						if scenario == "known_admission" {
							w.Header().Set("Content-Type", "application/json")
							w.Header().Set("X-Amzn-Errortype", "AccessDeniedException")
							w.WriteHeader(403)
							_, _ = io.WriteString(w, `{"message":"synthetic denied"}`)
							return
						}
						if scenario == "unknown_500" {
							w.Header().Set("Content-Type", "application/json")
							w.Header().Set("X-Amzn-Errortype", "InternalServerException")
							w.WriteHeader(500)
							_, _ = io.WriteString(w, `{"message":"synthetic unknown"}`)
							return
						}
						w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
						_, _ = w.Write(data)
						w.(http.Flusher).Flush()
						if scenario == "cancel_before_content" || scenario == "cancel_before_content_delayed" {
							if scenario == "cancel_before_content_delayed" {
								select {
								case <-firstStep:
								case <-time.After(time.Second):
									stopped <- false
									return
								}
								timer := time.NewTimer(100 * time.Millisecond)
								<-timer.C
							}
							// Physical downstream disconnect and notification-only idle
							// cancellation are independent regression contracts.
							(<-cancelDownstream)()
							close(notify)
						}
						if scenario == "cancel_while_idle" {
							select {
							case <-idleNotify:
							case <-time.After(time.Second):
								stopped <- false
								return
							}
							// Give Gin time to enter the next callback. Unlike the
							// original immediately-closed notification, this targets
							// an already idle SDK receive. The provider deadline and
							// actual closure assertion are unchanged.
							time.Sleep(50 * time.Millisecond)
							close(notify)
						}
						if strings.HasPrefix(scenario, "cancel_") && !(fallback && scenario == "cancel_after_metadata") {
							select {
							case <-r.Context().Done():
								stopped <- true
							case <-time.After(3 * time.Second):
								stopped <- false
							}
						}
					}))
					t.Cleanup(server.Close)
					t.Setenv("AWS_ENDPOINT_URL", server.URL)
					t.Setenv("AWS_MAX_ATTEMPTS", "1")
					path, body := "/v1/chat/completions", `{"model":"alias","messages":[{"role":"user","content":"hello"}],"max_tokens":1,"stream":true}`
					if fallback {
						path, body = "/v1/responses", `{"model":"alias","input":"hello","max_output_tokens":1,"stream":true}`
					}
					c, w, id := protocolContext(t, channeltype.AwsClaude, actual, path, body, server.URL, balance, 1, unlimited, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
					c.Set(ctxkey.Config, model.ChannelConfig{Region: "us-east-1", AK: "synthetic-access", SK: "synthetic-secret"})
					ctx, cancel := context.WithCancel(c.Request.Context())
					defer cancel()
					c.Request = c.Request.WithContext(ctx)
					cancelDownstream <- cancel
					c.Writer = &awsSecurityWriter{ResponseWriter: c.Writer, notify: notify, cancel: cancel, mode: scenario, idleNotify: idleNotify, firstStep: firstStep}
					var apiErr *relaymodel.ErrorWithStatusCode
					if fallback {
						apiErr = RelayResponseAPIHelper(c)
					} else {
						apiErr = RelayTextHelper(c)
					}
					drainCriticalTasks(t)
					require.EqualValues(t, 1, calls.Load(), "one real SDK invocation, not a replay of uncertain paid work")
					require.True(t, strings.HasSuffix(<-paths, "/converse-stream"))
					if strings.HasPrefix(scenario, "cancel_") && !(fallback && scenario == "cancel_after_metadata") {
						require.True(t, <-stopped, "the provider must observe closure instead of reaching a fixture timeout")
					}
					if fallback && scenario == "cancel_after_metadata" {
						require.Error(t, ctx.Err(), "the client cancels after the final Responses receipt is rendered; the SDK has already reached EOF")
					}
					meta := metalib.GetByContext(c)
					require.NotNil(t, meta)
					quote := int64(meta.PromptTokens) + 21
					expected := int64(meta.PromptTokens + openai.CountTokenText(plain, actual) + openai.CountTokenText(reason, actual))
					if scenario == "cancel_before_content" || scenario == "cancel_before_content_delayed" || scenario == "cancel_while_idle" || scenario == "unknown_500" {
						expected = int64(meta.PromptTokens)
					}
					expected = max(expected, quote)
					if measured {
						expected = 18
					}
					if scenario == "overdraft_receipt" {
						expected = 505
					}
					if scenario == "known_admission" {
						expected = 0
					}
					charge := requestCostQuota(t, id)
					if charge != expected {
						t.Logf("REPRODUCED_467_SDK_USAGE_LOSS actual=%d expected=%d held=%d", charge, expected, c.GetInt64(ctxkey.PreConsumedQuotaAmount))
					}
					require.Equal(t, expected, charge)
					require.Equal(t, balance-charge, reloadUserQuota(t))
					var token model.Token
					require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
					tokenBalance := balance - charge
					if unlimited {
						tokenBalance = balance
					}
					require.Equal(t, tokenBalance, token.RemainQuota)
					var logs []model.Log
					require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", id, model.LogTypeConsume).Find(&logs).Error)
					require.Len(t, logs, 1)
					require.EqualValues(t, charge, logs[0].Quota)
					if scenario == "complete" {
						require.Nil(t, apiErr)
						require.Contains(t, w.Body.String(), "[DONE]")
					} else if !(fallback && scenario == "cancel_after_metadata") {
						require.NotNil(t, apiErr)
					}
					if expected > 0 && !measured {
						require.Equal(t, true, logs[0].Metadata["billing_estimated"])
						require.NotContains(t, w.Body.String(), "[DONE]")
					}
					if expected > 0 {
						require.True(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
						require.False(t, BillingAllowsRetry(c))
					}
				})
			}
		}
	}
}
