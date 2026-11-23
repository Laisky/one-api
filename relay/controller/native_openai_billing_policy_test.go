package controller

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestNativeOpenAIStandardTierPolicyMatrix checks raw Responses tier admission
// across policy and ordinary models, model-ID spellings, channel types and raw
// service_tier encodings. Parameter t reports failures; nothing returns.
func TestNativeOpenAIStandardTierPolicyMatrix(t *testing.T) {
	t.Parallel()
	models := []struct {
		name     string
		declared bool
	}{
		{"gpt-6.1-terra", true},
		{" GPT-6.1-TERRA ", true},
		{"gpt-6-cyber", true},
		{"GPT-6-Cyber\t", true},
		{"gpt-6.2-astra", true},
		{" GPT-6.2-ASTRA ", true},
		{"gpt-6.2-sol", true},
		{"GPT-6.2-Sol\t", true},
		{"gpt-6-sol", false},
		{"gpt-6-luna", false},
		{"gpt-5.6-cyber", false},
	}
	channels := []struct {
		name        string
		channelType int
		native      bool
	}{
		{"openai", channeltype.OpenAI, true},
		{"unset", 0, false},
		{"azure", channeltype.Azure, false},
		{"openai_compatible", channeltype.OpenAICompatible, false},
		{"groq", channeltype.Groq, false},
	}
	tiers := []struct {
		name, raw string // raw is the encoded service_tier; empty omits it
		changed   bool
		errorText string
	}{
		{"omitted", "", true, ""},
		{"standard", `"default"`, false, ""},
		{"padded_upper_standard", `" DEFAULT "`, true, ""},
		{"null", `null`, true, ""},
		{"fast", `"fast"`, false, "Standard pricing does not support service_tier"},
		{"empty", `""`, false, "Standard pricing does not support service_tier"},
		{"non_string", `123`, false, "decode OpenAI Standard service_tier"},
	}
	for _, modelCase := range models {
		for _, channelCase := range channels {
			for _, tierCase := range tiers {
				name := channelCase.name + "/" + modelCase.name + "/" + tierCase.name
				root := map[string]json.RawMessage{"input": json.RawMessage(`"hello"`)}
				if tierCase.raw != "" {
					root["service_tier"] = json.RawMessage(tierCase.raw)
				}
				changed, err := normalizeNativeOpenAIStandardTier(root, modelCase.name, channelCase.channelType)
				if !modelCase.declared || !channelCase.native {
					require.NoError(t, err, name)
					require.False(t, changed, name)
					if tierCase.raw == "" {
						require.NotContains(t, root, "service_tier", name)
					} else {
						require.Equal(t, tierCase.raw, string(root["service_tier"]), name)
					}
					continue
				}
				if tierCase.errorText != "" {
					require.ErrorContains(t, err, tierCase.errorText, name)
					if tierCase.name != "non_string" {
						require.ErrorContains(t, err, strings.ToLower(strings.TrimSpace(modelCase.name)), name)
					}
					continue
				}
				require.NoError(t, err, name)
				require.Equal(t, tierCase.changed, changed, name)
				require.Equal(t, `"default"`, string(root["service_tier"]), name)
			}
		}
	}
}

// TestNativeOpenAIWebSocketPolicySpellings refuses HTTP-only policy models on
// the WebSocket transport regardless of model-ID case and surrounding
// whitespace. Parameter t reports failures; nothing returns.
func TestNativeOpenAIWebSocketPolicySpellings(t *testing.T) {
	for _, modelName := range []string{
		" GPT-6.1-TERRA ", "Gpt-6.1-Terra", "GPT-6-CYBER", "\tgpt-6-cyber\n",
		" GPT-6.2-ASTRA ", "Gpt-6.2-Astra", "GPT-6.2-SOL", "\tgpt-6.2-sol\n",
	} {
		t.Run(strings.TrimSpace(modelName), func(t *testing.T) {
			c, _ := newWSUpgradeContext(t, "model=policy")
			info := &metalib.Meta{ChannelType: channeltype.OpenAI, ActualModelName: modelName,
				BaseURL: "https://api.openai.com", Mode: relaymode.ResponseAPI}
			handled, bizErr := maybeHandleResponseAPIWebSocket(c, info)
			require.True(t, handled)
			require.NotNil(t, bizErr)
			require.Equal(t, http.StatusBadRequest, bizErr.StatusCode)
			require.Equal(t, "response_websocket_model_billing_not_supported", bizErr.Error.Code)
			require.ErrorContains(t, bizErr.RawError, "WebSocket billing is not qualified")
		})
	}
}
