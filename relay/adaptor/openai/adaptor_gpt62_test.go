package openai

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGPT62CatalogAndReasoning verifies public discovery and the published effort
// contract without inventing native Pro IDs or snapshots. Parameter t asserts.
func TestGPT62CatalogAndReasoning(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"gpt-6.2-astra", "gpt-6.2-sol"} {
		require.Contains(t, (&Adaptor{ChannelType: channeltype.OpenAI}).GetModelList(), name)
		cfg := ModelRatios[name]
		require.EqualValues(t, 1_050_000, cfg.ContextLength)
		require.EqualValues(t, 128_000, cfg.MaxOutputTokens)
		require.Contains(t, cfg.Description, "922,000")
		require.Equal(t, []string{"text", "image"}, cfg.InputModalities)
		require.Equal(t, []string{"text"}, cfg.OutputModalities)
		require.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, cfg.SupportedReasoningEfforts)
		require.Equal(t, "medium", cfg.DefaultReasoningEffort)
		for _, effort := range cfg.SupportedReasoningEfforts {
			requested := " " + strings.ToUpper(effort) + " "
			require.Equal(t, effort, *normalizeReasoningEffortForModel(name, &requested))
			require.False(t, modelSupportsSampling(name, &requested))
		}
		for _, effort := range []string{"none", "minimal", "ultra"} {
			require.False(t, isReasoningEffortAllowedForModel(name, effort))
			require.Equal(t, "medium", *normalizeReasoningEffortForModel(name, &effort))
		}
		for _, suffix := range []string{"-pro", "-2026-11-17", "-latest", "-ultrafast"} {
			require.NotContains(t, ModelRatios, name+suffix)
		}
		require.True(t, ImplicitPromptCacheWrites(channeltype.OpenAI, name))
	}
	// Earlier configured models retain their own rates and reasoning contracts.
	require.Contains(t, ModelRatios["gpt-6-sol"].SupportedReasoningEfforts, "none")
	require.Contains(t, ModelRatios, "gpt-6.1-sol")
}

// TestGPT62ResponsesConversion exercises tool routing and Standard-tier pinning
// from Chat and Claude ingress. Parameter t asserts; nothing is returned.
func TestGPT62ResponsesConversion(t *testing.T) {
	for _, name := range []string{"gpt-6.2-astra", "gpt-6.2-sol"} {
		for _, mode := range []int{relaymode.ChatCompletions, relaymode.ClaudeMessages} {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			info := &meta.Meta{ChannelType: channeltype.OpenAI, ActualModelName: name,
				OriginModelName: "customer-alias", Mode: mode, BaseURL: "https://api.openai.com"}
			meta.Set2Context(c, info)
			temp, topP, effort := .3, .9, "xhigh"
			request := &model.GeneralOpenAIRequest{
				Model: name, MaxTokens: 1024, ReasoningEffort: &effort, Temperature: &temp, TopP: &topP,
				Messages: []model.Message{
					{Role: "system", Content: "Review only the supplied patch."},
					{Role: "user", Content: "Explain this change."},
				},
				Tools: []model.Tool{{Type: "web_search"}},
			}
			provider := &Adaptor{ChannelType: channeltype.OpenAI}
			converted, err := provider.ConvertRequest(c, mode, request)
			require.NoError(t, err)
			wire, err := json.Marshal(converted)
			require.NoError(t, err)
			var payload map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(wire, &payload))
			require.JSONEq(t, `"`+name+`"`, string(payload["model"]))
			require.JSONEq(t, `"default"`, string(payload["service_tier"]))
			require.JSONEq(t, `"Review only the supplied patch."`, string(payload["instructions"]))
			require.Contains(t, string(payload["reasoning"]), `"effort":"xhigh"`)
			require.Contains(t, string(payload["tools"]), "web_search")
			require.NotContains(t, payload, "temperature")
			require.NotContains(t, payload, "top_p")
			url, err := provider.GetRequestURL(info)
			require.NoError(t, err)
			require.Equal(t, "https://api.openai.com/v1/responses", url)
		}
	}
}

// TestGPT62NativeResponsesSampling verifies catalog-driven serialization removes
// unsupported log probabilities while preserving useful includes. Parameter t asserts.
func TestGPT62NativeResponsesSampling(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"gpt-6.2-astra", "gpt-6.2-sol"} {
		var request ResponseAPIRequest
		require.NoError(t, json.Unmarshal([]byte(`{
			"model":"`+name+`", "input":"hello", "reasoning":{"effort":"max"},
			"temperature":0, "top_p":0.9,
			"include":["message.output_text.logprobs","reasoning.encrypted_content"]
		}`), &request))
		wire, err := json.Marshal(request)
		require.NoError(t, err)
		var payload map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(wire, &payload))
		require.NotContains(t, payload, "temperature")
		require.NotContains(t, payload, "top_p")
		require.JSONEq(t, `{"effort":"max"}`, string(payload["reasoning"]))
		require.JSONEq(t, `["reasoning.encrypted_content"]`, string(payload["include"]))
		require.NotNil(t, request.Temperature, "serialization must preserve the caller's value")
		require.Len(t, request.Include, 2)
	}
}
