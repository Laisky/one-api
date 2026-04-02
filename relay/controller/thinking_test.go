package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	openaipayload "github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/apitype"
	"github.com/songquanpeng/one-api/relay/channeltype"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestApplyThinkingQueryToChatRequestSetsReasoningEffort(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?thinking=true", nil)
	c.Request = req

	meta := &metalib.Meta{ActualModelName: "gpt-5", APIType: apitype.OpenAI, ChannelType: channeltype.OpenAI}
	payload := &relaymodel.GeneralOpenAIRequest{Model: "gpt-5"}

	applyThinkingQueryToChatRequest(c, payload, meta)

	require.NotNil(t, payload.ReasoningEffort)
	require.Equal(t, "high", *payload.ReasoningEffort)
}

func TestApplyThinkingQueryClampsMediumOnlyReasoningModels(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?thinking=true&reasoning_effort=high", nil)
	c.Request = req

	meta := &metalib.Meta{ActualModelName: "o4-mini", APIType: apitype.OpenAI, ChannelType: channeltype.OpenAI}
	payload := &relaymodel.GeneralOpenAIRequest{Model: "o4-mini"}

	applyThinkingQueryToChatRequest(c, payload, meta)

	require.NotNil(t, payload.ReasoningEffort)
	require.Equal(t, "medium", *payload.ReasoningEffort)
}

func TestApplyThinkingQueryRespectsUserProvidedEffort(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?thinking=true", nil)
	c.Request = req

	existing := "low"
	meta := &metalib.Meta{ActualModelName: "gpt-5", APIType: apitype.OpenAI, ChannelType: channeltype.OpenAI}
	payload := &relaymodel.GeneralOpenAIRequest{Model: "gpt-5", ReasoningEffort: &existing}

	applyThinkingQueryToChatRequest(c, payload, meta)

	require.Equal(t, &existing, payload.ReasoningEffort)
}

func TestApplyThinkingQueryHonorsReasoningEffortOverride(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?thinking=true&reasoning_effort=medium", nil)
	c.Request = req

	meta := &metalib.Meta{ActualModelName: "o4-mini-deep-research", APIType: apitype.OpenAI, ChannelType: channeltype.OpenAI}
	payload := &relaymodel.GeneralOpenAIRequest{Model: "o4-mini-deep-research"}

	applyThinkingQueryToChatRequest(c, payload, meta)

	require.NotNil(t, payload.ReasoningEffort)
	require.Equal(t, "medium", *payload.ReasoningEffort)
}

func TestApplyThinkingQuerySetsIncludeReasoningForOpenRouter(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?thinking=true", nil)
	c.Request = req

	meta := &metalib.Meta{ActualModelName: "grok-3", APIType: apitype.OpenAI, ChannelType: channeltype.OpenRouter}
	payload := &relaymodel.GeneralOpenAIRequest{Model: "grok-3"}

	applyThinkingQueryToChatRequest(c, payload, meta)

	require.NotNil(t, payload.IncludeReasoning)
	require.True(t, *payload.IncludeReasoning)
}

func TestApplyThinkingQueryToResponseRequest(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses?thinking=true", nil)
	c.Request = req

	meta := &metalib.Meta{ActualModelName: "gpt-5", APIType: apitype.OpenAI, ChannelType: channeltype.OpenAI}
	payload := &openaipayload.ResponseAPIRequest{Model: "gpt-5"}

	applyThinkingQueryToResponseRequest(c, payload, meta)

	require.NotNil(t, payload.Reasoning)
	require.NotNil(t, payload.Reasoning.Effort)
	require.Equal(t, "high", *payload.Reasoning.Effort)
}

func TestApplyThinkingQueryToResponseRequestClampsMediumOnlyModels(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses?thinking=true&reasoning_effort=high", nil)
	c.Request = req

	meta := &metalib.Meta{ActualModelName: "gpt-5.1-chat-latest", APIType: apitype.OpenAI, ChannelType: channeltype.OpenAI}
	payload := &openaipayload.ResponseAPIRequest{Model: "gpt-5.1-chat-latest"}

	applyThinkingQueryToResponseRequest(c, payload, meta)

	require.NotNil(t, payload.Reasoning)
	require.NotNil(t, payload.Reasoning.Effort)
	require.Equal(t, "medium", *payload.Reasoning.Effort)
}

