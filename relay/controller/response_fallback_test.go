package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/client"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/adaptor/openai_compatible"
	"github.com/songquanpeng/one-api/relay/channeltype"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

const (
	fallbackUserID              = 99001
	fallbackTokenID             = 99002
	fallbackChannelID           = 99003
	fallbackCompatibleChannelID = 99004
	fallbackAnthropicChannelID  = 99005
)

func TestRenderChatResponseAsResponseAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(ctxkey.RequestId, "req_123")

	textResp := &openai_compatible.SlimTextResponse{
		Choices: []openai_compatible.TextResponseChoice{
			{
				Message:      relaymodel.Message{Role: "assistant", Content: "Hello there"},
				FinishReason: "stop",
			},
		},
		Usage: relaymodel.Usage{PromptTokens: 12, CompletionTokens: 8, TotalTokens: 20},
	}

	parallel := true
	request := &openai.ResponseAPIRequest{ParallelToolCalls: &parallel}
	meta := &metalib.Meta{ActualModelName: "gpt-fallback"}

	err := renderChatResponseAsResponseAPI(c, http.StatusOK, textResp, request, meta)
	require.NoError(t, err, "unexpected error rendering response")

	var resp openai.ResponseAPIResponse
	err = json.Unmarshal(recorder.Body.Bytes(), &resp)
	require.NoError(t, err, "failed to unmarshal response body")

	require.Equal(t, "gpt-fallback", resp.Model, "expected model gpt-fallback")
	require.Equal(t, "completed", resp.Status, "expected status completed")
	require.Len(t, resp.Output, 1, "expected single output item")
	require.Equal(t, "message", resp.Output[0].Type, "expected message output")
	require.NotEmpty(t, resp.Output[0].Content, "expected message content")
	require.Equal(t, "Hello there", resp.Output[0].Content[0].Text, "expected message content preserved")
	require.NotNil(t, resp.Usage, "expected usage to be carried over")
	require.Equal(t, 20, resp.Usage.TotalTokens, "expected total tokens to be 20")
	require.True(t, resp.ParallelToolCalls, "expected parallel tool calls to be true")
}

func TestRelayResponseAPIHelper_FallbackAzure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)

	prevRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(prevRedis) })

	prevLogConsume := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(prevLogConsume) })

	upstreamCalled := false
	var upstreamPath string
	var upstreamBody []byte

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		upstreamPath = r.URL.Path
		if r.URL.RawQuery != "" {
			upstreamPath += "?" + r.URL.RawQuery
		}
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err, "failed to read upstream body")
		upstreamBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "id": "chatcmpl-123",
		  "object": "chat.completion",
		  "created": 1741036800,
		  "model": "gpt-4o-mini",
		  "choices": [
		    {
		      "index": 0,
		      "message": {"role": "assistant", "content": "Hi there!"},
		      "finish_reason": "stop"
		    }
		  ],
		  "usage": {"prompt_tokens": 5, "completion_tokens": 8, "total_tokens": 13}
		}`))
	}))
	defer upstream.Close()

	prevClient := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = prevClient })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	requestPayload := `{"model":"gpt-4o-mini","stream":false,"instructions":"You are helpful.","input":[{"role":"user","content":[{"type":"input_text","text":"Hello via response API"}]}],"parallel_tool_calls":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(requestPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer azure-key")
	c.Request = req

	gmw.SetLogger(c, logger.Logger)

	c.Set(ctxkey.Channel, channeltype.Azure)
	c.Set(ctxkey.ChannelId, fallbackChannelID)
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.TokenName, "fallback-token")
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.RequestModel, "gpt-4o-mini")
	c.Set(ctxkey.BaseURL, upstream.URL)
	c.Set(ctxkey.ContentType, "application/json")
	c.Set(ctxkey.RequestId, "req_fallback")
	c.Set(ctxkey.TokenQuotaUnlimited, true)
	c.Set(ctxkey.TokenQuota, int64(0))
	c.Set(ctxkey.Username, "response-fallback")
	c.Set(ctxkey.UserQuota, int64(1_000_000))
	c.Set(ctxkey.ChannelModel, &model.Channel{Id: fallbackChannelID, Type: channeltype.Azure})
	c.Set(ctxkey.Config, model.ChannelConfig{APIVersion: "2024-02-15-preview"})

	apiErr := RelayResponseAPIHelper(c)
	require.Nil(t, apiErr, "RelayResponseAPIHelper returned error")

	require.Equal(t, http.StatusOK, recorder.Code, "unexpected status code")
	require.True(t, upstreamCalled, "expected upstream to be called")
	require.Contains(t, upstreamPath, "/openai/deployments/gpt-4o-mini/chat/completions", "unexpected upstream path")
	require.Contains(t, upstreamPath, "api-version=", "expected api-version query parameter in upstream path")

	var fallbackResp openai.ResponseAPIResponse
	err := json.Unmarshal(recorder.Body.Bytes(), &fallbackResp)
	require.NoError(t, err, "failed to unmarshal fallback response body")
	require.Equal(t, "completed", fallbackResp.Status, "expected response status completed")
	require.Len(t, fallbackResp.Output, 1, "expected single output item")
	output := fallbackResp.Output[0]
	require.Equal(t, "message", output.Type, "expected message output type")
	require.NotEmpty(t, output.Content, "expected output content")
	require.Equal(t, "Hi there!", output.Content[0].Text, "unexpected output content")
	require.NotNil(t, fallbackResp.Usage, "expected usage")
	require.Equal(t, 13, fallbackResp.Usage.TotalTokens, "unexpected usage total tokens")
	require.Nil(t, fallbackResp.RequiredAction, "did not expect required_action for non-tool response")
	require.True(t, fallbackResp.ParallelToolCalls, "expected parallel tool calls to remain true")

	var chatReq relaymodel.GeneralOpenAIRequest
	err = json.Unmarshal(upstreamBody, &chatReq)
	require.NoError(t, err, "failed to unmarshal upstream chat request")
	require.Equal(t, "gpt-4o-mini", chatReq.Model, "expected chat request model gpt-4o-mini")
	require.Len(t, chatReq.Messages, 2, "expected two messages (system + user)")
	require.Equal(t, "system", chatReq.Messages[0].Role, "expected system role")
	require.Equal(t, "You are helpful.", chatReq.Messages[0].StringContent(), "system message not preserved")
	require.Equal(t, "user", chatReq.Messages[1].Role, "expected user role")
	require.Equal(t, "Hello via response API", chatReq.Messages[1].StringContent(), "user message not preserved")
}

