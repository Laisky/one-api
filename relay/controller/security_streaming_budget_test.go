package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/streaming"
)

// TestSecurityStreamFundsEveryObservation verifies real atomic debits, not a
// cached affordability mock, independently of the configured flush interval.
func TestSecurityStreamFundsEveryObservation(t *testing.T) {
	for _, tokenLimited := range []bool{false, true} {
		t.Run(fmt.Sprint(tokenLimited), func(t *testing.T) {
			balance := int64(20)
			if tokenLimited {
				balance = 1000
			}
			xaiVideoSetup(t, balance, false)
			if tokenLimited {
				require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fallbackTokenID).Update("remain_quota", 20).Error)
			}
			tracker := streaming.NewQuotaTracker(streaming.QuotaTrackerParams{UserID: fallbackUserID, TokenID: fallbackTokenID, ChannelID: fallbackChannelID, ModelName: "gpt-4", ModelRatio: 1, GroupRatio: 1, FlushInterval: time.Hour, Ctx: context.Background(), ChannelModelConfigs: map[string]model.ModelConfigLocal{"gpt-4": {Ratio: 1, CompletionRatio: 1}}})
			require.NoError(t, tracker.RecordCompletionTokens(5))
			err := tracker.RecordCompletionTokens(30)
			if err == nil {
				t.Log("REPRODUCED_487_BATCH_WINDOW_UNFUNDED_OUTPUT")
			}
			require.Error(t, err, "each observation beyond funded credit must stop immediately, not after the batch timer")
			require.EqualValues(t, 5, tracker.ChargedQuota())
			usage, charged, err := tracker.Finalize(nil)
			require.Error(t, err)
			require.EqualValues(t, 5, charged)
			require.Equal(t, 35, usage.CompletionTokens)
			// Enforcement errors must not erase the already-observed failing delta.
			meta := &metalib.Meta{UserId: fallbackUserID, TokenId: fallbackTokenID, ChannelId: fallbackChannelID, ActualModelName: "gpt-4", OriginModelName: "gpt-4"}
			quota := postConsumeQuota(context.Background(), usage, meta, &relaymodel.GeneralOpenAIRequest{Model: "gpt-4"}, 1, 0, charged, 1, nil, 1, false, map[string]model.ModelConfigLocal{"gpt-4": {Ratio: 1, CompletionRatio: 1}}, nil)
			require.EqualValues(t, 35, quota)
			require.Equal(t, balance-35, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.EqualValues(t, -15, token.RemainQuota)
		})
	}
}

// TestSecurityStreamEstimatedReceiptCannotEraseObservedWork preserves an owned
// snapshot while retaining authoritative measured-receipt positive controls.
func TestSecurityStreamEstimatedReceiptCannotEraseObservedWork(t *testing.T) {
	for _, estimated := range []bool{true, false} {
		t.Run(fmt.Sprint(estimated), func(t *testing.T) {
			xaiVideoSetup(t, 1000, false)
			tracker := streaming.NewQuotaTracker(streaming.QuotaTrackerParams{UserID: fallbackUserID, TokenID: fallbackTokenID, PromptTokens: 3, ModelName: "gpt-4", ModelRatio: 1, GroupRatio: 1, PreConsumedQuota: 1000, ChannelModelConfigs: map[string]model.ModelConfigLocal{"gpt-4": {Ratio: 1, CompletionRatio: 1}}})
			require.NoError(t, tracker.RecordCompletionTokens(30))
			receipt := &relaymodel.Usage{PromptTokens: 3, CompletionTokens: 0, TotalTokens: 3}
			if estimated {
				receipt.BillingEstimateReason = "partial_receipt"
			}
			tracker.UpdateFinalUsage(receipt)
			receipt.CompletionTokens = 9999 // The caller cannot mutate a stored receipt.
			usage, _, err := tracker.Finalize(nil)
			require.NoError(t, err)
			expected := 0
			if estimated {
				expected = 30
			}
			require.Equal(t, expected, usage.CompletionTokens)
			require.NoError(t, tracker.RecordCompletionTokens(2))
			usage, _, err = tracker.Finalize(nil)
			require.NoError(t, err)
			require.Equal(t, expected+2, usage.CompletionTokens)
		})
	}
}

