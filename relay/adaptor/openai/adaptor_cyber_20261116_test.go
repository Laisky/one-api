package openai

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGPT6CyberResponsesConversion exercises the provider request produced from
// Chat and Claude input. Parameter t reports assertions; nothing is returned.
func TestGPT6CyberResponsesConversion(t *testing.T) {
	for _, mode := range []int{relaymode.ChatCompletions, relaymode.ClaudeMessages} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		info := &meta.Meta{ChannelType: channeltype.OpenAI, ActualModelName: "gpt-6-cyber",
			OriginModelName: "security-alias", Mode: mode, BaseURL: "https://api.openai.com"}
		meta.Set2Context(c, info)
		request := &model.GeneralOpenAIRequest{
			Model: "gpt-6-cyber", MaxTokens: 512,
			Messages: []model.Message{
				{Role: "system", Content: "Review only the supplied patch."},
				{Role: "user", Content: "Explain this authorization check."},
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
		require.Equal(t, `"gpt-6-cyber"`, string(payload["model"]))
		require.Equal(t, `"default"`, string(payload["service_tier"]))
		require.Equal(t, `"Review only the supplied patch."`, string(payload["instructions"]))
		require.Contains(t, string(payload["tools"]), "web_search")
		url, err := provider.GetRequestURL(info)
		require.NoError(t, err)
		require.Equal(t, "https://api.openai.com/v1/responses", url)
	}
}

// TestGPT6CyberStandardTier rejects requests that would exceed the implemented
// Standard tariff. Parameter t reports assertions; nothing is returned.
func TestGPT6CyberStandardTier(t *testing.T) {
	provider := &Adaptor{ChannelType: channeltype.OpenAI}
	for _, tier := range []string{"auto", "fast", "priority", "flex", "ultrafast"} {
		request := &model.GeneralOpenAIRequest{Model: "customer-alias", ServiceTier: &tier}
		info := &meta.Meta{ChannelType: channeltype.OpenAI, ActualModelName: "gpt-6-cyber"}
		require.ErrorContains(t, provider.applyRequestTransformations(info, request), "Standard")
	}
	// Unverified compatible deployments retain their operator-controlled tiers.
	tier := "fast"
	request := &model.GeneralOpenAIRequest{Model: "gpt-6-cyber", ServiceTier: &tier}
	info := &meta.Meta{ChannelType: channeltype.OpenAICompatible, ActualModelName: "gpt-6-cyber"}
	require.NoError(t, provider.applyRequestTransformations(info, request))
	require.Equal(t, "fast", *request.ServiceTier)
}
