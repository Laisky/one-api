package cloudflare

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	rootmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGetRequestURL verifies that every supported relay mode uses a matching
// Cloudflare OpenAI-compatible endpoint and that supported base URL forms are
// normalized without duplicated path segments.
func TestGetRequestURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		baseURL   string
		accountID string
		mode      int
		want      string
	}{
		{
			name:      "standard chat",
			baseURL:   "https://api.cloudflare.com",
			accountID: "account-id",
			mode:      relaymode.ChatCompletions,
			want:      "https://api.cloudflare.com/client/v4/accounts/account-id/ai/v1/chat/completions",
		},
		{
			name:      "standard trailing slash",
			baseURL:   "https://api.cloudflare.com/",
			accountID: "account-id",
			mode:      relaymode.Embeddings,
			want:      "https://api.cloudflare.com/client/v4/accounts/account-id/ai/v1/embeddings",
		},
		{
			name:    "full account base",
			baseURL: "https://api.cloudflare.com/client/v4/accounts/path-account/ai",
			mode:    relaymode.ResponseAPI,
			want:    "https://api.cloudflare.com/client/v4/accounts/path-account/ai/v1/responses",
		},
		{
			name:    "full account OpenAI base",
			baseURL: "https://api.cloudflare.com/client/v4/accounts/path-account/ai/v1/",
			mode:    relaymode.ChatCompletions,
			want:    "https://api.cloudflare.com/client/v4/accounts/path-account/ai/v1/chat/completions",
		},
		{
			name:      "client API base",
			baseURL:   "https://api.cloudflare.com/client/v4",
			accountID: "account-id",
			mode:      relaymode.Completions,
			want:      "https://api.cloudflare.com/client/v4/accounts/account-id/ai/v1/chat/completions",
		},
		{
			name:    "account base",
			baseURL: "https://api.cloudflare.com/client/v4/accounts/path-account",
			mode:    relaymode.ClaudeMessages,
			want:    "https://api.cloudflare.com/client/v4/accounts/path-account/ai/v1/chat/completions",
		},
		{
			name:    "legacy AI Gateway",
			baseURL: "https://gateway.ai.cloudflare.com/v1/account/gateway/workers-ai",
			mode:    relaymode.ChatCompletions,
			want:    "https://gateway.ai.cloudflare.com/v1/account/gateway/workers-ai/v1/chat/completions",
		},
		{
			name:    "legacy AI Gateway OpenAI base",
			baseURL: "https://gateway.ai.cloudflare.com/v1/account/gateway/workers-ai/v1",
			mode:    relaymode.Embeddings,
			want:    "https://gateway.ai.cloudflare.com/v1/account/gateway/workers-ai/v1/embeddings",
		},
		{
			name:      "gateway lookalike is not trusted as a legacy gateway",
			baseURL:   "https://gateway.ai.cloudflare.com.example.test/proxy",
			accountID: "account-id",
			mode:      relaymode.ChatCompletions,
			want:      "https://gateway.ai.cloudflare.com.example.test/proxy/client/v4/accounts/account-id/ai/v1/chat/completions",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := (&Adaptor{}).GetRequestURL(&meta.Meta{
				Mode:    test.mode,
				BaseURL: test.baseURL,
				Config:  rootmodel.ChannelConfig{UserID: test.accountID},
			})
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

// TestGetRequestURLRejectsInvalidConfiguration verifies that malformed base
// URLs, missing account identifiers, and unsupported relay modes fail closed.
func TestGetRequestURLRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	_, err := adaptor.GetRequestURL(nil)
	require.Error(t, err)

	tests := []struct {
		name      string
		baseURL   string
		accountID string
		mode      int
	}{
		{name: "missing account ID", baseURL: "https://api.cloudflare.com", mode: relaymode.ChatCompletions},
		{name: "invalid scheme", baseURL: "ftp://api.cloudflare.com", accountID: "account-id", mode: relaymode.ChatCompletions},
		{name: "embedded credentials", baseURL: "https://user:pass@api.cloudflare.com", accountID: "account-id", mode: relaymode.ChatCompletions},
		{name: "query string", baseURL: "https://api.cloudflare.com?debug=true", accountID: "account-id", mode: relaymode.ChatCompletions},
		{name: "invalid account ID", baseURL: "https://api.cloudflare.com", accountID: "bad/id", mode: relaymode.ChatCompletions},
		{name: "invalid gateway path", baseURL: "https://gateway.ai.cloudflare.com/v1/account/gateway", mode: relaymode.ChatCompletions},
		{name: "unsupported mode", baseURL: "https://api.cloudflare.com", accountID: "account-id", mode: relaymode.ImagesGenerations},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := adaptor.GetRequestURL(&meta.Meta{
				Mode:    test.mode,
				BaseURL: test.baseURL,
				Config:  rootmodel.ChannelConfig{UserID: test.accountID},
			})
			require.Error(t, err)
		})
	}
}