// streamBudgetWriter provides explicit CloseNotify semantics for real Gin
// streaming fixtures and acknowledges only delivered content to the provider.
type streamBudgetWriter struct {
	gin.ResponseWriter
	notify chan bool
	ack    chan struct{}
	fail   bool
}

// CloseNotify returns the synthetic downstream's lifecycle signal.
func (w *streamBudgetWriter) CloseNotify() <-chan bool { return w.notify }

// Write observes actual delivered content rather than an internal tracker call.
func (w *streamBudgetWriter) Write(p []byte) (int, error) {
	if w.fail && strings.Contains(string(p), "data:") {
		return 0, io.ErrClosedPipe
	}
	n, err := w.ResponseWriter.Write(p)
	if err == nil && strings.Contains(string(p), "tick ") {
		select {
		case w.ack <- struct{}{}:
		default:
		}
	}
	return n, err
}

// WriteString shares the exact same delivery boundary.
func (w *streamBudgetWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// TestSecurityStreamReceiptSettlesAfterExhaustion reaches the ordinary finalizer
// with an authoritative receipt larger than the owner's remaining balance.
func TestSecurityStreamReceiptSettlesAfterExhaustion(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(fmt.Sprint(fallback), func(t *testing.T) {
			const balance = int64(150)
			xaiVideoSetup(t, balance, false)
			oldPre := config.PreConsumedQuota
			config.PreConsumedQuota = 20
			t.Cleanup(func() { config.PreConsumedQuota = oldPre })
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":500,\"total_tokens\":505}}\n\ndata: [DONE]\n\n")
			}))
			t.Cleanup(server.Close)
			old := client.HTTPClient
			client.HTTPClient = server.Client()
			t.Cleanup(func() { client.HTTPClient = old })
			path, body := "/v1/chat/completions", `{"model":"alias","messages":[{"role":"user","content":"hello"}],"max_tokens":1,"stream":true}`
			if fallback {
				path, body = "/v1/responses", `{"model":"alias","input":"hello","max_output_tokens":1,"stream":true}`
			}
			c, _, id := protocolContext(t, channeltype.OpenAICompatible, "gpt-4", path, body, server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			var apiErr *relaymodel.ErrorWithStatusCode
			if fallback {
				apiErr = RelayResponseAPIHelper(c)
			} else {
				apiErr = RelayTextHelper(c)
			}
			drainCriticalTasks(t)
			require.EqualValues(t, 1, calls.Load())
			actual := requestCostQuota(t, id)
			if actual != 505 {
				t.Logf("REPRODUCED_495_RECEIPT_NOT_SETTLED actual=%d", actual)
			}
			require.EqualValues(t, 505, actual)
			require.Equal(t, balance-505, reloadUserQuota(t))
			require.NotNil(t, apiErr)
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance-505, token.RemainQuota)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", id, model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			require.EqualValues(t, 505, logs[0].Quota)
		})
	}
}

