package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestResponseFallbackClaudeMaxOutputDefault pins when a Claude-lowered
// fallback request gets the model's maximum output: only when the client set
// no limit, only for Claude targets, from the channel config before the
// catalog, and never past the context window left after the prompt.
func TestResponseFallbackClaudeMaxOutputDefault(t *testing.T) {
	t.Parallel()
	claude := &metalib.Meta{APIType: apitype.Anthropic, ChannelType: channeltype.Anthropic, ActualModelName: "claude-opus-5-5"}
	limit := 64
	cases := []struct {
		name    string
		meta    *metalib.Meta
		request *openai.ResponseAPIRequest
		chat    *relaymodel.GeneralOpenAIRequest
		configs map[string]model.ModelConfigLocal
		prompt  int
		want    int
	}{
		{name: "catalog_maximum", meta: claude, request: &openai.ResponseAPIRequest{}, chat: &relaymodel.GeneralOpenAIRequest{}, prompt: 1000, want: 128000},
		{name: "client_limit_kept", meta: claude, request: &openai.ResponseAPIRequest{MaxOutputTokens: &limit}, chat: &relaymodel.GeneralOpenAIRequest{MaxTokens: 64}, prompt: 1000, want: 0},
		{name: "chat_limit_kept", meta: claude, request: &openai.ResponseAPIRequest{}, chat: &relaymodel.GeneralOpenAIRequest{MaxTokens: 512}, prompt: 1000, want: 0},
		{name: "channel_config_first", meta: claude, request: &openai.ResponseAPIRequest{}, chat: &relaymodel.GeneralOpenAIRequest{},
			configs: map[string]model.ModelConfigLocal{"claude-opus-5-5": {Ratio: 1, MaxTokens: 32000}}, prompt: 1000, want: 32000},
		{name: "context_window_cap", meta: claude, request: &openai.ResponseAPIRequest{}, chat: &relaymodel.GeneralOpenAIRequest{},
			configs: map[string]model.ModelConfigLocal{"claude-opus-5-5": {Ratio: 1, ContextLength: 200000, MaxTokens: 64000}}, prompt: 190000, want: 10000},
		{name: "context_window_full", meta: claude, request: &openai.ResponseAPIRequest{}, chat: &relaymodel.GeneralOpenAIRequest{},
			configs: map[string]model.ModelConfigLocal{"claude-opus-5-5": {Ratio: 1, ContextLength: 200000, MaxTokens: 64000}}, prompt: 200000, want: 0},
		{name: "unknown_model", meta: &metalib.Meta{APIType: apitype.Anthropic, ActualModelName: "claude-unlisted"}, request: &openai.ResponseAPIRequest{}, chat: &relaymodel.GeneralOpenAIRequest{}, prompt: 10, want: 0},
		{name: "chat_upstream_untouched", meta: &metalib.Meta{APIType: apitype.OpenAI, ChannelType: channeltype.OpenAICompatible, ActualModelName: "claude-opus-5-5"},
			request: &openai.ResponseAPIRequest{}, chat: &relaymodel.GeneralOpenAIRequest{}, prompt: 10, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, responseFallbackClaudeMaxOutput(tc.meta, tc.request, tc.chat, tc.configs, nil, tc.prompt))
		})
	}
}

// TestResponseFallbackClaudeMaxOutputHTTP relays a Responses request without
// max_output_tokens (as Codex sends) to an Anthropic channel and proves the
// upstream Claude request asks for the model's 128K maximum instead of the
// 2048-token DEFAULT_MAX_TOKEN, while the quota hold keeps its prompt-only
// basis (128K output at completion ratio 2 would hold 256K quota).
func TestResponseFallbackClaudeMaxOutputHTTP(t *testing.T) {
	const balance = int64(1_000_000)
	for _, stream := range []bool{true, false} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			xaiVideoSetup(t, balance, false)
			provider, observed, calls := codexFallbackProvider(t, channeltype.Anthropic, stream)
			body := fmt.Sprintf(`{"model":"alias","stream":%t,"input":"Write the whole patch."}`, stream)
			c, recorder, requestID := protocolContext(t, channeltype.Anthropic, "claude-opus-5-5", "/v1/responses", body, provider.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2})
			responseNamespaceCleanLedger(t, requestID)
			require.Nil(t, RelayResponseAPIHelper(c))
			drainCriticalTasks(t)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			require.EqualValues(t, 1, calls.Load())
			got := <-observed
			require.NoError(t, got.Err)

			var upstream struct {
				MaxTokens int `json:"max_tokens"`
			}
			require.NoError(t, json.Unmarshal(got.Body, &upstream))
			require.Equal(t, 128000, upstream.MaxTokens, "an omitted max_output_tokens means the model maximum")
			held := c.GetInt64(ctxkey.PreConsumedQuotaAmount)
			require.Positive(t, held)
			require.Less(t, held, int64(1000), "the hold stays prompt-only, as before")
		})
	}
}