func TestRelayResponseAPIHelper_FallbackBlocksDisallowedWebSearch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)

	prevRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(prevRedis) })

	prevLogConsume := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(prevLogConsume) })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	requestPayload := `{"model":"gpt-4o-mini","input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}],"tools":[{"type":"web_search"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(requestPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer azure-key")
	c.Request = req

	gmw.SetLogger(c, logger.Logger)

	channel := &model.Channel{Id: fallbackChannelID, Type: channeltype.Azure, Name: "azure-fallback", Status: model.ChannelStatusEnabled}
	err := channel.SetToolingConfig(&model.ChannelToolingConfig{
		Whitelist: []string{"code_interpreter"},
		Pricing: map[string]model.ToolPricingLocal{
			"code_interpreter": {UsdPerCall: 0.03},
		},
	})
	require.NoError(t, err, "failed to set channel tooling")

	c.Set(ctxkey.Channel, channeltype.Azure)
	c.Set(ctxkey.ChannelId, fallbackChannelID)
	c.Set(ctxkey.ChannelModel, channel)
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.TokenName, "fallback-token")
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.RequestModel, "gpt-4o-mini")
	c.Set(ctxkey.BaseURL, "https://example.azure.com")
	c.Set(ctxkey.ContentType, "application/json")
	c.Set(ctxkey.TokenQuotaUnlimited, true)
	c.Set(ctxkey.TokenQuota, int64(0))
	c.Set(ctxkey.Username, "response-fallback")
	c.Set(ctxkey.UserQuota, int64(1_000_000))
	c.Set(ctxkey.Config, model.ChannelConfig{})

	apiErr := RelayResponseAPIHelper(c)
	require.NotNil(t, apiErr, "expected error when web_search tool is not whitelisted")
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode, "expected status 400")
	require.Contains(t, apiErr.Message, "web_search", "expected error mentioning web_search")
}

func TestRelayResponseAPIHelper_FallbackStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)

	prevRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(prevRedis) })

	prevLogConsume := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(prevLogConsume) })

	upstreamCalled := false
	var upstreamPath string
	var upstreamBody []byte

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		upstreamPath = r.URL.Path
		if r.URL.RawQuery != "" {
			upstreamPath += "?" + r.URL.RawQuery
		}
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err, "failed to read upstream body")
		upstreamBody = body

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		require.True(t, ok, "response writer does not support flushing")

		chunks := []string{
			`{"id":"chatcmpl-123","object":"chat.completion.chunk","created":1741036800,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-123","object":"chat.completion.chunk","created":1741036800,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"content":" world!"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-123","object":"chat.completion.chunk","created":1741036800,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":7,"total_tokens":12}}`,
		}
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	prevClient := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = prevClient })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	requestPayload := `{"model":"gpt-4o-mini","stream":true,"instructions":"You are helpful.","input":[{"role":"user","content":[{"type":"input_text","text":"Hello via response API stream"}]}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(requestPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer compat-key")
	c.Request = req

	gmw.SetLogger(c, logger.Logger)

	c.Set(ctxkey.Channel, channeltype.OpenAICompatible)
	c.Set(ctxkey.ChannelId, fallbackCompatibleChannelID)
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.TokenName, "fallback-token")
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.RequestModel, "gpt-4o-mini")
	c.Set(ctxkey.BaseURL, upstream.URL)
	c.Set(ctxkey.ContentType, "application/json")
	c.Set(ctxkey.RequestId, "req_fallback_stream")
	c.Set(ctxkey.TokenQuotaUnlimited, true)
	c.Set(ctxkey.TokenQuota, int64(0))
	c.Set(ctxkey.Username, "response-fallback")
	c.Set(ctxkey.UserQuota, int64(1_000_000))
	c.Set(ctxkey.ChannelModel, &model.Channel{Id: fallbackCompatibleChannelID, Type: channeltype.OpenAICompatible})
	c.Set(ctxkey.Config, model.ChannelConfig{})

	apiErr := RelayResponseAPIHelper(c)
	require.Nil(t, apiErr, "RelayResponseAPIHelper returned error")

	require.Equal(t, http.StatusOK, recorder.Code, "unexpected status code")
	require.True(t, upstreamCalled, "expected upstream to be called")
	require.True(t, strings.Contains(upstreamPath, "/v1/chat/completions") || strings.Contains(upstreamPath, "/chat/completions"), "unexpected upstream path for streaming fallback: %s", upstreamPath)

	var chatReq relaymodel.GeneralOpenAIRequest
	chatErr := json.Unmarshal(upstreamBody, &chatReq)
	require.NoError(t, chatErr, "failed to unmarshal upstream chat request")
	require.True(t, chatReq.Stream, "expected chat request stream flag to be true")
	require.NotEmpty(t, chatReq.Messages, "expected messages in chat request")
	require.NotEmpty(t, chatReq.Messages[0].StringContent(), "expected user message in chat request")

	events := parseSSEEvents(recorder.Body.String())
	t.Logf("raw SSE: %s", recorder.Body.String())
	t.Logf("parsed SSE events: %+v", events)
	require.NotEmpty(t, events, "expected SSE events, got none")

	var (
		seenCreated   bool
		seenCompleted bool
		deltaCount    int
		finalResponse *openai.ResponseAPIResponse
	)

	for idx, ev := range events {
		if idx == len(events)-1 {
			require.True(t, ev.event == "" && strings.TrimSpace(ev.data) == "[DONE]", "expected final SSE chunk to be [DONE], got event=%q data=%q", ev.event, ev.data)
			continue
		}
		switch ev.event {
		case "response.created":
			seenCreated = true
		case "response.output_text.delta":
			deltaCount++
		case "response.completed":
			seenCompleted = true
			var streamEvent openai.ResponseAPIStreamEvent
			err := json.Unmarshal([]byte(ev.data), &streamEvent)
			require.NoError(t, err, "failed to unmarshal response.completed event")
			require.NotNil(t, streamEvent.Response, "expected response payload in response.completed event")
			finalResponse = streamEvent.Response
		}
	}

	require.True(t, seenCreated, "missing response.created event")
	require.GreaterOrEqual(t, deltaCount, 2, "expected at least two delta events")
	require.True(t, seenCompleted, "missing response.completed event")
	require.NotNil(t, finalResponse, "missing final response")
	require.Equal(t, "completed", finalResponse.Status, "expected final status completed")
	require.NotEmpty(t, finalResponse.Output, "expected output in final response")
	require.NotEmpty(t, finalResponse.Output[0].Content, "expected output content in final response")
	require.Equal(t, "Hello world!", finalResponse.Output[0].Content[0].Text, "unexpected final response text")
	require.NotNil(t, finalResponse.Usage, "expected usage in final response")
	require.Equal(t, 12, finalResponse.Usage.TotalTokens, "unexpected usage total tokens in final response")
}

func TestRelayResponseAPIHelper_FallbackStreamingToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)

	prevRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(prevRedis) })

	prevLogConsume := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(prevLogConsume) })

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		require.True(t, ok, "response writer does not support flushing")

		chunks := []string{
			`{"id":"chatcmpl-tool","object":"chat.completion.chunk","created":1741036800,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"id":"call_delta","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}`,
			`{"id":"chatcmpl-tool","object":"chat.completion.chunk","created":1741036801,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"type":"function","function":{"arguments":"{\"location\":\"San Francisco, CA\",\"unit\":\"celsius\"}"}}]},"finish_reason":null}]}`,
			`{"id":"chatcmpl-tool","object":"chat.completion.chunk","created":1741036802,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":18,"completion_tokens":9,"total_tokens":27}}`,
		}
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	prevClient := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = prevClient })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	requestPayload := `{"model":"gpt-4o-mini","stream":true,"instructions":"You are helpful.","input":[{"role":"user","content":[{"type":"input_text","text":"Please call get_weather for San Francisco, CA."}]}],"tools":[{"type":"function","name":"get_weather","description":"Get the current weather for a location","parameters":{"type":"object","properties":{"location":{"type":"string"},"unit":{"type":"string"}},"required":["location"]}}],"tool_choice":{"type":"tool","name":"get_weather"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(requestPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer compat-key")
	c.Request = req

	gmw.SetLogger(c, logger.Logger)

	c.Set(ctxkey.Channel, channeltype.OpenAICompatible)
	c.Set(ctxkey.ChannelId, fallbackCompatibleChannelID)
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.TokenName, "fallback-token")
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.RequestModel, "gpt-4o-mini")
	c.Set(ctxkey.BaseURL, upstream.URL)
	c.Set(ctxkey.ContentType, "application/json")
	c.Set(ctxkey.RequestId, "req_fallback_stream_tool")
	c.Set(ctxkey.TokenQuotaUnlimited, true)
	c.Set(ctxkey.TokenQuota, int64(0))
	c.Set(ctxkey.Username, "response-fallback")
	c.Set(ctxkey.UserQuota, int64(1_000_000))
	c.Set(ctxkey.ChannelModel, &model.Channel{Id: fallbackCompatibleChannelID, Type: channeltype.OpenAICompatible})
	c.Set(ctxkey.Config, model.ChannelConfig{})

	apiErr := RelayResponseAPIHelper(c)
	require.Nil(t, apiErr, "RelayResponseAPIHelper returned error")

	require.Equal(t, http.StatusOK, recorder.Code, "unexpected status code")

	events := parseSSEEvents(recorder.Body.String())
	require.NotEmpty(t, events, "expected SSE events, got none")

	var (
		seenRequiredAction bool
		finalResponse      *openai.ResponseAPIResponse
	)

	for _, ev := range events {
		switch ev.event {
		case "response.required_action.added", "response.required_action.delta", "response.required_action.done":
			var streamEvent openai.ResponseAPIStreamEvent
			unmarshalErr := json.Unmarshal([]byte(ev.data), &streamEvent)
			require.NoError(t, unmarshalErr, "failed to decode required_action event")
			require.NotNil(t, streamEvent.RequiredAction, "expected required_action in event")
			require.NotNil(t, streamEvent.RequiredAction.SubmitToolOutputs, "expected submit_tool_outputs in required_action")
			require.NotEmpty(t, streamEvent.RequiredAction.SubmitToolOutputs.ToolCalls, "expected tool call in required_action event")
			seenRequiredAction = true
		case "response.completed":
			var streamEvent openai.ResponseAPIStreamEvent
			err := json.Unmarshal([]byte(ev.data), &streamEvent)
			require.NoError(t, err, "failed to decode response.completed event")
			require.NotNil(t, streamEvent.Response, "expected response payload in response.completed event")
			finalResponse = streamEvent.Response
		}
	}

	require.True(t, seenRequiredAction, "expected required_action events in stream, got %v", events)
	require.NotNil(t, finalResponse, "missing final response payload")
	require.NotNil(t, finalResponse.RequiredAction, "expected required_action in final response")
	require.NotNil(t, finalResponse.RequiredAction.SubmitToolOutputs, "expected submit_tool_outputs in required_action")
	toolCalls := finalResponse.RequiredAction.SubmitToolOutputs.ToolCalls
	require.Len(t, toolCalls, 1, "expected single tool call in final response")
	fn := toolCalls[0].Function
	require.NotNil(t, fn, "expected function in tool call")
	require.Equal(t, "get_weather", fn.Name, "unexpected tool call function name")
	require.NotEmpty(t, fn.Arguments, "expected arguments in tool call function")
}

func TestRelayResponseAPIHelper_FallbackAnthropicStreamingHandled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)

	prevRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(prevRedis) })

	prevLogConsume := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(prevLogConsume) })

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "anthropic-key" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"invalid x-api-key"}}`))
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, _ := w.(http.Flusher)

		events := []string{
			`{"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","model":"claude-3-5-haiku","usage":{"input_tokens":5,"output_tokens":0}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"id":"cb_1","type":"text","text":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world!"}}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":5,"output_tokens":7}}`,
			`{"type":"message_stop"}`,
		}

		for _, ev := range events {
			fmt.Fprintf(w, "data: %s\n\n", ev)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	t.Cleanup(upstream.Close)

	prevClient := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = prevClient })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	requestPayload := `{"model":"claude-3.5-haiku","stream":true,"instructions":"You are helpful.","input":[{"role":"user","content":[{"type":"input_text","text":"Hello via response API stream"}]}]}`
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
	c.Set(ctxkey.RequestModel, "claude-3.5-haiku")
	c.Set(ctxkey.BaseURL, upstream.URL)
	c.Set(ctxkey.ContentType, "application/json")
	c.Set(ctxkey.RequestId, "req_fallback_stream_anthropic")
	c.Set(ctxkey.TokenQuotaUnlimited, true)
	c.Set(ctxkey.TokenQuota, int64(0))
	c.Set(ctxkey.Username, "response-fallback")
	c.Set(ctxkey.UserQuota, int64(1_000_000))
	c.Set(ctxkey.ChannelModel, &model.Channel{Id: fallbackAnthropicChannelID, Type: channeltype.Anthropic})
	c.Set(ctxkey.Config, model.ChannelConfig{})

	apiErr := RelayResponseAPIHelper(c)
	require.Nil(t, apiErr, "expected anthropic streaming fallback to succeed")

	require.Equal(t, http.StatusOK, recorder.Code, "expected status 200")

	ct := recorder.Header().Get("Content-Type")
	require.Contains(t, ct, "text/event-stream", "expected text/event-stream content type")

	body := recorder.Body.String()
	events := parseSSEEvents(body)
	require.NotEmpty(t, events, "expected SSE events from anthropic stream, got none. body=%q", body)

	var (
		seenCreated   bool
		seenCompleted bool
		finalResponse *openai.ResponseAPIResponse
	)

	for _, ev := range events {
		switch ev.event {
		case "response.created":
			seenCreated = true
		case "response.completed":
			seenCompleted = true
			var streamEvent openai.ResponseAPIStreamEvent
			err := json.Unmarshal([]byte(ev.data), &streamEvent)
			require.NoError(t, err, "failed to decode response.completed event")
			require.NotNil(t, streamEvent.Response, "expected response payload in response.completed event")
			finalResponse = streamEvent.Response
		}
	}

	require.True(t, seenCreated, "missing response.created event from anthropic stream. events=%#v", events)
	require.True(t, seenCompleted, "missing response.completed event from anthropic stream. events=%#v", events)
	require.NotNil(t, finalResponse, "missing final response payload")
	require.Equal(t, "completed", finalResponse.Status, "expected completed status")
	require.NotEmpty(t, finalResponse.Output, "expected final output")
	require.NotEmpty(t, finalResponse.Output[0].Content, "expected final output content")
	require.Equal(t, "Hello world!", finalResponse.Output[0].Content[0].Text, "unexpected final response text")
}