// TestSecurityFallbackRapidStreamBudget uses actual HTTP cancellation and a
// delivery handshake, so fast small chunks cannot hide behind a debit timer.
func TestSecurityFallbackRapidStreamBudget(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(fmt.Sprint(fallback), func(t *testing.T) {
			const balance = int64(150)
			xaiVideoSetup(t, balance, false)
			oldPre, oldInterval := config.PreConsumedQuota, config.StreamingBillingIntervalSec
			config.PreConsumedQuota = 20
			config.StreamingBillingIntervalSec = 3600
			t.Cleanup(func() { config.PreConsumedQuota = oldPre; config.StreamingBillingIntervalSec = oldInterval })
			ack := make(chan struct{}, 1)
			stopped := make(chan bool, 1)
			var sent atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				for i := 0; i < 200; i++ {
					chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "tick "}}}})
					if _, err := fmt.Fprintf(w, "data: %s\n\n", chunk); err != nil {
						stopped <- true
						return
					}
					w.(http.Flusher).Flush()
					sent.Add(1)
					select {
					case <-ack:
					case <-r.Context().Done():
						stopped <- true
						return
					case <-time.After(2 * time.Second):
						stopped <- false
						return
					}
				}
				_, _ = io.WriteString(w, "data: [DONE]\n\n")
				stopped <- false
			}))
			t.Cleanup(server.Close)
			old := client.HTTPClient
			client.HTTPClient = server.Client()
			t.Cleanup(func() { client.HTTPClient = old })
			path, body := "/v1/chat/completions", `{"model":"alias","messages":[{"role":"user","content":"hello"}],"max_tokens":1,"stream":true}`
			if fallback {
				path, body = "/v1/responses", `{"model":"alias","input":"hello","max_output_tokens":1,"stream":true}`
			}
			c, w, id := protocolContext(t, channeltype.OpenAICompatible, "gpt-4", path, body, server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			c.Writer = &streamBudgetWriter{ResponseWriter: c.Writer, notify: make(chan bool), ack: ack}
			var apiErr *relaymodel.ErrorWithStatusCode
			if fallback {
				apiErr = RelayResponseAPIHelper(c)
			} else {
				apiErr = RelayTextHelper(c)
			}
			drainCriticalTasks(t)
			ended := <-stopped
			if !ended {
				t.Logf("REPRODUCED_487_UNBOUNDED_RAPID_STREAM sent=%d", sent.Load())
			}
			require.True(t, ended, "the real provider must observe cancellation before finishing all chunks")
			require.NotNil(t, apiErr)
			if fallback {
				require.NotContains(t, w.Body.String(), "event: response.completed")
				require.Contains(t, w.Body.String(), "event: response.failed")
			}
			require.Less(t, sent.Load(), int32(200))
			require.Less(t, securityDeliveredTicks(t, w.Body.String(), fallback), 200)
			charge := requestCostQuota(t, id)
			require.GreaterOrEqual(t, charge, balance, "the last consumed over-budget chunk still settles")
			require.Equal(t, balance-charge, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance-charge, token.RemainQuota)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", id, model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			require.EqualValues(t, charge, logs[0].Quota)
		})
	}
}

// TestSecuritySameFrameReceiptSettlesAfterExhaustion reaches the ordinary finalizer
// with an authoritative receipt larger than the owner's remaining balance.
func TestSecuritySameFrameReceiptSettlesAfterExhaustion(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(fmt.Sprint(fallback), func(t *testing.T) {
			const balance = int64(150)
			xaiVideoSetup(t, balance, false)
			oldPre := config.PreConsumedQuota
			config.PreConsumedQuota = 20
			t.Cleanup(func() { config.PreConsumedQuota = oldPre })
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated already_generated \"}}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":500,\"total_tokens\":505}}\n\ndata: [DONE]\n\n")
			}))
			t.Cleanup(server.Close)
			old := client.HTTPClient
			client.HTTPClient = server.Client()
			t.Cleanup(func() { client.HTTPClient = old })
			path, body := "/v1/chat/completions", `{"model":"alias","messages":[{"role":"user","content":"hello"}],"max_tokens":1,"stream":true}`
			if fallback {
				path, body = "/v1/responses", `{"model":"alias","input":"hello","max_output_tokens":1,"stream":true}`
			}
			c, _, id := protocolContext(t, channeltype.OpenAICompatible, "gpt-4", path, body, server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			var apiErr *relaymodel.ErrorWithStatusCode
			if fallback {
				apiErr = RelayResponseAPIHelper(c)
			} else {
				apiErr = RelayTextHelper(c)
			}
			drainCriticalTasks(t)
			require.EqualValues(t, 1, calls.Load())
			actual := requestCostQuota(t, id)
			if actual != 505 {
				t.Logf("REPRODUCED_495_RECEIPT_NOT_SETTLED actual=%d", actual)
			}
			require.EqualValues(t, 505, actual)
			require.Equal(t, balance-505, reloadUserQuota(t))
			require.NotNil(t, apiErr)
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance-505, token.RemainQuota)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", id, model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			require.EqualValues(t, 505, logs[0].Quota)
		})
	}
}

