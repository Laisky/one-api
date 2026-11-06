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
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// codexViewImagePayload builds a non-streaming Codex-shaped Responses request
// whose view_image tool output returns text followed by an inline image.
// Parameters: modelName is the requested model and imageData the base64 PNG.
// Returns: the JSON request body.
func codexViewImagePayload(t *testing.T, modelName, imageData string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"model":  modelName,
		"stream": false,
		"tools": []any{map[string]any{
			"type": "function", "name": "view_image", "description": "View a local image",
			"parameters": map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []any{"path"}},
		}},
		"input": []any{
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "What is in a.png?"}}},
			map[string]any{"type": "function_call", "call_id": "call_view", "name": "view_image", "arguments": `{"path":"a.png"}`},
			map[string]any{"type": "function_call_output", "call_id": "call_view", "output": []any{
				map[string]any{"type": "input_text", "text": "a.png"},
				map[string]any{"type": "input_image", "image_url": "data:image/png;base64," + imageData, "detail": "original"},
			}},
		},
	})
	require.NoError(t, err)
	return string(payload)
}

// runToolImageFallback drives RelayResponseAPIHelper against a fake upstream.
// Parameters: channelType and channelID select the fixture channel, modelName
// the model, authKey the client key, baseURL the fake upstream and payload the
// Responses body. Returns: the recorder holding the client response.
func runToolImageFallback(t *testing.T, channelType, channelID int, modelName, authKey, baseURL, payload string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authKey)
	c.Request = req
	gmw.SetLogger(c, logger.Logger)

	c.Set(ctxkey.Channel, channelType)
	c.Set(ctxkey.ChannelId, channelID)
	c.Set(ctxkey.ChannelModel, &model.Channel{Id: channelID, Type: channelType})
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.TokenName, "fallback-token")
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.RequestModel, modelName)
	c.Set(ctxkey.BaseURL, baseURL)
	c.Set(ctxkey.ContentType, "application/json")
	c.Set(ctxkey.RequestId, "req_fallback_tool_image")
	c.Set(ctxkey.TokenQuotaUnlimited, true)
	c.Set(ctxkey.TokenQuota, int64(0))
	c.Set(ctxkey.Username, "response-fallback")
	c.Set(ctxkey.UserObj, &model.User{Quota: 1_000_000})
	c.Set(ctxkey.Config, model.ChannelConfig{})

	apiErr := RelayResponseAPIHelper(c)
	drainResponseFallbackBilling(t)
	require.Nil(t, apiErr, "tool image fallback must succeed")
	require.Equal(t, http.StatusOK, recorder.Code)
	return recorder
}

// prepareToolImageFallback installs fixtures and a fake upstream that records
// the request body and answers with response. Parameters: response is the JSON
// body returned upstream. Returns: the server and a channel of recorded bodies.
func prepareToolImageFallback(t *testing.T, response string) (*httptest.Server, chan []byte) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)
	prevRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(prevRedis) })
	prevLogConsume := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(prevLogConsume) })

	bodies := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read upstream body: %v", err)
			return
		}
		bodies <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(upstream.Close)
	prevClient := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = prevClient })
	return upstream, bodies
}

// TestRelayResponseAPIHelper_FallbackClaudeToolResultImage proves a Codex
// view_image output reaches a Claude upstream as an image block inside the
// tool_result, and that the Chat carrier message is not forwarded.
func TestRelayResponseAPIHelper_FallbackClaudeToolResultImage(t *testing.T) {
	upstream, bodies := prepareToolImageFallback(t, `{"id":"msg_img","type":"message","role":"assistant","model":"claude-3-5-haiku","content":[{"type":"text","text":"A blank square."}],"stop_reason":"end_turn","usage":{"input_tokens":20,"output_tokens":4}}`)
	imageData := providerImageFixture(t)
	recorder := runToolImageFallback(t, channeltype.Anthropic, fallbackAnthropicChannelID,
		"claude-3.5-haiku", "anthropic-key", upstream.URL, codexViewImagePayload(t, "claude-3.5-haiku", imageData))

	body := <-bodies
	require.NotContains(t, string(body), "[Image output from tool call", "the Chat carrier must not reach Claude")
	var wire struct {
		Messages []struct {
			Role    string           `json:"role"`
			Content []map[string]any `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &wire))
	require.Len(t, wire.Messages, 3)
	require.Equal(t, "tool_use", wire.Messages[1].Content[0]["type"])
	toolUseID := wire.Messages[1].Content[0]["id"]

	require.Equal(t, "user", wire.Messages[2].Role)
	require.Len(t, wire.Messages[2].Content, 1)
	toolResult := wire.Messages[2].Content[0]
	require.Equal(t, "tool_result", toolResult["type"])
	require.Equal(t, toolUseID, toolResult["tool_use_id"])
	require.Equal(t, []any{
		map[string]any{"type": "text", "text": "a.png"},
		map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": imageData}},
	}, toolResult["content"])

	var response openai.ResponseAPIResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "completed", response.Status)
	require.Equal(t, "A blank square.", response.Output[0].Content[0].Text)
}

// TestRelayResponseAPIHelper_FallbackChatToolResultImage proves a Chat-only
// upstream receives a text tool message directly after the assistant tool call,
// followed by one user message that carries the image.
func TestRelayResponseAPIHelper_FallbackChatToolResultImage(t *testing.T) {
	upstream, bodies := prepareToolImageFallback(t, `{"id":"chatcmpl-img","object":"chat.completion","created":1741036800,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"A blank square."},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":4,"total_tokens":24}}`)
	imageData := providerImageFixture(t)
	recorder := runToolImageFallback(t, channeltype.OpenAICompatible, fallbackCompatibleChannelID,
		"gpt-4o-mini", "compat-key", upstream.URL, codexViewImagePayload(t, "gpt-4o-mini", imageData))

	var wire struct {
		Messages []map[string]any `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(<-bodies, &wire))
	require.Len(t, wire.Messages, 4)
	require.Equal(t, "assistant", wire.Messages[1]["role"])
	require.Equal(t, "tool", wire.Messages[2]["role"])
	require.Equal(t, "a.png", wire.Messages[2]["content"], "tool messages stay text-only")
	require.Equal(t, "user", wire.Messages[3]["role"])
	require.Equal(t, []any{
		map[string]any{"type": "text", "text": "[Image output from tool call view]"},
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + imageData, "detail": "high"}},
	}, wire.Messages[3]["content"])

	var response openai.ResponseAPIResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "completed", response.Status)
}