func TestNormalizeResponseAPIRawBody_RemovesUnsupportedParams(t *testing.T) {
	temp := 0.7
	topP := 0.9
	req := &openai.ResponseAPIRequest{Model: "gpt-5-mini", Temperature: &temp, TopP: &topP}

	sanitizeResponseAPIRequest(req, channeltype.OpenAI)
	require.Nil(t, req.Temperature, "expected temperature pointer to be nil after sanitization")
	require.Nil(t, req.TopP, "expected top_p pointer to be nil after sanitization")

	raw := []byte(`{"model":"gpt-5-mini","temperature":0.7,"top_p":0.9}`)
	patched, err := normalizeResponseAPIRawBody(raw, req)
	require.NoError(t, err, "normalizeResponseAPIRawBody failed")
	require.False(t, bytes.Contains(patched, []byte(`"temperature"`)), "temperature should have been removed: %s", patched)
	require.False(t, bytes.Contains(patched, []byte(`"top_p"`)), "top_p should have been removed: %s", patched)
}

type parsedSSE struct {
	event string
	data  string
}

func parseSSEEvents(raw string) []parsedSSE {
	var events []parsedSSE
	remaining := raw
	for len(remaining) > 0 {
		chunkEnd := strings.Index(remaining, "\n\n")
		var chunk string
		if chunkEnd == -1 {
			chunk = remaining
			remaining = ""
		} else {
			chunk = remaining[:chunkEnd]
			remaining = remaining[chunkEnd+2:]
		}
		chunk = strings.Trim(chunk, "\n")
		if chunk == "" {
			continue
		}

		lines := strings.Split(chunk, "\n")
		var ev parsedSSE
		for _, line := range lines {
			line = strings.TrimRight(line, "\r")
			if strings.HasPrefix(line, "event: ") {
				ev.event = strings.TrimSpace(line[len("event: "):])
			} else if strings.HasPrefix(line, "data: ") {
				dataLine := line[len("data: "):]
				if ev.data != "" {
					ev.data += "\n"
				}
				ev.data += dataLine
			}
		}
		if ev.event != "" || ev.data != "" {
			events = append(events, ev)
		}
	}
	return events
}

