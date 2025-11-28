package openai

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/channeltype"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// TestDoResponseClaudeStreamingSkipsConversion verifies that streaming Claude conversions
// do not attempt to convert the response a second time, which would re-read a closed body.
func TestDoResponseClaudeStreamingSkipsConversion(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Set(ctxkey.ClaudeMessagesConversion, true)

	db, err := gorm.Open(sqlite.Open("file:claude_stream_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Trace{}))
	origDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = origDB
	})
	origApprox := config.ApproximateTokenEnabled
	config.ApproximateTokenEnabled = true
	t.Cleanup(func() {
		config.ApproximateTokenEnabled = origApprox
	})

	meta := &metalib.Meta{
		Mode:            relaymode.ClaudeMessages,
		ChannelType:     channeltype.OpenAI,
		APIType:         channeltype.OpenAI,
		IsStream:        true,
		OriginModelName: "gpt-4o-mini",
		ActualModelName: "gpt-4o-mini",
		StartTime:       time.Now(),
	}
	metalib.Set2Context(c, meta)

	adaptor := &Adaptor{}
	adaptor.Init(meta)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("data: [DONE]\n\n")),
	}

	usage, respErr := adaptor.DoResponse(c, resp, meta)
	require.Nil(t, respErr)
	require.NotNil(t, usage)

	_, exists := c.Get(ctxkey.ConvertedResponse)
	require.False(t, exists)
	require.Contains(t, recorder.Body.String(), "data: [DONE]")
}

func TestDoResponseClaudeStreamingConvertsToClaudeSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Set(ctxkey.ClaudeMessagesConversion, true)

	meta := &metalib.Meta{
		Mode:            relaymode.ClaudeMessages,
		ChannelType:     channeltype.OpenAI,
		APIType:         channeltype.OpenAI,
		IsStream:        true,
		OriginModelName: "gpt-4o-mini",
		ActualModelName: "gpt-4o-mini",
		PromptTokens:    5,
		StartTime:       time.Now(),
	}
	metalib.Set2Context(c, meta)

	adaptor := &Adaptor{}
	adaptor.Init(meta)

	upstream := "data: {\"id\":\"chunk\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Hello\"}}]}\n\ndata: [DONE]\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstream)),
	}

	usage, respErr := adaptor.DoResponse(c, resp, meta)
	require.Nil(t, respErr)
	require.NotNil(t, usage)
	require.Greater(t, usage.TotalTokens, 0)

	body := recorder.Body.String()
	require.Contains(t, body, "\"type\":\"message_start\"")
	require.Contains(t, body, "\"type\":\"content_block_delta\"")
	require.Contains(t, body, "Hello")
	require.Contains(t, body, "data: [DONE]")
}

func TestDoResponseClaudeNonStreamDefersToConverter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Set(ctxkey.ClaudeMessagesConversion, true)

	meta := &metalib.Meta{
		Mode:            relaymode.ClaudeMessages,
		ChannelType:     channeltype.OpenAI,
		APIType:         channeltype.OpenAI,
		IsStream:        false,
		OriginModelName: "gpt-4o-mini",
		ActualModelName: "gpt-4o-mini",
	}
	metalib.Set2Context(c, meta)

	adaptor := &Adaptor{}
	adaptor.Init(meta)

	payload := `{"id":"resp_123","object":"response","model":"gpt-4o-mini","output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"get_weather","arguments":"{\"location\":\"San Francisco, CA\"}"}],"usage":{"input_tokens":12,"output_tokens":4}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(payload)),
	}

	usage, respErr := adaptor.DoResponse(c, resp, meta)
	require.Nil(t, respErr)
	require.Nil(t, usage)
	require.Equal(t, 0, recorder.Body.Len(), "handler should not have written directly")

	convertedAny, exists := c.Get(ctxkey.ConvertedResponse)
	require.True(t, exists)
	convertedResp := convertedAny.(*http.Response)
	bodyBytes, readErr := io.ReadAll(convertedResp.Body)
	require.NoError(t, readErr)
	convertedResp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	require.Contains(t, string(bodyBytes), "\"tool_use\"")
	require.Contains(t, string(bodyBytes), "get_weather")
}
