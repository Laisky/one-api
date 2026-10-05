package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/stretchr/testify/require"
)

// TestResponsesBridgeReceiptCoverageHTTP verifies real registered adapters, transport,
// terminal Responses events, receipt chronology and exactly one physical settlement.
func TestResponsesBridgeReceiptCoverageHTTP(t *testing.T) {
	for _, route := range []struct {
		name, actual string
		channel      int
	}{
		{"native_adapter", "gpt-4", channeltype.OpenAICompatible},
		{"shared_adapter", "deepseek-chat", channeltype.DeepSeek},
	} {
		for _, tc := range []struct {
			name, cancelEvent, receiptKind string
			later, done, failure, truncate bool
		}{
			{name: "no_receipt_canceled_ordinary_eof", receiptKind: "none", later: true, failure: true, cancelEvent: "response.output_text.delta"},
			{name: "no_receipt_truncated_transport", receiptKind: "none", later: true, failure: true, truncate: true},
			{name: "partial_receipt_canceled_ordinary_eof", receiptKind: "partial", later: true, failure: true, cancelEvent: "response.output_text.delta"},
			{name: "partial_receipt_truncated_transport", receiptKind: "partial", later: true, failure: true, truncate: true},
			{name: "partial_same_frame_truncated", receiptKind: "partial_same_frame", later: true, failure: true, truncate: true},
			{name: "measured_done_control", done: true},
			{name: "measured_cancel_control", failure: true, cancelEvent: "response.created"},
			{name: "omitted_total_done", receiptKind: "omitted_total", done: true},
			{name: "zero_total_done", receiptKind: "zero_total", done: true},
			{name: "cached_top_level_done", receiptKind: "cached", done: true},
			{name: "deepseek_cache_hit_done", receiptKind: "deepseek_cached", done: true},
			{name: "cache_write_top_level_done", receiptKind: "write", done: true},
			{name: "top_level_combined_cache_done", receiptKind: "combined", done: true},
			{name: "nested_cache_buckets_control", receiptKind: "nested", done: true},
		} {
			t.Run(route.name+"/"+tc.name, func(t *testing.T) {
				balance := int64(1_000_000)
				xaiVideoSetup(t, balance, false)
				oldApprox := config.ApproximateTokenEnabled
				config.ApproximateTokenEnabled = true
				t.Cleanup(func() { config.ApproximateTokenEnabled = oldApprox })
				oldPre := config.PreConsumedQuota
				config.PreConsumedQuota = 20
				t.Cleanup(func() { config.PreConsumedQuota = oldPre })
				later := ""
				if tc.later {
					later = strings.Repeat("receipt gap output ", 20)
				}
				// With approximate tokenization, the known hello/user request costs
				// 3 message framing + 1 hello + 1 user + 3 assistant framing = 8.
				// Count the provider output independently from tracker/finalization.
				input, output := int64(11), int64(7)
				estimated := tc.later
				rawUsage := `{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}`
				cached, written := int64(0), int64(0)
				switch tc.receiptKind {
				case "none":
					input, output = 8, int64(len(later)*38/100)
					rawUsage = ""
				case "partial", "partial_same_frame":
					input, output = 11, int64(len(later)*38/100)
					rawUsage = `{"prompt_tokens":11}`
				case "omitted_total":
					rawUsage = `{"prompt_tokens":11,"completion_tokens":7}`
				case "zero_total":
					rawUsage = `{"prompt_tokens":11,"completion_tokens":7,"total_tokens":0}`
				case "cached":
					cached = 4
					rawUsage = `{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18,"cached_tokens":4}`
				case "deepseek_cached":
					cached = 4
					rawUsage = `{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18,"prompt_cache_hit_tokens":4,"prompt_cache_miss_tokens":7}`
				case "write":
					written = 4
					rawUsage = `{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18,"cache_write_tokens":4}`
				case "combined":
					cached, written = 4, 4
					rawUsage = `{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18,"cached_tokens":4,"cache_write_tokens":4}`
				case "nested":
					cached, written = 4, 4
					rawUsage = `{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18,"prompt_tokens_details":{"cached_tokens":4},"cache_write_5m_tokens":4}`
				}
				total := input + output
				// Distinct configured prices: ordinary input/output 1, cache hit
				// 1/4 and cache write 2, with writes included in input tokens.
				expected := input - cached - written + cached/4 + 2*written + output
				receipt := ""
				if rawUsage != "" {
					receipt = "data: {\"choices\":[],\"usage\":" + rawUsage + "}\n\n"
				}
				wire := receipt
				if tc.later {
					raw, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": later}}}})
					require.NoError(t, err)
					if tc.receiptKind == "partial_same_frame" {
						raw = []byte(strings.TrimSuffix(string(raw), "}") + `,"usage":{"prompt_tokens":11}}`)
						wire = ""
					}
					wire += "data: " + string(raw) + "\n\n"
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
				c, rec, id := protocolContext(t, route.channel, route.actual, "/v1/responses", `{"model":"alias","input":"hello","max_output_tokens":1,"stream":true}`, server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1, CachedInputRatio: 0.25, CacheWrite5mRatio: 2, CacheWrite1hRatio: 3})
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
				terminalUsagePresent := ok
				charge := requestCostQuota(t, id)
				var token model.Token
				require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
				var logs []model.Log
				require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", id, model.LogTypeConsume).Find(&logs).Error)
				require.Len(t, logs, 1, "one final consume row")
				var costRows int64
				require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", id).Count(&costRows).Error)
				require.EqualValues(t, 1, costRows)
				var apiCode any
				if apiErr != nil {
					apiCode = apiErr.Error.Code
				}
				t.Logf("BRIDGE_PROTOCOL case=%s expected=%d expected_total=%d request=%d owner=%d token=%d log=%d complete=%d failed=%d terminal_status=%v terminal_usage=%v cached_log=%d input_log=%d output_log=%d log_meta=%v estimated=%v api_error=%v canceled=%v ordinary_eof=%v truncated=%v api_code=%v terminal_error=%v", tc.name, expected, total, charge, balance-reloadUserQuota(t), balance-token.RemainQuota, logs[0].Quota, completed, failed, response["status"], terminalUsage, logs[0].CachedPromptTokens, logs[0].PromptTokens, logs[0].CompletionTokens, logs[0].Metadata, logs[0].Metadata["billing_estimated"], apiErr != nil, ctx.Err() != nil, rawEOF.Load(), rawTruncated.Load(), apiCode, response["error"])
				if tc.failure {
					require.NotNil(t, apiErr)
					require.Equal(t, "read_stream_failed", apiErr.Error.Code)
					terminalError, ok := response["error"].(map[string]any)
					require.True(t, ok)
					require.Equal(t, "read_stream_failed", terminalError["code"])
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
				require.Equal(t, expected, charge, "independent raw receipt/tokenizer and cache-price oracle")
				require.Equal(t, expected, balance-reloadUserQuota(t))
				require.Equal(t, expected, balance-token.RemainQuota)
				require.EqualValues(t, expected, logs[0].Quota)
				require.EqualValues(t, input, logs[0].PromptTokens)
				require.EqualValues(t, output, logs[0].CompletionTokens)
				require.EqualValues(t, cached, logs[0].CachedPromptTokens)
				require.True(t, terminalUsagePresent, "terminal usage must come from actual adapter observations")
				require.EqualValues(t, input, terminalUsage["input_tokens"])
				require.EqualValues(t, output, terminalUsage["output_tokens"])
				require.EqualValues(t, total, terminalUsage["total_tokens"])
				if written > 0 {
					require.EqualValues(t, written, terminalUsage["cache_write_tokens"])
				}
				if cached > 0 {
					details, ok := terminalUsage["input_tokens_details"].(map[string]any)
					require.True(t, ok)
					require.EqualValues(t, cached, details["cached_tokens"])
				}

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