func TestApplyThinkingQueryToChatRequestSetsQwenDisableOverride(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?thinking=false", nil)
	c.Request = req

	meta := &metalib.Meta{ActualModelName: "Qwen/Qwen3.5-35B-A3B", APIType: apitype.OpenAI, ChannelType: channeltype.OpenAICompatible}
	payload := &relaymodel.GeneralOpenAIRequest{Model: "Qwen/Qwen3.5-35B-A3B"}

	applyThinkingQueryToChatRequest(c, payload, meta)

	kwargs := payload.ExtraBody["chat_template_kwargs"].(map[string]any)
	require.Equal(t, false, kwargs["enable_thinking"])
	require.Nil(t, payload.ReasoningEffort)
}

func TestApplyThinkingQueryToChatRequestPreservesExistingQwenOverride(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?thinking=true", nil)
	c.Request = req

	meta := &metalib.Meta{ActualModelName: "Qwen/Qwen3.5-35B-A3B", APIType: apitype.OpenAI, ChannelType: channeltype.OpenAICompatible}
	payload := &relaymodel.GeneralOpenAIRequest{
		Model: "Qwen/Qwen3.5-35B-A3B",
		ExtraBody: map[string]any{
			"chat_template_kwargs": map[string]any{"enable_thinking": false},
		},
	}

	applyThinkingQueryToChatRequest(c, payload, meta)

	kwargs := payload.ExtraBody["chat_template_kwargs"].(map[string]any)
	require.Equal(t, false, kwargs["enable_thinking"])
}

func TestApplyThinkingQueryToResponseRequestSetsQwenEnableOverride(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses?thinking=true", nil)
	c.Request = req

	meta := &metalib.Meta{ActualModelName: "Qwen/Qwen3.5-35B-A3B", APIType: apitype.OpenAI, ChannelType: channeltype.OpenAICompatible}
	payload := &openaipayload.ResponseAPIRequest{Model: "Qwen/Qwen3.5-35B-A3B"}

	applyThinkingQueryToResponseRequest(c, payload, meta)

	kwargs := payload.ExtraBody["chat_template_kwargs"].(map[string]any)
	require.Equal(t, true, kwargs["enable_thinking"])
}

func TestApplyThinkingQueryToChatRequestSkipsQwenOverrideForHostedProviders(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?thinking=false", nil)
	c.Request = req

	meta := &metalib.Meta{
		ActualModelName: "Qwen/Qwen3.5-35B-A3B",
		APIType:         apitype.OpenAI,
		ChannelType:     channeltype.OpenAI,
		BaseURL:         "https://api.openai.com",
	}
	payload := &relaymodel.GeneralOpenAIRequest{Model: "Qwen/Qwen3.5-35B-A3B"}

	applyThinkingQueryToChatRequest(c, payload, meta)

	require.Nil(t, payload.ExtraBody)
	require.Nil(t, payload.ReasoningEffort)
}

func TestApplyThinkingQueryToChatRequestSkipsQwenOverrideForAliBailian(t *testing.T) {
	t.Parallel()
	// Ali Bailian uses its own enable_thinking passthrough at root level,
	// NOT the vLLM chat_template_kwargs mechanism. Verify the override is skipped.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?thinking=false", nil)
	c.Request = req

	meta := &metalib.Meta{
		ActualModelName: "qwen3.5-27b",
		APIType:         apitype.OpenAI,
		ChannelType:     channeltype.AliBailian,
		BaseURL:         "https://dashscope.aliyuncs.com",
	}
	payload := &relaymodel.GeneralOpenAIRequest{Model: "qwen3.5-27b"}

	applyThinkingQueryToChatRequest(c, payload, meta)

	// ExtraBody should remain nil — vLLM override must NOT be applied for AliBailian.
	// DashScope uses root-level enable_thinking which is handled by the passthrough layer.
	require.Nil(t, payload.ExtraBody)
	require.Nil(t, payload.ReasoningEffort)
}

func TestApplyThinkingQueryToChatRequestAllowsQwenOverrideForVLLMBaseURL(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?thinking=true", nil)
	c.Request = req

	meta := &metalib.Meta{
		ActualModelName: "Qwen/Qwen3.5-35B-A3B",
		APIType:         apitype.OpenAI,
		ChannelType:     channeltype.OpenAI,
		BaseURL:         "http://vllm.internal:8000/v1",
	}
	payload := &relaymodel.GeneralOpenAIRequest{Model: "Qwen/Qwen3.5-35B-A3B"}

	applyThinkingQueryToChatRequest(c, payload, meta)

	kwargs := payload.ExtraBody["chat_template_kwargs"].(map[string]any)
	require.Equal(t, true, kwargs["enable_thinking"])
}
