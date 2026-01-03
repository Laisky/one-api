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