func ensureResponseFallbackFixtures(t *testing.T) {
	t.Helper()
	ensureResponseFallbackDB(t)

	err := model.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.UserRequestCost{}, &model.Log{}, &model.Trace{})
	require.NoError(t, err, "failed to migrate tables")

	err = model.DB.Where("id = ?", fallbackUserID).Delete(&model.User{}).Error
	require.NoError(t, err, "failed to clean user fixture")
	user := &model.User{Id: fallbackUserID, Username: "response-fallback", Quota: 1_000_000, Status: model.UserStatusEnabled}
	err = model.DB.Create(user).Error
	require.NoError(t, err, "failed to create user fixture")

	err = model.DB.Where("id = ?", fallbackTokenID).Delete(&model.Token{}).Error
	require.NoError(t, err, "failed to clean token fixture")
	token := &model.Token{
		Id:             fallbackTokenID,
		UserId:         fallbackUserID,
		Key:            "fallback-token-key",
		Name:           "fallback-token",
		Status:         model.TokenStatusEnabled,
		UnlimitedQuota: true,
		RemainQuota:    0,
	}
	err = model.DB.Create(token).Error
	require.NoError(t, err, "failed to create token fixture")

	err = model.DB.Where("id = ?", fallbackChannelID).Delete(&model.Channel{}).Error
	require.NoError(t, err, "failed to clean channel fixture")
	channel := &model.Channel{Id: fallbackChannelID, Type: channeltype.Azure, Name: "azure-fallback", Status: model.ChannelStatusEnabled}
	err = model.DB.Create(channel).Error
	require.NoError(t, err, "failed to create channel fixture")

	err = model.DB.Where("id = ?", fallbackCompatibleChannelID).Delete(&model.Channel{}).Error
	require.NoError(t, err, "failed to clean openai-compatible channel fixture")
	compatibleChannel := &model.Channel{Id: fallbackCompatibleChannelID, Type: channeltype.OpenAICompatible, Name: "compatible-fallback", Status: model.ChannelStatusEnabled}
	err = model.DB.Create(compatibleChannel).Error
	require.NoError(t, err, "failed to create openai-compatible channel fixture")

	err = model.DB.Where("id = ?", fallbackAnthropicChannelID).Delete(&model.Channel{}).Error
	require.NoError(t, err, "failed to clean anthropic channel fixture")
	anthropicChannel := &model.Channel{Id: fallbackAnthropicChannelID, Type: channeltype.Anthropic, Name: "anthropic-fallback", Status: model.ChannelStatusEnabled}
	err = model.DB.Create(anthropicChannel).Error
	require.NoError(t, err, "failed to create anthropic channel fixture")
}

func ensureResponseFallbackDB(t *testing.T) {
	t.Helper()
	if model.DB != nil {
		if model.LOG_DB == nil {
			model.LOG_DB = model.DB
		}
		return
	}
	db, err := gorm.Open(sqlite.Open("file:response_fallback_tests?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err, "failed to open sqlite database")
	model.DB = db
	model.LOG_DB = db
}
