package controller

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openrouter"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestClaude431ReviewProviderIsolation checks the real post-conversion JSON boundary.
// Claude controls remain available to Claude DTOs but must not leak into shared Chat DTOs.
func TestClaude431ReviewProviderIsolation(t *testing.T) {
	for _, tc := range []struct {
		name, model  string
		api, channel int
		adaptor      adaptor.Adaptor
		claude       bool
	}{
		{"openrouter_claude", "anthropic/claude-sonnet-5.5", apitype.OpenRouter, channeltype.OpenRouter, &openrouter.Adaptor{}, false},
		{"openrouter_other", "openai/gpt-4.1", apitype.OpenRouter, channeltype.OpenRouter, &openrouter.Adaptor{}, false},
		{"openai", "gpt-4.1", apitype.OpenAI, channeltype.OpenAI, &openai.Adaptor{ChannelType: channeltype.OpenAI}, false},
		{"native_claude", "claude-sonnet-5-5", apitype.Anthropic, channeltype.Anthropic, &anthropic.Adaptor{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{"model":"` + tc.model + `","max_tokens":4096,"messages":[{"role":"user","content":"hello"}],"thinking":{"type":"adaptive","display":"omitted","future":{"id":9007199254740993}},"output_config":{"effort":"high"}}`
			var request relaymodel.GeneralOpenAIRequest
			require.NoError(t, json.Unmarshal([]byte(raw), &request))
			before, err := json.Marshal(request.Thinking)
			require.NoError(t, err)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(raw))
			m := &metalib.Meta{APIType: tc.api, ChannelType: tc.channel, ActualModelName: tc.model, OriginModelName: tc.model, Mode: relaymode.ChatCompletions}
			metalib.Set2Context(c, m)
			body, err := getRequestBody(c, m, &request, tc.adaptor, false)
			require.NoError(t, err)
			encoded, err := io.ReadAll(body)
			require.NoError(t, err)
			var payload map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(encoded, &payload))
			if tc.claude {
				require.JSONEq(t, `{"effort":"high"}`, string(payload["output_config"]))
				require.Contains(t, string(payload["thinking"]), `"display":"omitted"`)
				require.Contains(t, string(payload["thinking"]), `9007199254740993`)
			} else {
				require.NotContains(t, payload, "output_config")
				require.NotContains(t, string(payload["thinking"]), "display")
				require.NotContains(t, string(payload["thinking"]), "future")
				converted, ok := c.Get(ctxkey.ConvertedRequest)
				require.True(t, ok)
				switch wire := converted.(type) {
				case *relaymodel.GeneralOpenAIRequest:
					require.Empty(t, wire.OutputConfig)
					if wire.Thinking != nil {
						require.Empty(t, wire.Thinking.ExtraFields)
					}
				case *openai.ResponseAPIRequest:
					// GPT-4.1 selects the provider's native Responses DTO.
					require.Empty(t, wire.OutputConfig)
					require.Nil(t, wire.Thinking)
				default:
					t.Fatalf("unexpected upstream DTO %T", converted)
				}
				// Sanitizing the outgoing DTO must not destroy caller-owned provider fields.
				after, err := json.Marshal(request.Thinking)
				require.NoError(t, err)
				require.JSONEq(t, string(before), string(after))
				require.JSONEq(t, `{"effort":"high"}`, string(request.OutputConfig))
			}
		})
	}
}

// TestClaude431ReviewSanitizerOwnership pins nil/value behavior and nested copy isolation.
// A common DTO is sanitized, not mutated; native Claude DTOs keep their own full contract.
func TestClaude431ReviewSanitizerOwnership(t *testing.T) {
	t.Parallel()
	var nilChat *relaymodel.GeneralOpenAIRequest
	require.Nil(t, sanitizeConvertedChatFields(nil))
	require.Equal(t, nilChat, sanitizeConvertedChatFields(nilChat))
	var request relaymodel.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal([]byte(`{"thinking":{"type":"enabled","budget_tokens":2048,"block_binding":{"prefix_mismatch_behavior":"error"},"future":true},"output_config":{"effort":"high"}}`), &request))
	for _, value := range []any{request, &request} {
		out := sanitizeConvertedChatFields(value)
		var got relaymodel.GeneralOpenAIRequest
		switch typed := out.(type) {
		case relaymodel.GeneralOpenAIRequest:
			got = typed
		case *relaymodel.GeneralOpenAIRequest:
			got = *typed
		default:
			t.Fatalf("unexpected DTO %T", out)
		}
		require.Empty(t, got.OutputConfig)
		require.Empty(t, got.Thinking.ExtraFields)
		require.Nil(t, got.Thinking.BlockBinding)
		require.Equal(t, "enabled", got.Thinking.Type)
		require.Equal(t, 2048, *got.Thinking.BudgetTokens)
		require.NotSame(t, request.Thinking, got.Thinking)
		got.Thinking.Type = "disabled"
		require.Equal(t, "enabled", request.Thinking.Type)
		require.NotEmpty(t, request.Thinking.ExtraFields)
		require.NotNil(t, request.Thinking.BlockBinding)
		require.NotEmpty(t, request.OutputConfig)
	}
	native := &anthropic.Request{Thinking: request.Thinking, OutputConfig: request.OutputConfig}
	require.Same(t, native, sanitizeConvertedChatFields(native))
}
