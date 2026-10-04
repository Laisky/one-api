package controller

import (
	"bytes"
	"context"
	"fmt"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// awsDelayedCloseNotifyWriter observes Gin flushing its first decoded SDK event.
type awsDelayedCloseNotifyWriter struct {
	*awsSecurityWriter
	firstStep chan struct{}
	flushes   int
}

// Flush skips the header-only flush and reports Gin completing the SDK messageStart step.
func (w *awsDelayedCloseNotifyWriter) Flush() {
	w.ResponseWriter.Flush()
	w.flushes++
	// SetEventStreamHeaders flushes headers once before c.Stream starts.
	// The next flush follows the decoded messageStart step, which renders no chunk.
	if w.flushes == 2 {
		close(w.firstStep)
	}
}

// TestSecurityAWSStreamingDelayedCloseNotify traverses actual SDK binary decoding,
// both relay entry points, finite/unlimited balances and one final consume row.
func TestSecurityAWSStreamingDelayedCloseNotify(t *testing.T) {
	for _, actual := range []string{"qwen3-coder-480b"} {
		for _, fallback := range []bool{true} {
			for _, scenario := range []string{"cancel_before_content"} {
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
					if scenario == "cancel_before_content" {
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
					firstStep := make(chan struct{})
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
						if scenario == "cancel_before_content" {
							select {
							case <-firstStep:
							case <-time.After(time.Second):
								stopped <- false
								return
							}
							// The first SDK event has reached Gin. Give its next step a bounded
							// opportunity to wait for content before notifying a client disconnect.
							timer := time.NewTimer(100 * time.Millisecond)
							<-timer.C
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
					c.Writer = &awsDelayedCloseNotifyWriter{awsSecurityWriter: &awsSecurityWriter{ResponseWriter: c.Writer, notify: notify, cancel: cancel, mode: scenario}, firstStep: firstStep}
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
					if scenario == "cancel_before_content" || scenario == "unknown_500" {
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
