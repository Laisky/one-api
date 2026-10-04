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
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// receiptFreshnessCancelWriter cancels only after the real handler delivers the later visible delta.
type receiptFreshnessCancelWriter struct {
	gin.ResponseWriter
	cancel context.CancelFunc
	seen   bool
}

// Write observes delivered post-receipt text independently of internal quota bookkeeping.
func (w *receiptFreshnessCancelWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if err == nil && strings.Contains(string(p), "freshness output ") {
		w.seen = true
		w.cancel()
	}
	return n, err
}

// WriteString preserves the same cancellation boundary for native rendering.
func (w *receiptFreshnessCancelWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// TestNativeStreamReceiptFreshnessHTTP checks actual provider HTTP, receipt chronology and all settlement ledgers.
func TestNativeStreamReceiptFreshnessHTTP(t *testing.T) {
	for _, route := range []struct {
		name     string
		channel  int
		model    string
		fallback bool
	}{{"native/chat", channeltype.OpenAICompatible, "gpt-4", false}, {"native/responses", channeltype.OpenAICompatible, "gpt-4", true}, {"shared/chat", channeltype.DeepSeek, "deepseek-chat", false}, {"shared/responses", channeltype.DeepSeek, "deepseek-chat", true}} {
		for _, exit := range []string{"eof", "reset", "cancel", "quota_abort", "fresh_receipt", "fresh_final_receipt"} {
			t.Run(route.name+"/"+exit, func(t *testing.T) {
				balance := int64(10000)
				// Oversized synthetic output exercises defensive settlement, not a claim that real providers ignore output caps.
				maxTokens := 1
				if exit == "quota_abort" {
					balance, maxTokens = 150, 1
				}
				xaiVideoSetup(t, balance, false)
				oldPre, oldInterval := config.PreConsumedQuota, config.StreamingBillingIntervalSec
				config.PreConsumedQuota, config.StreamingBillingIntervalSec = 20, 3600
				t.Cleanup(func() { config.PreConsumedQuota, config.StreamingBillingIntervalSec = oldPre, oldInterval })
				text := strings.Repeat("freshness output ", 100)
				if exit == "fresh_receipt" {
					text = ""
				}
				// The oracle counts the known later provider text, without consulting tracker/estimator snapshots.
				floor := int64(18 + openai.CountTokenText(text, route.model))
				var calls atomic.Int32
				providerBody := make(chan []byte, 1)
				providerPath := make(chan string, 1)
				providerStopped := make(chan bool, 1)
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					body, err := io.ReadAll(io.LimitReader(r.Body, 16<<10))
					if err != nil {
						providerStopped <- false
						return
					}
					providerBody <- body
					providerPath <- r.URL.Path
					w.Header().Set("Content-Type", "text/event-stream")
					if exit == "reset" {
						// A declared length longer than the bounded frames produces a real HTTP unexpected EOF.
						w.Header().Set("Content-Length", "100000")
					}
					_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":7,\"total_tokens\":18}}\n\n")
					if text != "" {
						event, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": text}}}})
						if err != nil {
							providerStopped <- false
							return
						}
						_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
					}
					w.(http.Flusher).Flush()
					if exit == "fresh_final_receipt" {
						// A later measured receipt is authoritative even when the local visible-text estimate was larger.
						_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":7,\"total_tokens\":18}}\n\n")
					}
					switch exit {
					case "reset":
						providerStopped <- true
						return
					case "cancel", "quota_abort":
						select {
						case <-r.Context().Done():
							providerStopped <- true
						case <-time.After(5 * time.Second):
							providerStopped <- false
						}
					default:
						_, _ = io.WriteString(w, "data: [DONE]\n\n")
						providerStopped <- true
					}
				}))
				t.Cleanup(server.Close)
				oldClient := client.HTTPClient
				client.HTTPClient = server.Client()
				client.HTTPClient.Timeout = 6 * time.Second
				t.Cleanup(func() { client.HTTPClient = oldClient })
				request := fmt.Sprintf(`{"model":"alias","messages":[{"role":"user","content":"hello"}],"max_tokens":%d,"stream":true}`, maxTokens)
				path := "/v1/chat/completions"
				if route.fallback {
					path = "/v1/responses"
					request = fmt.Sprintf(`{"model":"alias","input":"hello","max_output_tokens":%d,"stream":true}`, maxTokens)
				}
				c, downstream, id := protocolContext(t, route.channel, route.model, path, request, server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
				if route.fallback {
					require.False(t, supportsNativeResponseAPI(metalib.GetByContext(c)), "fixture must use real Chat-to-Responses fallback")
				}
				ctx, cancel := context.WithCancel(c.Request.Context())
				defer cancel()
				c.Request = c.Request.WithContext(ctx)
				cw := &receiptFreshnessCancelWriter{ResponseWriter: c.Writer, cancel: cancel}
				if exit == "cancel" {
					c.Writer = cw
				}
				var apiErr *relaymodel.ErrorWithStatusCode
				if route.fallback {
					apiErr = RelayResponseAPIHelper(c)
				} else {
					apiErr = RelayTextHelper(c)
				}
				drainCriticalTasks(t)
				require.EqualValues(t, 1, calls.Load())
				require.Equal(t, "/v1/chat/completions", <-providerPath, "verify actual native Chat provider path")
				select {
				case stopped := <-providerStopped:
					require.True(t, stopped, "real upstream must finish or observe cancellation within the bound")
				case <-time.After(6 * time.Second):
					t.Fatal("fake provider lifecycle not collected")
				}
				var captured map[string]any
				require.NoError(t, json.Unmarshal(<-providerBody, &captured))
				require.Equal(t, route.model, captured["model"])
				if exit == "cancel" {
					require.True(t, cw.seen, "cancel only after later output is delivered")
				}
				if exit != "quota_abort" && text != "" {
					require.Contains(t, downstream.Body.String(), text)
				}
				charge := requestCostQuota(t, id)
				ownerCharge := balance - reloadUserQuota(t)
				var token model.Token
				require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
				var logs []model.Log
				require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", id, model.LogTypeConsume).Find(&logs).Error)
				require.Len(t, logs, 1, "settlement consumes exactly once")
				var rows int64
				require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", id).Count(&rows).Error)
				require.EqualValues(t, 1, rows)
				t.Logf("RECEIPT_FRESHNESS route=%s exit=%s later_tokens=%d floor=%d owner=%d token=%d request=%d log=%d estimated=%v reason=%v api_error=%v", route.name, exit, floor-18, floor, ownerCharge, balance-token.RemainQuota, charge, logs[0].Quota, logs[0].Metadata["billing_estimated"], logs[0].Metadata["billing_estimate_reason"], apiErr != nil)
				require.Equal(t, charge, ownerCharge)
				require.Equal(t, charge, balance-token.RemainQuota)
				require.EqualValues(t, charge, logs[0].Quota)
				if text == "" || exit == "fresh_final_receipt" {
					require.EqualValues(t, 18, charge, "fresh complete measured receipt remains authoritative")
					require.NotEqual(t, true, logs[0].Metadata["billing_estimated"])
					require.Nil(t, logs[0].Metadata["billing_estimate_reason"])
					return
				}
				require.Equal(t, floor, charge, "settle the receipt and independently counted later output exactly once")
				require.EqualValues(t, 11, logs[0].PromptTokens)
				require.EqualValues(t, floor-11, logs[0].CompletionTokens)
				require.Equal(t, true, logs[0].Metadata["billing_estimated"])
				reason := "stream_output_after_last_receipt"
				if strings.HasPrefix(route.name, "shared/") && exit == "eof" {
					reason = "stream_usage_missing_counters"
				}
				require.Equal(t, reason, logs[0].Metadata["billing_estimate_reason"])
			})
		}
	}
}
