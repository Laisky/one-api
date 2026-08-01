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
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestRelayResponseAPIHelperDeepSeekV4FlashNativeToolContinuation reproduces a
// Codex post-tool turn and verifies that one-api preserves its Responses wire
// format, including the reasoning item required by DeepSeek thinking mode.
func TestRelayResponseAPIHelperDeepSeekV4FlashNativeToolContinuation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)

	previousRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(previousRedis) })

	previousLogConsume := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(previousLogConsume) })

	const mcpServerName = "mcp-deepseek-native-precedence"
	require.NoError(t, model.DB.Where("name = ?", mcpServerName).Delete(&model.MCPServer{}).Error)
	mcpServer := &model.MCPServer{
		Name:          mcpServerName,
		Status:        model.MCPServerStatusEnabled,
		BaseURL:       "http://mcp.deepseek-native.example.com",
		ToolWhitelist: model.JSONStringSlice{"web_search"},
	}
	require.NoError(t, model.DB.Create(mcpServer).Error)
	t.Cleanup(func() {
		require.NoError(t, model.DB.Where("server_id = ?", mcpServer.Id).Delete(&model.MCPTool{}).Error)
		require.NoError(t, model.DB.Where("id = ?", mcpServer.Id).Delete(&model.MCPServer{}).Error)
	})
	require.NoError(t, model.DB.Create(&model.MCPTool{
		ServerId:    mcpServer.Id,
		Name:        "web_search",
		Description: "Search the web",
		InputSchema: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`,
	}).Error)

	var upstreamPath string
	var upstreamBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		upstreamBody = body

		w.Header().Set("Content-Type", "text/event-stream")
		_, err = io.WriteString(w, strings.Join([]string{
			"event: response.output_item.added",
			`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"rs_next","type":"reasoning","content":[{"type":"text","text":"continue after tool"}]}}`,
			"",
			"event: response.output_item.added",
			`data: {"type":"response.output_item.added","output_index":1,"item":{"id":"fc_next","type":"function_call","call_id":"call_next","name":"read_file","arguments":"{}","status":"completed"}}`,
			"",
			"event: response.completed",
			`data: {"type":"response.completed","response":{"id":"resp_next","object":"response","status":"completed","model":"deepseek-v4-flash","output":[{"id":"rs_next","type":"reasoning","content":[{"type":"text","text":"continue after tool"}]},{"id":"fc_next","type":"function_call","call_id":"call_next","name":"read_file","arguments":"{}","status":"completed"}],"usage":{"input_tokens":12,"output_tokens":7,"total_tokens":19}}}`,
			"",
		}, "\n"))
		require.NoError(t, err)
	}))
	t.Cleanup(upstream.Close)

	previousClient := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = previousClient })

	requestPayload := `{"model":"deepseek-v4-flash","stream":true,"input":[{"type":"reasoning","id":"rs_prev","content":[{"type":"text","text":"inspect the file first"}]},{"type":"function_call","id":"fc_prev","call_id":"call_prev","name":"read_file","arguments":"{\"path\":\"README.md\"}"},{"type":"function_call_output","call_id":"call_prev","output":"file contents"}],"tools":[{"type":"function","name":"read_file","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}},{"type":"web_search"}]}`
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(requestPayload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Authorization", "Bearer test-upstream-key")
	gmw.SetLogger(c, logger.Logger)

	c.Set(ctxkey.Channel, channeltype.DeepSeek)
	c.Set(ctxkey.ChannelId, fallbackNVIDIAChannelID)
	c.Set(ctxkey.ChannelModel, &model.Channel{Id: fallbackNVIDIAChannelID, Type: channeltype.DeepSeek})
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.TokenName, "fallback-token")
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.RequestModel, "deepseek-v4-flash")
	c.Set(ctxkey.BaseURL, upstream.URL)
	c.Set(ctxkey.ContentType, "application/json")
	c.Set(ctxkey.RequestId, "req_deepseek_native_tool_continuation")
	c.Set(ctxkey.TokenQuotaUnlimited, true)
	c.Set(ctxkey.TokenQuota, int64(0))
	c.Set(ctxkey.Username, "response-fallback")
	c.Set(ctxkey.UserObj, &model.User{Quota: 1_000_000})
	c.Set(ctxkey.Config, model.ChannelConfig{})

	apiErr := RelayResponseAPIHelper(c)
	drainResponseFallbackBilling(t)
	require.Nil(t, apiErr)
	require.Equal(t, "/v1/responses", upstreamPath, "DeepSeek V4 Flash must use its native Responses endpoint")

	var forwarded map[string]any
	require.NoError(t, json.Unmarshal(upstreamBody, &forwarded))
	require.NotContains(t, forwarded, "messages", "the continuation must not be converted to Chat Completions")
	input, ok := forwarded["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 3)
	reasoning, ok := input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "reasoning", reasoning["type"])
	require.NotEmpty(t, reasoning["content"], "plaintext reasoning state must reach DeepSeek")

	responseBody := recorder.Body.String()
	require.Contains(t, responseBody, "event: response.output_item.added\n")
	require.Contains(t, responseBody, `"type":"reasoning","content"`)
	require.Contains(t, responseBody, `"type":"function_call"`)
	require.Contains(t, responseBody, "event: response.completed\n")
}
