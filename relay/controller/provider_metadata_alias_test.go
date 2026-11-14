package controller

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/deepseek"
	"github.com/Laisky/one-api/relay/adaptor/mistral"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestChatProviderMetadataCarrierAliases verifies every casefold carrier is scoped at the actual request-body boundary without touching caller data.
func TestChatProviderMetadataCarrierAliases(t *testing.T) {
	none := metalib.Meta{APIType: apitype.OpenAI, ChannelType: channeltype.OpenAICompatible, BaseURL: "https://llm.example.com"}
	google := metalib.Meta{APIType: apitype.OpenAI, ChannelType: channeltype.OpenAICompatible, BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai"}
	for _, route := range []struct {
		name, model string
		meta        metalib.Meta
		provider    adaptor.Adaptor
		google, raw bool
	}{
		{"none_typed", "deepseek-chat", metalib.Meta{APIType: apitype.DeepSeek, ChannelType: channeltype.DeepSeek}, &deepseek.Adaptor{}, false, false},
		{"none_map", "mistral-large-latest", metalib.Meta{APIType: apitype.Mistral, ChannelType: channeltype.Mistral}, &mistral.Adaptor{}, false, false},
		{"none_normalized", "chat-model", none, &openai.Adaptor{ChannelType: channeltype.OpenAICompatible}, false, false},
		{"none_raw_control", "chat-model", none, &openai.Adaptor{ChannelType: channeltype.OpenAICompatible}, false, true},
		{"google_typed", "zai-org/glm-5.2-maas", metalib.Meta{APIType: apitype.VertexAI, ChannelType: channeltype.VertextAI}, &vertexai.Adaptor{}, true, false},
		{"google_map", "mistral-large-latest", metalib.Meta{APIType: apitype.Mistral, ChannelType: channeltype.Mistral, BaseURL: google.BaseURL}, &mistral.Adaptor{}, true, false},
		{"google_normalized", "chat-model", google, &openai.Adaptor{ChannelType: channeltype.OpenAICompatible}, true, false},
		{"google_raw_control", "chat-model", google, &openai.Adaptor{ChannelType: channeltype.OpenAICompatible}, true, true},
	} {
		for _, shape := range []struct {
			name string
			keys []string
		}{
			{"canonical_and_alias", []string{"extra_content", "Extra_Content"}},
			{"alias_only", []string{"Extra_Content"}},
			{"multiple_aliases", []string{"extra_content", "Extra_Content", "EXTRA_CONTENT", "eXtRa_CoNtEnT"}},
		} {
			for _, mode := range []string{"google_only", "oneapi_only", "mixed"} {
				t.Run(route.name+"/"+shape.name+"/"+mode, func(t *testing.T) {
					previous := config.EnforceIncludeUsage
					config.EnforceIncludeUsage = !route.raw
					t.Cleanup(func() { config.EnforceIncludeUsage = previous })
					part := map[string]any{"type": "text", "text": "hi"}
					for i, key := range shape.keys {
						value := map[string]any{}
						if mode == "google_only" || mode == "mixed" && i == 0 {
							value["google"] = map[string]any{"thought_signature": "alias-sig"}
						}
						if mode == "oneapi_only" || mode == "mixed" {
							value["oneapi"] = map[string]any{"gemini_part_index": 0}
						}
						part[key] = value
					}
					const arguments = `{"extra_content":"literal","Extra_Content":{"oneapi":"caller-data"},"n":9007199254740993}`
					schema := map[string]any{"type": "object", "properties": map[string]any{
						"extra_content": map[string]any{"type": "string"},
						"Extra_Content": map[string]any{"type": "object", "default": map[string]any{"oneapi": "literal"}},
					}}
					request := map[string]any{"model": route.model, "messages": []any{
						map[string]any{"role": "user", "content": []any{part}},
						map[string]any{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"id": "call_x", "type": "function", "function": map[string]any{"name": "lookup", "arguments": arguments}}}},
						map[string]any{"role": "tool", "tool_call_id": "call_x", "content": "ok"},
					}, "tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "lookup", "parameters": schema}}}}
					raw, err := json.Marshal(request)
					require.NoError(t, err)
					m := route.meta
					m.ActualModelName, m.OriginModelName, m.Mode = route.model, route.model, relaymode.ChatCompletions
					body := outboundChatBody(t, string(raw), &m, route.provider)
					var sent map[string]any
					require.NoError(t, json.Unmarshal(body, &sent))
					messages := sent["messages"].([]any)
					parts := messages[0].(map[string]any)["content"].([]any)
					outbound := parts[0].(map[string]any)
					carriers := 0
					for key, value := range outbound {
						if !strings.EqualFold(key, "extra_content") {
							continue
						}
						carriers++
						require.True(t, route.google, "None must remove every casefold carrier key: %s", key)
						require.Equal(t, "extra_content", key, "GoogleWire may emit only the canonical carrier spelling")
						require.Equal(t, map[string]any{"google": map[string]any{"thought_signature": "alias-sig"}}, value)
					}
					if route.google && mode != "oneapi_only" && shape.name != "alias_only" {
						require.Equal(t, 1, carriers)
					} else {
						require.Zero(t, carriers)
					}
					tools := sent["tools"].([]any)
					gotSchema := tools[0].(map[string]any)["function"].(map[string]any)["parameters"]
					require.Equal(t, schema, gotSchema, "schema property names and defaults are literal caller data")
					calls := messages[1].(map[string]any)["tool_calls"].([]any)
					require.Equal(t, arguments, calls[0].(map[string]any)["function"].(map[string]any)["arguments"], "argument JSON is literal caller data")
				})
			}
		}
	}
}
