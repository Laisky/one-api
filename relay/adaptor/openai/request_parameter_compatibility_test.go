package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	relaymeta "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestModelSamplingCatalogPolicy verifies actual catalog entries rather than
// duplicating model capabilities in the wire implementation.
func TestModelSamplingCatalogPolicy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, effort string
		sampling     bool
	}{
		{"gpt-6-luna", "", false},
		{"gpt-6-sol", "none", true},
		{"gpt-6-astra", "none", false},
		{"gpt-6-luna", "none", true},
		{"gpt-6-luna", "high", false},
		{"gpt-5.2", "none", true},
		{"o3", "high", false},
		{"gpt-6-luna-2026-09-24", "high", false},
		{"gpt-6-luna-2026-99-99", "high", true},
		{"gpt-6-luna-custom", "high", true},
		{"gpt-4o", "", true},
		{"openai/gpt-oss-120b", "high", true},
		{"Qwen/Qwen3.5-35B-A3B", "high", true},
	} {
		t.Run(tc.name+"/"+tc.effort, func(t *testing.T) {
			var effort *string
			if tc.effort != "" {
				effort = &tc.effort
			}
			require.Equal(t, tc.sampling, modelSupportsSampling(tc.name, effort))
		})
	}
	root := map[string]json.RawMessage{"model": json.RawMessage(`42`), "temperature": json.RawMessage(`0`)}
	require.Empty(t, NormalizeModelRequestParameters(root))
	require.Contains(t, root, "temperature")
}

// TestResponseSamplingSerializationIsImmutable covers the typed fallback and
// Chat-to-Responses serialization path, including shared include backing arrays.
func TestResponseSamplingSerializationIsImmutable(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		effort   string
		sampling bool
	}{
		{"", false}, {"high", false}, {"none", true},
	} {
		t.Run(tc.effort, func(t *testing.T) {
			raw := []byte(`{"model":"gpt-6-luna","input":"hello","temperature":0,"top_p":0.9,"include":["message.output_text.logprobs","reasoning.encrypted_content"]}`)
			var request ResponseAPIRequest
			require.NoError(t, json.Unmarshal(raw, &request))
			if tc.effort != "" {
				require.NoError(t, json.Unmarshal([]byte(`{"reasoning":{"effort":"`+tc.effort+`"}}`), &request))
			}
			include := request.Include
			wire, err := json.Marshal(request)
			require.NoError(t, err)
			var root map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(wire, &root))
			_, present := root["temperature"]
			require.Equal(t, tc.sampling, present)
			_, present = root["top_p"]
			require.Equal(t, tc.sampling, present)
			if tc.sampling {
				require.Equal(t, `0`, string(root["temperature"]))
				require.JSONEq(t, `["message.output_text.logprobs","reasoning.encrypted_content"]`, string(root["include"]))
			} else {
				require.JSONEq(t, `["reasoning.encrypted_content"]`, string(root["include"]))
			}
			require.NotNil(t, request.Temperature)
			require.Equal(t, 0.0, *request.Temperature)
			require.NotNil(t, request.TopP)
			require.Equal(t, []string{"message.output_text.logprobs", "reasoning.encrypted_content"}, include)
			require.Equal(t, include, request.Include)
		})
	}
}

// TestMappedChatSamplingPreservesNone verifies the actual upstream model, not
// the tenant alias, controls normalization before conversion to Responses.
func TestMappedChatSamplingPreservesNone(t *testing.T) {
	for _, name := range []string{"gpt-6-luna", "gpt-6-sol", "gpt-5.2"} {
		t.Run(name, func(t *testing.T) {
			temperature, topP, effort := 0.0, 0.9, "none"
			request := &model.GeneralOpenAIRequest{
				Model: "tenant-alias", Temperature: &temperature, TopP: &topP,
				ReasoningEffort: &effort,
				Messages: []model.Message{{Role: "user", Content: "hello"}},
			}
			info := &relaymeta.Meta{
				ChannelType: channeltype.OpenAI, Mode: relaymode.ChatCompletions,
				ActualModelName: name,
			}
			a := &Adaptor{ChannelType: channeltype.OpenAI}
			require.NoError(t, a.applyRequestTransformations(info, request))
			require.NotNil(t, request.Temperature)
			require.Equal(t, 0.0, *request.Temperature)
			require.NotNil(t, request.TopP)
			require.Equal(t, 0.9, *request.TopP)
			require.Equal(t, "none", *request.ReasoningEffort)
		})
	}
}
