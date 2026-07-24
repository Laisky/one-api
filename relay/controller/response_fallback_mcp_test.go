package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestRelayResponseAPIHelper_FallbackAnthropicMCPPrunesUnmatchedResponseTools reproduces Response fallback with MCP tools and prunes unmatched Response-only tools.
func TestRelayResponseAPIHelper_FallbackAnthropicMCPPrunesUnmatchedResponseTools(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)

	prevRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(prevRedis) })

	prevLogConsume := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(prevLogConsume) })

	err := model.DB.Where("name = ?", "mcp-response-fallback").Delete(&model.MCPServer{}).Error
	require.NoError(t, err, "failed to clean mcp server fixture")
	err = model.DB.Where("name = ?", "web_search").Delete(&model.MCPTool{}).Error
	require.NoError(t, err, "failed to clean mcp tool fixture")

	server := &model.MCPServer{
		Name:          "mcp-response-fallback",
		Status:        model.MCPServerStatusEnabled,
		BaseURL:       "http://mcp.response-fallback.example.com",
		ToolWhitelist: model.JSONStringSlice{"web_search"},
	}
	err = model.DB.Create(server).Error
	require.NoError(t, err, "failed to create mcp server fixture")
	t.Cleanup(func() {
		cleanupErr := model.DB.Where("id = ?", server.Id).Delete(&model.MCPServer{}).Error
		require.NoError(t, cleanupErr, "failed to clean mcp server fixture")
	})

	tool := &model.MCPTool{
		ServerId:    server.Id,
		Name:        "web_search",
		Description: "Search the web",
		InputSchema: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`,
	}
	err = model.DB.Create(tool).Error
	require.NoError(t, err, "failed to create mcp tool fixture")
	t.Cleanup(func() {
		cleanupErr := model.DB.Where("id = ?", tool.Id).Delete(&model.MCPTool{}).Error
		require.NoError(t, cleanupErr, "failed to clean mcp tool fixture")
	})

	upstreamCalled := false
	var upstreamBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		require.Equal(t, "anthropic-key", r.Header.Get("x-api-key"), "unexpected anthropic api key header")

		body, readErr := io.ReadAll(r.Body)
		require.NoError(t, readErr, "failed to read upstream body")
		upstreamBody = body

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "id": "msg_mcp_prune",
		  "type": "message",
		  "role": "assistant",
		  "model": "claude-sonnet-4-5",
		  "content": [{"type": "text", "text": "MCP fallback ok"}],
		  "stop_reason": "end_turn",
		  "usage": {"input_tokens": 11, "output_tokens": 5}
		}`))
	}))
	t.Cleanup(upstream.Close)

	prevClient := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = prevClient })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	requestPayload := `{
	  "model": "claude-sonnet-4-5",
	  "stream": true,
	  "instructions": "You are helpful.",
	  "input": [{"role": "user", "content": [{"type": "input_text", "text": "Search and summarize."}]}],
	  "tools": [
	    {"type": "web_search"},
	    {"type": "namespace"},
	    {"type": "function", "name": "section_edit", "description": "Edit a section", "parameters": {"type": "object"}}
	  ]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(requestPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer anthropic-key")
	c.Request = req

	gmw.SetLogger(c, logger.Logger)

	c.Set(ctxkey.Channel, channeltype.Anthropic)
	c.Set(ctxkey.ChannelId, fallbackAnthropicChannelID)
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.TokenName, "fallback-token")
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.RequestModel, "claude-sonnet-4-5")
	c.Set(ctxkey.BaseURL, upstream.URL)
	c.Set(ctxkey.ContentType, "application/json")
	c.Set(ctxkey.RequestId, "req_fallback_mcp_prune")
	c.Set(ctxkey.TokenQuotaUnlimited, true)
	c.Set(ctxkey.TokenQuota, int64(0))
	c.Set(ctxkey.Username, "response-fallback")
	c.Set(ctxkey.UserObj, &model.User{Quota: 1_000_000})
	c.Set(ctxkey.ChannelModel, &model.Channel{Id: fallbackAnthropicChannelID, Type: channeltype.Anthropic})
	c.Set(ctxkey.Config, model.ChannelConfig{})

	apiErr := RelayResponseAPIHelper(c)
	// Drain async billing before assertions/cleanups so its DB writes cannot race
	// this test's MCP-fixture teardown under -race (fixes the SQLITE_LOCKED flake).
	drainResponseFallbackBilling(t)
	require.Nil(t, apiErr, "expected anthropic MCP fallback to succeed")
	require.True(t, upstreamCalled, "expected upstream to be called after pruning unmatched Response-only tools")
	require.Equal(t, http.StatusOK, recorder.Code, "unexpected response status")

	var upstreamReq anthropic.Request
	err = json.Unmarshal(upstreamBody, &upstreamReq)
	require.NoError(t, err, "failed to unmarshal upstream anthropic request")
	require.False(t, upstreamReq.Stream, "MCP fallback should force non-streaming upstream requests")
	require.Len(t, upstreamReq.Tools, 2, "expected only function and expanded MCP tools to reach Anthropic")

	toolNames := []string{upstreamReq.Tools[0].Name, upstreamReq.Tools[1].Name}
	require.ElementsMatch(t, []string{"section_edit", "web_search"}, toolNames)
	require.NotContains(t, toolNames, "namespace", "unmatched Response-only namespace tool should be pruned")

	require.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream", "streaming Responses clients must receive SSE")
	events := parseSSEEvents(recorder.Body.String())
	require.NotEmpty(t, events, "expected downstream Responses SSE events")

	var fallbackResp *openai.ResponseAPIResponse
	for _, event := range events {
		if event.event != "response.completed" {
			continue
		}
		var streamEvent openai.ResponseAPIStreamEvent
		err = json.Unmarshal([]byte(event.data), &streamEvent)
		require.NoError(t, err, "failed to unmarshal response.completed event")
		fallbackResp = streamEvent.Response
	}
	require.NotNil(t, fallbackResp, "stream must include a terminal response.completed event")
	require.Equal(t, "completed", fallbackResp.Status, "expected completed response")
	require.Equal(t, "MCP fallback ok", fallbackResp.Output[0].Content[0].Text, "unexpected fallback response text")
	require.Equal(t, "[DONE]", strings.TrimSpace(events[len(events)-1].data), "stream must end with [DONE]")
}