// securityDeliveredTicks measures delivered delta content, excluding cumulative snapshots.
func securityDeliveredTicks(t *testing.T, wire string, fallback bool) int {
	t.Helper()
	count := 0
	for _, line := range strings.Split(wire, "\n") {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			continue
		}
		if fallback {
			var event struct {
				Type  string `json:"type"`
				Delta string `json:"delta"`
			}
			require.NoError(t, json.Unmarshal([]byte(payload), &event))
			if event.Type == "response.output_text.delta" {
				count += strings.Count(event.Delta, "tick ")
			}
		} else {
			var event struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			require.NoError(t, json.Unmarshal([]byte(payload), &event))
			for _, choice := range event.Choices {
				count += strings.Count(choice.Delta.Content, "tick ")
			}
		}
	}
	return count
}

// TestSecuritySharedProviderRapidBudget uses actual HTTP cancellation and a
// delivery handshake, so fast small chunks cannot hide behind a debit timer.
func TestSecuritySharedProviderRapidBudget(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(fmt.Sprint(fallback), func(t *testing.T) {
			const balance = int64(150)
			xaiVideoSetup(t, balance, false)
			oldPre, oldInterval := config.PreConsumedQuota, config.StreamingBillingIntervalSec
			config.PreConsumedQuota = 20
			config.StreamingBillingIntervalSec = 3600
			t.Cleanup(func() { config.PreConsumedQuota = oldPre; config.StreamingBillingIntervalSec = oldInterval })
			ack := make(chan struct{}, 1)
			stopped := make(chan bool, 1)
			var sent atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				for i := 0; i < 200; i++ {
					chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "tick "}}}})
					if _, err := fmt.Fprintf(w, "data: %s\n\n", chunk); err != nil {
						stopped <- true
						return
					}
					w.(http.Flusher).Flush()
					sent.Add(1)
					select {
					case <-ack:
					case <-r.Context().Done():
						stopped <- true
						return
					case <-time.After(2 * time.Second):
						stopped <- false
						return
					}
				}
				_, _ = io.WriteString(w, "data: [DONE]\n\n")
				stopped <- false
			}))
			t.Cleanup(server.Close)
			old := client.HTTPClient
			client.HTTPClient = server.Client()
			t.Cleanup(func() { client.HTTPClient = old })
			path, body := "/v1/chat/completions", `{"model":"alias","messages":[{"role":"user","content":"hello"}],"max_tokens":1,"stream":true}`
			if fallback {
				path, body = "/v1/responses", `{"model":"alias","input":"hello","max_output_tokens":1,"stream":true}`
			}
			c, w, id := protocolContext(t, channeltype.DeepSeek, "deepseek-chat", path, body, server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			c.Writer = &streamBudgetWriter{ResponseWriter: c.Writer, notify: make(chan bool), ack: ack}
			var apiErr *relaymodel.ErrorWithStatusCode
			if fallback {
				apiErr = RelayResponseAPIHelper(c)
			} else {
				apiErr = RelayTextHelper(c)
			}
			drainCriticalTasks(t)
			ended := <-stopped
			if !ended {
				t.Logf("REPRODUCED_487_UNBOUNDED_RAPID_STREAM sent=%d", sent.Load())
			}
			require.True(t, ended, "the real provider must observe cancellation before finishing all chunks")
			require.NotNil(t, apiErr)
			if fallback {
				require.NotContains(t, w.Body.String(), "event: response.completed")
				require.Contains(t, w.Body.String(), "event: response.failed")
			}
			require.Less(t, sent.Load(), int32(200))
			require.Less(t, securityDeliveredTicks(t, w.Body.String(), fallback), 200)
			charge := requestCostQuota(t, id)
			require.GreaterOrEqual(t, charge, balance, "the last consumed over-budget chunk still settles")
			require.Equal(t, balance-charge, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance-charge, token.RemainQuota)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", id, model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			require.EqualValues(t, charge, logs[0].Quota)
		})
	}
}
