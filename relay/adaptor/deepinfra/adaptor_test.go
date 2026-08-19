package deepinfra

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGetRequestURL verifies every DeepInfra relay surface and custom base URL normalization.
func TestGetRequestURL(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	tests := []struct {
		name      string
		mode      int
		modelName string
		wantURL   string
	}{
		{name: "chat", mode: relaymode.ChatCompletions, wantURL: "https://api.deepinfra.com/v1/openai/chat/completions"},
		{name: "responses fallback", mode: relaymode.ResponseAPI, wantURL: "https://api.deepinfra.com/v1/openai/chat/completions"},
		{name: "completions", mode: relaymode.Completions, wantURL: "https://api.deepinfra.com/v1/openai/completions"},
		{name: "embeddings", mode: relaymode.Embeddings, wantURL: "https://api.deepinfra.com/v1/openai/embeddings"},
		{name: "image generation", mode: relaymode.ImagesGenerations, wantURL: "https://api.deepinfra.com/v1/openai/images/generations"},
		{name: "image edit", mode: relaymode.ImagesEdits, wantURL: "https://api.deepinfra.com/v1/images/edits"},
		{name: "speech", mode: relaymode.AudioSpeech, wantURL: "https://api.deepinfra.com/v1/audio/speech"},
		{name: "transcription", mode: relaymode.AudioTranscription, wantURL: "https://api.deepinfra.com/v1/audio/transcriptions"},
		{name: "translation", mode: relaymode.AudioTranslation, wantURL: "https://api.deepinfra.com/v1/audio/translations"},
		{name: "claude messages", mode: relaymode.ClaudeMessages, wantURL: "https://api.deepinfra.com/anthropic/v1/messages"},
		{name: "rerank", mode: relaymode.Rerank, modelName: "Qwen/Qwen3-Reranker-8B", wantURL: "https://api.deepinfra.com/v1/inference/Qwen/Qwen3-Reranker-8B"},
		{name: "versioned rerank", mode: relaymode.Rerank, modelName: "Qwen/Qwen3-Reranker-8B:2026-08-01", wantURL: "https://api.deepinfra.com/v1/inference/Qwen/Qwen3-Reranker-8B:2026-08-01"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			url, err := adaptor.GetRequestURL(&meta.Meta{
				Mode:            test.mode,
				BaseURL:         " https://api.deepinfra.com/ ",
				ActualModelName: test.modelName,
			})
			require.NoError(t, err)
			require.Equal(t, test.wantURL, url)
		})
	}
}

// TestGetRequestURLRejectsInvalidInputs verifies unsupported modes and unsafe model paths.
func TestGetRequestURLRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	_, err := adaptor.GetRequestURL(nil)
	require.Error(t, err)

	_, err = adaptor.GetRequestURL(&meta.Meta{Mode: relaymode.Moderations, BaseURL: "https://api.deepinfra.com"})
	require.ErrorContains(t, err, "unsupported DeepInfra relay mode")

	_, err = adaptor.GetRequestURL(&meta.Meta{Mode: relaymode.Rerank, BaseURL: "https://api.deepinfra.com", ActualModelName: "../secret"})
	require.Error(t, err)
}

// TestSetupRequestHeader verifies Bearer auth and native Anthropic headers.
func TestSetupRequestHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	clientRequest := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	clientRequest.Header.Set("anthropic-version", "2024-01-01")
	clientRequest.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = clientRequest

	upstreamRequest := httptest.NewRequest(http.MethodPost, "https://api.deepinfra.com/anthropic/v1/messages", nil)
	adaptor := &Adaptor{}
	err := adaptor.SetupRequestHeader(context, upstreamRequest, &meta.Meta{
		Mode:   relaymode.ClaudeMessages,
		APIKey: "deepinfra-token",
	})
	require.NoError(t, err)
	require.Equal(t, "Bearer deepinfra-token", upstreamRequest.Header.Get("Authorization"))
	require.Equal(t, "2024-01-01", upstreamRequest.Header.Get("anthropic-version"))
	require.Equal(t, "prompt-caching-2024-07-31", upstreamRequest.Header.Get("anthropic-beta"))
}

// TestConvertClaudeRequest verifies that native Messages passthrough state is enabled.
func TestConvertClaudeRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	request := &model.ClaudeRequest{Model: "Qwen/Qwen3.8-Max"}

	converted, err := (&Adaptor{}).ConvertClaudeRequest(context, request)
	require.NoError(t, err)
	require.Same(t, request, converted)
	require.Equal(t, request.Model, context.GetString(ctxkey.ClaudeModel))
	require.True(t, context.GetBool(ctxkey.ClaudeMessagesNative))
	require.True(t, context.GetBool(ctxkey.ClaudeDirectPassthrough))
}