// TestConvertCompletionToChatRequest verifies that the legacy Completions input
// is converted to Chat Completions without mutating the caller or dropping
// shared sampling and streaming fields.
func TestConvertCompletionToChatRequest(t *testing.T) {
	t.Parallel()

	temperature := 0.25
	topP := 0.9
	n := 2
	request := &relaymodel.GeneralOpenAIRequest{
		Model:       "@cf/qwen/qwen3.8-27b",
		Prompt:      "  preserve prompt whitespace  ",
		MaxTokens:   128,
		Temperature: &temperature,
		TopP:        &topP,
		Stop:        []string{"END"},
		Stream:      true,
		N:           &n,
	}

	convertedAny, err := (&Adaptor{}).ConvertRequest(nil, relaymode.Completions, request)
	require.NoError(t, err)
	converted, ok := convertedAny.(*relaymodel.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotSame(t, request, converted)
	require.Empty(t, converted.Prompt)
	require.Len(t, converted.Messages, 1)
	require.Equal(t, "user", converted.Messages[0].Role)
	require.Equal(t, request.Prompt, converted.Messages[0].Content)
	require.Equal(t, request.MaxTokens, converted.MaxTokens)
	require.Equal(t, request.Temperature, converted.Temperature)
	require.Equal(t, request.TopP, converted.TopP)
	require.Equal(t, request.Stop, converted.Stop)
	require.Equal(t, request.Stream, converted.Stream)
	require.Equal(t, request.N, converted.N)
	require.Equal(t, "  preserve prompt whitespace  ", request.Prompt, "conversion must not mutate the original request")

	_, err = (&Adaptor{}).ConvertRequest(nil, relaymode.Completions, &relaymodel.GeneralOpenAIRequest{Prompt: "   "})
	require.Error(t, err)
}

// TestConvertClaudeRequest verifies that Claude Messages input is converted by
// the shared OpenAI-compatible bridge and records the response-conversion marker.
func TestConvertClaudeRequest(t *testing.T) {
	t.Parallel()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	stream := true

	convertedAny, err := (&Adaptor{}).ConvertClaudeRequest(ctx, &relaymodel.ClaudeRequest{
		Model:     "@cf/qwen/qwen3.8-27b",
		MaxTokens: 256,
		System:    "Follow the instructions.",
		Messages: []relaymodel.ClaudeMessage{
			{Role: "user", Content: "Hello"},
		},
		Stream: &stream,
		Tools: []relaymodel.ClaudeTool{
			{
				Name:        "lookup",
				Description: "Look up an item.",
				InputSchema: map[string]any{"type": "object"},
			},
		},
	})
	require.NoError(t, err)

	converted, ok := convertedAny.(*relaymodel.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, "@cf/qwen/qwen3.8-27b", converted.Model)
	require.True(t, converted.Stream)
	require.NotNil(t, converted.MaxCompletionTokens)
	require.Equal(t, 256, *converted.MaxCompletionTokens)
	require.Len(t, converted.Messages, 2)
	require.Equal(t, "system", converted.Messages[0].Role)
	require.Equal(t, "Follow the instructions.", converted.Messages[0].Content)
	require.Equal(t, "user", converted.Messages[1].Role)
	require.Len(t, converted.Tools, 1)
	require.True(t, ctx.GetBool(ctxkey.ClaudeMessagesConversion))
}

// TestDoResponseConvertsChatResponseToClaude verifies the response half of the
// Cloudflare Claude Messages bridge, including usage and finish-reason mapping.
func TestDoResponseConvertsChatResponseToClaude(t *testing.T) {
	t.Parallel()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, err := (&Adaptor{}).ConvertClaudeRequest(ctx, &relaymodel.ClaudeRequest{
		Model:     "@cf/qwen/qwen3.8-27b",
		MaxTokens: 32,
		Messages:  []relaymodel.ClaudeMessage{{Role: "user", Content: "Hello"}},
	})
	require.NoError(t, err)

	body := `{"id":"chatcmpl_cf","object":"chat.completion","created":1,"model":"@cf/qwen/qwen3.8-27b","choices":[{"index":0,"message":{"role":"assistant","content":"Hello from Cloudflare"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	usage, responseErr := (&Adaptor{}).DoResponse(ctx, resp, &meta.Meta{
		Mode:            relaymode.ClaudeMessages,
		ActualModelName: "@cf/qwen/qwen3.8-27b",
		PromptTokens:    4,
	})
	require.Nil(t, responseErr)
	require.Nil(t, usage)

	raw, exists := ctx.Get(ctxkey.ConvertedResponse)
	require.True(t, exists)
	convertedResp, ok := raw.(*http.Response)
	require.True(t, ok)
	convertedBody, err := io.ReadAll(convertedResp.Body)
	require.NoError(t, err)

	var converted relaymodel.ClaudeResponse
	require.NoError(t, json.Unmarshal(convertedBody, &converted))
	require.Equal(t, "message", converted.Type)
	require.Equal(t, "assistant", converted.Role)
	require.Equal(t, "end_turn", converted.StopReason)
	require.Equal(t, 4, converted.Usage.InputTokens)
	require.Equal(t, 3, converted.Usage.OutputTokens)
	require.Len(t, converted.Content, 1)
	require.Equal(t, "text", converted.Content[0].Type)
	require.Equal(t, "Hello from Cloudflare", converted.Content[0].Text)
}
