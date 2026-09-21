package controller

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/xai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestSystematicNativeXAIReasoningWire verifies the native Responses boundary,
// including aliases and non-tunable models. Parameters: t runs the catalog
// matrix. Returns: none; assertions inspect the final serialized payload.
func TestSystematicNativeXAIReasoningWire(t *testing.T) {
	for name, cfg := range xai.ModelRatios {
		for _, effort := range []string{"none", "low", "medium", "high", "xhigh", "ultra"} {
			t.Run(name+"/"+effort, func(t *testing.T) {
				raw := []byte(fmt.Sprintf(`{"model":%q,"input":"test","reasoning":{"effort":%q,"vendor":{"counter":9007199254740993}}}`, name, effort))
				var req openai.ResponseAPIRequest
				require.NoError(t, json.Unmarshal(raw, &req))
				body, _, _, err := normalizeResponseAPIRawBody(raw, &req, channeltype.XAI)
				require.NoError(t, err)
				var root map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(body, &root))
				var reasoning map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(root["reasoning"], &reasoning))
				if slices.Contains(cfg.SupportedReasoningEfforts, effort) {
					require.Equal(t, fmt.Sprintf("%q", effort), string(reasoning["effort"]))
				} else {
					require.NotContains(t, reasoning, "effort")
				}
				require.Contains(t, string(reasoning["vendor"]), "9007199254740993")
				again, _, changed, err := normalizeResponseAPIRawBody(body, &req, channeltype.XAI)
				require.NoError(t, err)
				require.False(t, changed)
				require.Equal(t, body, again)
			})
		}
	}
}

// TestSystematicNativeReasoningProviderIsolation verifies that xAI rules never
// leak onto custom providers or unknown model IDs. Parameters: t runs cases.
// Returns: none; explicit opaque provider vocabulary remains untouched.
func TestSystematicNativeReasoningProviderIsolation(t *testing.T) {
	for _, tc := range []struct { model string; channel int }{
		{"grok-4.7", channeltype.OpenAICompatible},
		{"grok-future", channeltype.XAI},
	} {
		raw := []byte(fmt.Sprintf(`{"model":%q,"input":"test","reasoning":{"effort":"future-effort"}}`, tc.model))
		var req openai.ResponseAPIRequest
		require.NoError(t, json.Unmarshal(raw, &req))
		body, _, changed, err := normalizeResponseAPIRawBody(raw, &req, tc.channel)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, raw, body)
	}
}
