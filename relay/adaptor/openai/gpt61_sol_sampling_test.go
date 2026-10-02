package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestGPT61SolWireSampling verifies catalog-driven wire cleanup removes unsupported
// sampling fields without losing tools, continuation fields, or unknown JSON values.
func TestGPT61SolWireSampling(t *testing.T) {
	t.Parallel()

	for _, surface := range []string{"chat", "responses"} {
		for _, effort := range []string{"", "low", "medium", "high", "xhigh", "max"} {
			t.Run(surface+"/"+effort, func(t *testing.T) {
				var root map[string]json.RawMessage
				require.NoError(t, json.Unmarshal([]byte(`{
					"model":"gpt-6.1-sol", "temperature":0, "top_p":0.9,
					"logprobs":true, "top_logprobs":5,
					"include":["message.output_text.logprobs","reasoning.encrypted_content"],
					"tools":[{"type":"web_search"}],
					"opaque_extension":{"counter":9007199254740993}
				}`), &root))
				if surface == "chat" {
					root["messages"] = json.RawMessage(`[{"role":"user","content":"hello"}]`)
					if effort != "" {
						value, err := json.Marshal(effort)
						require.NoError(t, err)
						root["reasoning_effort"] = value
					}
				} else {
					root["input"] = json.RawMessage(`"hello"`)
					if effort != "" {
						value, err := json.Marshal(map[string]string{"effort": effort})
						require.NoError(t, err)
						root["reasoning"] = value
					}
				}
				tools, extension := string(root["tools"]), string(root["opaque_extension"])
				removed := NormalizeModelRequestParameters(root)
				for _, key := range []string{"temperature", "top_p", "logprobs", "top_logprobs"} {
					require.Contains(t, removed, key)
					require.NotContains(t, root, key)
				}
				require.JSONEq(t, `["reasoning.encrypted_content"]`, string(root["include"]))
				require.Equal(t, tools, string(root["tools"]))
				require.Equal(t, extension, string(root["opaque_extension"]))
			})
		}
	}
}

// TestGPT61SolResponsesSerialization verifies native and converted Responses bodies
// preserve reasoning and function calls while removing unsupported sampling fields.
func TestGPT61SolResponsesSerialization(t *testing.T) {
	t.Parallel()

	var request ResponseAPIRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"model":"gpt-6.1-sol", "input":"hello", "reasoning":{"effort":"max"},
		"temperature":0, "top_p":0.9,
		"include":["message.output_text.logprobs","reasoning.encrypted_content"]
	}`), &request))
	wire, err := json.Marshal(request)
	require.NoError(t, err)
	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(wire, &root))
	require.NotContains(t, root, "temperature")
	require.NotContains(t, root, "top_p")
	require.JSONEq(t, `{"effort":"max"}`, string(root["reasoning"]))
	require.JSONEq(t, `["reasoning.encrypted_content"]`, string(root["include"]))
	require.NotNil(t, request.Temperature)
	require.Equal(t, 0.0, *request.Temperature)
	require.Equal(t, []string{"message.output_text.logprobs", "reasoning.encrypted_content"}, request.Include)

	var chat model.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"model":"gpt-6.1-sol", "messages":[{"role":"user","content":"hello"}],
		"reasoning_effort":"xhigh", "temperature":0, "top_p":0.9,
		"tools":[{"type":"function","function":{"name":"lookup","parameters":{
			"type":"object","properties":{},"additionalProperties":false
		}}}]
	}`), &chat))
	wire, err = json.Marshal(ConvertChatCompletionToResponseAPI(&chat))
	require.NoError(t, err)
	root = nil
	require.NoError(t, json.Unmarshal(wire, &root))
	require.NotContains(t, root, "temperature")
	require.NotContains(t, root, "top_p")
	var reasoning map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(root["reasoning"], &reasoning))
	require.JSONEq(t, `"xhigh"`, string(reasoning["effort"]))
	var tools []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(root["tools"], &tools))
	require.Len(t, tools, 1)
	require.JSONEq(t, `"function"`, string(tools[0]["type"]))
	require.JSONEq(t, `"lookup"`, string(tools[0]["name"]))
}
