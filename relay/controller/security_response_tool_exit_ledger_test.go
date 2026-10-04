package controller

import (
	"context"
	"fmt"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// responseSecurityCancelWriter cancels a synthetic client only after the real
// handler has observed and delivered a provider receipt containing a paid action.
type responseSecurityCancelWriter struct {
	gin.ResponseWriter
	cancel context.CancelFunc
}

// Write cancels after a delivered receipt, independently of billing internals.
func (w *responseSecurityCancelWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if strings.Contains(string(p), "web_search_call") {
		w.cancel()
	}
	return n, err
}

// TestSecurityResponseToolExitLedger traverses admission, actual HTTP streaming,
// tool pricing and durable owner/token/log settlement on cancellation and failure.
func TestSecurityResponseToolExitLedger(t *testing.T) {
	for _, cancelClient := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelClient), func(t *testing.T) {
			const balance = int64(100_000)
			xaiVideoSetup(t, balance, false)
			var calls atomic.Int32
			paths := make(chan string, 1)
			stoppedByCancel := make(chan bool, 1)
			upstreamStopped := make(chan struct{})
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				paths <- r.URL.Path
				defer close(upstreamStopped)
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, `event: response.completed
data: {"type":"response.completed","response":{"id":"resp-synthetic","status":"completed","output":[{"type":"web_search_call","id":"ws-synthetic","action":{"type":"search","query":"synthetic"}},{"type":"message","id":"msg-synthetic","role":"assistant","content":[{"type":"output_text","text":"answer"}]}],"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}}}

`[0:])
				w.(http.Flusher).Flush()
				if cancelClient {
					select {
					case <-r.Context().Done():
						stoppedByCancel <- true
					case <-time.After(5 * time.Second):
						stoppedByCancel <- false
					}
				}
				panic(http.ErrAbortHandler)
			}))
			t.Cleanup(server.Close)
			old := client.HTTPClient
			client.HTTPClient = server.Client()
			t.Cleanup(func() { client.HTTPClient = old })
			c, _, id := protocolContext(t, channeltype.OpenAICompatible, "gpt-4o", "/v1/responses", `{"model":"alias","input":"hello","stream":true,"max_output_tokens":1,"tools":[{"type":"web_search"}]}`, server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			c.Set(ctxkey.Config, model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse})
			require.True(t, supportsNativeResponseAPI(metalib.GetByContext(c)), "fixture must enter the actual native Responses handler")
			value, _ := c.Get(ctxkey.ChannelModel)
			channel := value.(*model.Channel)
			require.NoError(t, channel.SetToolingConfig(&model.ChannelToolingConfig{Whitelist: []string{"web_search"}, Pricing: map[string]model.ToolPricingLocal{"web_search": {QuotaPerCall: 17}}}))
			ctx, cancel := context.WithCancel(c.Request.Context())
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			if cancelClient {
				c.Writer = &responseSecurityCancelWriter{ResponseWriter: c.Writer, cancel: cancel}
			}
			_ = RelayResponseAPIHelper(c)
			drainCriticalTasks(t)
			require.EqualValues(t, 1, calls.Load())
			require.Equal(t, "/v1/responses", <-paths)
			select {
			case <-upstreamStopped:
			case <-time.After(6 * time.Second):
				t.Fatal("upstream did not stop")
			}
			if cancelClient {
				require.True(t, <-stoppedByCancel, "real upstream must observe client cancellation")
			}
			actual := requestCostQuota(t, id)
			if actual != 35 {
				t.Logf("REPRODUCED_488_TOOL_LEDGER actual=%d expected=35", actual)
			}
			require.EqualValues(t, 35, actual)
			require.Equal(t, balance-35, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance-35, token.RemainQuota)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", id, model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			require.EqualValues(t, 35, logs[0].Quota)
		})
	}
}
