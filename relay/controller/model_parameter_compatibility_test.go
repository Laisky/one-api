package controller

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestNativeResponsesReasoningParameterCompatibility reproduces the Luna 400
// using the real native Responses body builder, including model mapping.
func TestNativeResponsesReasoningParameterCompatibility(t *testing.T) {
	t.Parallel()
	for _, channel := range []int{channeltype.OpenAI, channeltype.Azure, channeltype.OpenAICompatible} {
		for _, modelName := range []string{"gpt-6-luna", "gpt-6-sol", "gpt-6-astra", "o3"} {
			for _, effort := range []string{"", "high"} {
				t.Run(modelName+"/"+effort+"/"+strconv.Itoa(channel), func(t *testing.T) {
					// Given SDK sampling controls and opaque provider data, routed
					// from a public alias to the actual upstream reasoning model.
					raw := []byte(`{"model":"public-alias","input":"hello","temperature":0,"top_p":0.8,"top_logprobs":5,"logprobs":false,"include":["reasoning.encrypted_content","message.output_text.logprobs"],"max_output_tokens":100,"previous_response_id":"resp_123","vendor_extension":{"id":9007199254740993}}`)
					var request openai.ResponseAPIRequest
					require.NoError(t, json.Unmarshal(raw, &request))
					request.Model = modelName
					if effort != "" {
						require.NoError(t, json.Unmarshal([]byte(`{"reasoning":{"effort":"`+effort+`"}}`), &request))
					}

					// When the native Responses wire payload is built.
					wire, _, changed, err := normalizeResponseAPIRawBody(raw, &request, channel)
					require.NoError(t, err)
					require.True(t, changed)
					var root map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(wire, &root))

					// Then unsupported controls are absent; state, output limits,
					// unrelated include entries and numeric precision survive.
					for _, key := range []string{"temperature", "top_p", "top_logprobs", "logprobs"} {
						require.NotContains(t, root, key)
					}
					require.JSONEq(t, `["reasoning.encrypted_content"]`, string(root["include"]))
					require.Equal(t, `100`, string(root["max_output_tokens"]))
					require.Equal(t, `"resp_123"`, string(root["previous_response_id"]))
					require.Equal(t, `{"id":9007199254740993}`, string(root["vendor_extension"]))
					again, _, changedAgain, err := normalizeResponseAPIRawBody(wire, &request, channel)
					require.NoError(t, err)
					require.False(t, changedAgain)
					require.Equal(t, string(wire), string(again))
				})
			}
		}
	}
}

// TestNativeResponsesPreservesSupportedSampling prevents a blanket blacklist
// from breaking non-reasoning modes and unrelated providers.
func TestNativeResponsesPreservesSupportedSampling(t *testing.T) {
	t.Parallel()
	for _, modelName := range []string{"gpt-6-luna", "gpt-6-sol", "gpt-5.2", "gpt-4o", "Qwen/Qwen3.5-35B-A3B"} {
		t.Run(modelName, func(t *testing.T) {
			raw := []byte(`{"model":"` + modelName + `","input":"hello","reasoning":{"effort":"none"},"temperature":0,"top_p":0.8,"top_logprobs":0,"include":["message.output_text.logprobs"]}`)
			var request openai.ResponseAPIRequest
			require.NoError(t, json.Unmarshal(raw, &request))
			wire, _, _, err := normalizeResponseAPIRawBody(raw, &request, channeltype.OpenAICompatible)
			require.NoError(t, err)
			var root map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(wire, &root))
			require.Equal(t, `0`, string(root["temperature"]))
			require.Equal(t, `0.8`, string(root["top_p"]))
			require.Equal(t, `0`, string(root["top_logprobs"]))
			require.JSONEq(t, `["message.output_text.logprobs"]`, string(root["include"]))
		})
	}
}

// TestChatPassthroughCannotResurrectUnsupportedParameters checks policy after
// conversion and controlled passthrough, not just the typed request fields.
func TestChatPassthroughCannotResurrectUnsupportedParameters(t *testing.T) {
	t.Parallel()
	original := []byte(`{"model":"alias","messages":[{"role":"user","content":"hello"}],"temperature":0.2,"top_p":0.9,"logprobs":true,"top_logprobs":3,"opaque":{"id":9007199254740993}}`)
	updated := []byte(`{"model":"gpt-6-luna","messages":[{"role":"user","content":"hello"}],"reasoning_effort":"high","temperature":1,"top_p":0.9,"logprobs":true,"top_logprobs":3}`)
	wire, stats, _, err := mergeControlledPassthroughJSON(original, updated, true)
	require.NoError(t, err)
	require.Equal(t, 4, stats.UnsupportedParametersRemoved)
	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(wire, &root))
	for _, key := range []string{"temperature", "top_p", "logprobs", "top_logprobs"} {
		require.NotContains(t, root, key)
	}
	require.Equal(t, `{"id":9007199254740993}`, string(root["opaque"]))
}
