package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestSecurityDeferredToolNativeToConvertedRetry verifies the current provider determines schema admission after a real failed native attempt.
func TestSecurityDeferredToolNativeToConvertedRetry(t *testing.T) {
	for _, balance := range []int64{1000, 100000} {
		name := "underfunded"
		if balance > 1000 {
			name = "funded"
		}
		t.Run(name, func(t *testing.T) {
			xaiVideoSetup(t, balance, false)
			stored := &model.MCPServer{Name: "retry-deferral-" + name, Status: model.MCPServerStatusEnabled, BaseURL: "https://example.com/synthetic-unused-mcp"}
			require.NoError(t, model.DB.Create(stored).Error)
			t.Cleanup(func() { require.NoError(t, model.DB.Delete(stored).Error) })
			schema, err := json.Marshal(deferredAdmissionTool("absent")["input_schema"])
			require.NoError(t, err)
			tool := &model.MCPTool{ServerId: stored.Id, Name: "lookup_record", Description: "Synthetic catalog lookup", InputSchema: string(schema)}
			require.NoError(t, model.DB.Create(tool).Error)
			t.Cleanup(func() { require.NoError(t, model.DB.Delete(tool).Error) })
			var calls atomic.Int32
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
				if err != nil {
					t.Errorf("read synthetic request: %v", err)
					http.Error(w, "invalid request", 400)
					return
				}
				if !bytes.Contains(body, []byte("synthetic schema content")) {
					t.Error("catalog schema was not dispatched")
				}
				w.Header().Set("Content-Type", "application/json")
				payload := `{"id":"retry-catalog","type":"message","role":"assistant","model":"claude-sonnet-4","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7}}`
				if call == 1 {
					w.WriteHeader(http.StatusInternalServerError)
					payload = `{"type":"error","error":{"type":"api_error","message":"retry fixture failure"}}`
				}
				if _, err := io.WriteString(w, payload); err != nil {
					t.Errorf("write synthetic response: %v", err)
				}
			}))
			defer upstream.Close()
			previous := client.HTTPClient
			client.HTTPClient = upstream.Client()
			t.Cleanup(func() { client.HTTPClient = previous })
			payload := `{"model":"alias","max_tokens":64,"messages":[{"role":"user","content":"hello"}],"tools":[{"type":"tool_search_tool_regex_20251119","name":"tool_search_tool_regex"}]}`
			c, _, id := protocolContext(t, channeltype.Anthropic, "claude-sonnet-4", "/v1/messages", payload, upstream.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			firstErr := RelayClaudeMessagesHelper(c)
			drainCriticalTasks(t)
			require.NotNil(t, firstErr)
			require.Equal(t, http.StatusInternalServerError, firstErr.StatusCode)
			require.True(t, c.GetBool(ctxkey.ClaudeDirectPassthrough), "the real native adaptor must establish passthrough provenance")
			require.EqualValues(t, 1, calls.Load())
			ResetPerAttemptBillingForRetry(gmw.Ctx(c), c)
			drainCriticalTasks(t)
			require.Equal(t, balance, reloadUserQuota(t), "the abandoned attempt hold must be refunded before retry")
			channel := *c.MustGet(ctxkey.ChannelModel).(*model.Channel)
			channel.Id++
			channel.Type = channeltype.OpenAI
			channel.Group = "default"
			channel.BaseURL = &upstream.URL
			mapping := `{"alias":"claude-sonnet-4"}`
			channel.ModelMapping = &mapping
			middleware.SetupContextForSelectedChannel(c, &channel, "alias")
			body, err := common.GetRequestBody(c)
			require.NoError(t, err)
			c.Request.Body = io.NopCloser(bytes.NewReader(body))
			secondErr := RelayClaudeMessagesHelper(c)
			drainCriticalTasks(t)
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			if balance == 1000 {
				require.NotNil(t, secondErr, "a stale native flag must not exempt the converted catalog schema")
				require.Equal(t, http.StatusForbidden, secondErr.StatusCode)
				require.EqualValues(t, 1, calls.Load(), "underfunded retry must not dispatch")
				require.Equal(t, balance, reloadUserQuota(t))
				require.Equal(t, balance, token.RemainQuota)
				require.Zero(t, token.UsedQuota)
				return
			}
			require.Nil(t, secondErr)
			require.EqualValues(t, 2, calls.Load(), "funded retry must remain functional")
			require.EqualValues(t, 18, requestCostQuota(t, id))
			require.Equal(t, balance-18, reloadUserQuota(t))
			require.Equal(t, balance-18, token.RemainQuota)
			require.EqualValues(t, 18, token.UsedQuota)
		})
	}
}
