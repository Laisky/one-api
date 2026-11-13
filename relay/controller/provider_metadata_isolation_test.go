package controller

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/deepseek"
	"github.com/Laisky/one-api/relay/adaptor/mistral"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/adaptor/vertexai/maas"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// googleSignedChatHistory replays a Chat history exactly as the gateway emits it for Gemini turns:
// content-part, message and tool-call carriers with Google and gateway namespaces. User data that
// happens to use the name extra_content (arguments, schema properties) must never be touched.
const googleSignedChatHistory = `{"model":"%MODEL%","messages":[` +
	`{"role":"user","content":[{"type":"text","text":"hi","extra_content":{"google":{"thought_signature":"c2lnLXBhcnQ="}}}]},` +
	`{"role":"assistant","content":"","extra_content":{"google":{"thought_signature":"c2lnLW1zZw=="},"oneapi":{"gemini_text_present":false}},` +
	`"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"extra_content\":\"user-data\",\"n\":9007199254740993}"},` +
	`"extra_content":{"google":{"thought_signature":"c2lnLWNhbGw="},"oneapi":{"gemini_part_index":0}}}]},` +
	`{"role":"tool","tool_call_id":"call_1","content":"ok"}],` +
	`"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"extra_content":{"type":"string"}}}}}]}`

// chatCarrierWire decodes the outbound Chat payload into the positions that can carry provider metadata.
type chatCarrierWire struct {
	Messages []struct {
		ExtraContent json.RawMessage `json:"extra_content"`
		Content      json.RawMessage `json:"content"`
		ToolCalls    []struct {
			ExtraContent json.RawMessage `json:"extra_content"`
			Function     struct {
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	} `json:"messages"`
	Tools []struct {
		ExtraContent json.RawMessage `json:"extra_content"`
		Function     struct {
			Parameters json.RawMessage `json:"parameters"`
		} `json:"function"`
	} `json:"tools"`
}

// outboundChatBody runs the actual Chat request-body boundary for one destination.
func outboundChatBody(t *testing.T, raw string, m *metalib.Meta, provider adaptor.Adaptor) []byte {
	t.Helper()
	var request relaymodel.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal([]byte(raw), &request))
	// Holders of the decoded slices (billing snapshots, the native prepared cache) keep their view.
	shared := relaymodel.GeneralOpenAIRequest{Messages: request.Messages, Tools: request.Tools}
	before := chatCarrierSnapshot(t, &shared)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(raw))
	metalib.Set2Context(c, m)
	provider.Init(m)
	reader, err := getRequestBody(c, m, &request, provider, false)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, before, chatCarrierSnapshot(t, &shared), "scoping must not write through previously shared slices or carriers")
	return body
}

// chatCarrierSnapshot serializes every provider-metadata carrier reachable from request's slices.
func chatCarrierSnapshot(t *testing.T, request *relaymodel.GeneralOpenAIRequest) string {
	t.Helper()
	var carriers []any
	for _, message := range request.Messages {
		carriers = append(carriers, message.ExtraContent, message.Content)
		for _, call := range message.ToolCalls {
			carriers = append(carriers, call.ExtraContent)
		}
	}
	for _, tool := range request.Tools {
		carriers = append(carriers, tool.ExtraContent)
	}
	encoded, err := json.Marshal(carriers)
	require.NoError(t, err)
	return string(encoded)
}

// partCarriers returns the extra_content values of a message's content parts.
func partCarriers(t *testing.T, content json.RawMessage) []json.RawMessage {
	t.Helper()
	var parts []map[string]json.RawMessage
	if json.Unmarshal(content, &parts) != nil {
		return nil
	}
	var out []json.RawMessage
	for _, part := range parts {
		out = append(out, part["extra_content"])
	}
	return out
}

// TestChatProviderMetadataIsolation proves gateway-emitted Google carriers are scoped to the destination wire
// on every Chat request-body branch: raw passthrough, normalized passthrough, typed adaptor payloads,
// map payloads and the forced Responses conversion.
func TestChatProviderMetadataIsolation(t *testing.T) {
	compatible := metalib.Meta{APIType: apitype.OpenAI, ChannelType: channeltype.OpenAICompatible, BaseURL: "https://llm.example.com"}
	googleHost := metalib.Meta{APIType: apitype.OpenAI, ChannelType: channeltype.OpenAICompatible, BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai"}
	geminiCompat := metalib.Meta{APIType: apitype.OpenAI, ChannelType: channeltype.GeminiOpenAICompatible}
	for _, tc := range []struct {
		name, model string
		meta        metalib.Meta
		provider    adaptor.Adaptor
		rawBranch   bool
		google      bool
	}{
		{"openai_chat_only_typed", "gpt-4o-search-preview", metalib.Meta{APIType: apitype.OpenAI, ChannelType: channeltype.OpenAI}, &openai.Adaptor{ChannelType: channeltype.OpenAI}, false, false},
		{"openai_forced_responses", "gpt-5.5", metalib.Meta{APIType: apitype.OpenAI, ChannelType: channeltype.OpenAI}, &openai.Adaptor{ChannelType: channeltype.OpenAI}, false, false},
		{"compatible_raw_passthrough", "chat-model", compatible, &openai.Adaptor{ChannelType: channeltype.OpenAICompatible}, true, false},
		{"compatible_normalized_passthrough", "chat-model", compatible, &openai.Adaptor{ChannelType: channeltype.OpenAICompatible}, false, false},
		{"deepseek_typed", "deepseek-chat", metalib.Meta{APIType: apitype.DeepSeek, ChannelType: channeltype.DeepSeek}, &deepseek.Adaptor{}, false, false},
		{"mistral_map_payload", "mistral-large-latest", metalib.Meta{APIType: apitype.Mistral, ChannelType: channeltype.Mistral}, &mistral.Adaptor{}, false, false},
		{"gemini_openai_compatible_raw", "gemini-3.8-flash", geminiCompat, &openai.Adaptor{ChannelType: channeltype.GeminiOpenAICompatible}, true, true},
		{"gemini_openai_compatible_normalized", "gemini-3.8-flash", geminiCompat, &openai.Adaptor{ChannelType: channeltype.GeminiOpenAICompatible}, false, true},
		{"compatible_google_host_raw", "gemini-3.8-flash", googleHost, &openai.Adaptor{ChannelType: channeltype.OpenAICompatible}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := config.EnforceIncludeUsage
			config.EnforceIncludeUsage = !tc.rawBranch
			t.Cleanup(func() { config.EnforceIncludeUsage = old })
			m := tc.meta
			m.ActualModelName, m.OriginModelName, m.Mode = tc.model, tc.model, relaymode.ChatCompletions
			body := outboundChatBody(t, strings.ReplaceAll(googleSignedChatHistory, "%MODEL%", tc.model), &m, tc.provider)
			if tc.name == "openai_forced_responses" {
				// The Responses conversion builds items from known fields only; keep it that way.
				require.NotContains(t, string(body), "c2lnL", "signatures must not reach a Responses wire: %s", body)
				require.NotContains(t, string(body), "gemini_", "gateway metadata must not reach a Responses wire: %s", body)
				return
			}
			var wire chatCarrierWire
			require.NoError(t, json.Unmarshal(body, &wire), string(body))
			require.Len(t, wire.Messages, 3)
			carriers := []json.RawMessage{wire.Messages[1].ExtraContent, wire.Messages[1].ToolCalls[0].ExtraContent}
			carriers = append(carriers, partCarriers(t, wire.Messages[0].Content)...)
			require.Len(t, carriers, 3)
			for _, carrier := range carriers {
				if !tc.google {
					require.Empty(t, carrier, "a non-Google wire must not receive gateway provider metadata: %s", body)
					continue
				}
				require.NotContains(t, string(carrier), "oneapi", "gateway-only metadata is never a Google wire field: %s", body)
			}
			if tc.google {
				require.JSONEq(t, `{"google":{"thought_signature":"c2lnLWNhbGw="}}`, string(wire.Messages[1].ToolCalls[0].ExtraContent))
				require.JSONEq(t, `{"google":{"thought_signature":"c2lnLW1zZw=="}}`, string(wire.Messages[1].ExtraContent))
				require.JSONEq(t, `{"google":{"thought_signature":"c2lnLXBhcnQ="}}`, string(carriers[2]))
			}
			for _, tool := range wire.Tools {
				require.Empty(t, tool.ExtraContent)
			}
			require.Contains(t, wire.Messages[1].ToolCalls[0].Function.Arguments, `"extra_content":"user-data"`, "argument data is caller content")
			require.Contains(t, wire.Messages[1].ToolCalls[0].Function.Arguments, "9007199254740993")
			require.Contains(t, string(wire.Tools[0].Function.Parameters), `"extra_content"`, "schema property names are caller content")
		})
	}
}

// TestChatProviderMetadataRawPassthroughControl keeps carrier-free raw passthrough byte-identical.
func TestChatProviderMetadataRawPassthroughControl(t *testing.T) {
	old := config.EnforceIncludeUsage
	config.EnforceIncludeUsage = false
	t.Cleanup(func() { config.EnforceIncludeUsage = old })
	raw := `{"model":"chat-model","messages":[{"role":"user","content":"hi","vendor_field":{"n":9007199254740993}}],"vendor_root":true}`
	m := metalib.Meta{APIType: apitype.OpenAI, ChannelType: channeltype.OpenAICompatible, BaseURL: "https://llm.example.com", ActualModelName: "chat-model", OriginModelName: "chat-model", Mode: relaymode.ChatCompletions}
	body := outboundChatBody(t, raw, &m, &openai.Adaptor{ChannelType: channeltype.OpenAICompatible})
	require.Equal(t, raw, string(body))
}

// TestClaudePassthroughProviderMetadataIsolation proves Anthropic-wire passthrough never forwards carriers.
func TestClaudePassthroughProviderMetadataIsolation(t *testing.T) {
	raw := []byte(`{"model":"claude-sonnet-5-5","max_tokens":64,"messages":[` +
		`{"role":"assistant","content":[{"type":"thinking","thinking":"plan","signature":"sig-native"},` +
		`{"type":"text","text":"","extra_content":{"google":{"thought_signature":"c2lnLXRleHQ="},"oneapi":{"gemini_text_present":true}}},` +
		`{"type":"tool_use","id":"toolu_1","name":"lookup","input":{"extra_content":"user-data","n":9007199254740993},"extra_content":{"google":{"thought_signature":"c2lnLWNhbGw="}}}]},` +
		`{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"}]}]}`)
	var request ClaudeMessagesRequest
	require.NoError(t, json.Unmarshal(raw, &request))
	got, _, err := rewriteAndSanitizeClaudeRequestBody(raw, &request)
	require.NoError(t, err)
	var wire struct {
		Messages []struct {
			Content []map[string]json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(got, &wire))
	for _, block := range wire.Messages[0].Content {
		require.NotContains(t, block, "extra_content", "Anthropic rejects unknown content-block fields: %s", got)
	}
	require.Len(t, wire.Messages[0].Content, 2, "a metadata-only empty text block goes with its carrier; Anthropic rejects empty text blocks: %s", got)
	require.JSONEq(t, `"sig-native"`, string(wire.Messages[0].Content[0]["signature"]))
	require.JSONEq(t, `"tool_use"`, string(wire.Messages[0].Content[1]["type"]))
	require.Equal(t, `{"extra_content":"user-data","n":9007199254740993}`, string(wire.Messages[0].Content[1]["input"]))
}

// TestResponsesNativeProviderMetadataIsolation proves native Responses input never forwards carriers.
func TestResponsesNativeProviderMetadataIsolation(t *testing.T) {
	raw := []byte(`{"model":"gpt-5.5","input":[` +
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]},` +
		`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"","extra_content":{"google":{"thought_signature":"c2lnLXRleHQ="},"oneapi":{"gemini_text_present":true}}}]},` +
		`{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"extra_content\":\"user-data\"}","extra_content":{"google":{"thought_signature":"c2lnLWNhbGw="}}},` +
		`{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`)
	var request openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal(raw, &request))
	patched, _, _, err := normalizeResponseAPIRawBody(raw, &request, channeltype.OpenAI)
	require.NoError(t, err)
	var wire struct {
		Input []map[string]json.RawMessage `json:"input"`
	}
	require.NoError(t, json.Unmarshal(patched, &wire))
	require.Len(t, wire.Input, 4)
	for _, item := range wire.Input {
		require.NotContains(t, item, "extra_content", "OpenAI Responses rejects unknown input item fields: %s", patched)
		var parts []map[string]json.RawMessage
		if content, ok := item["content"]; ok && json.Unmarshal(content, &parts) == nil {
			for _, part := range parts {
				require.NotContains(t, part, "extra_content")
			}
		}
	}
	require.JSONEq(t, `"{\"extra_content\":\"user-data\"}"`, string(wire.Input[2]["arguments"]))
}

// TestChatProviderMetadataScopePolicy pins the destination policy to channel type, dispatch route and base URL.
func TestChatProviderMetadataScopePolicy(t *testing.T) {
	require.True(t, vertexai.UsesNativeGemini("gemma-3-27b-it"), "fixture: Gemma dispatches to native Vertex GenerateContent")
	require.NotEmpty(t, maas.ModelList)
	for _, tc := range []struct {
		name string
		meta *metalib.Meta
		want relaymodel.ProviderMetadataScope
	}{
		{"nil_meta", nil, relaymodel.ProviderMetadataNone},
		{"gemini_api", &metalib.Meta{APIType: apitype.Gemini, ActualModelName: "anything"}, relaymodel.ProviderMetadataNative},
		{"vertex_native_gemini", &metalib.Meta{APIType: apitype.VertexAI, ActualModelName: "gemini-3.8-flash"}, relaymodel.ProviderMetadataNative},
		{"vertex_native_gemma", &metalib.Meta{APIType: apitype.VertexAI, ActualModelName: "gemma-3-27b-it"}, relaymodel.ProviderMetadataNative},
		{"vertex_managed_open_model", &metalib.Meta{APIType: apitype.VertexAI, ActualModelName: maas.ModelList[0]}, relaymodel.ProviderMetadataGoogleWire},
		{"gemini_openai_compatible", &metalib.Meta{APIType: apitype.OpenAI, ChannelType: channeltype.GeminiOpenAICompatible}, relaymodel.ProviderMetadataGoogleWire},
		{"google_host_regional_vertex", &metalib.Meta{APIType: apitype.OpenAI, ChannelType: channeltype.OpenAICompatible, BaseURL: "https://us-central1-aiplatform.googleapis.com/v1/projects/p/locations/us-central1/endpoints/openapi"}, relaymodel.ProviderMetadataGoogleWire},
		{"google_host_uppercase", &metalib.Meta{APIType: apitype.OpenAI, ChannelType: channeltype.OpenAICompatible, BaseURL: "https://GenerativeLanguage.googleapis.com./v1beta/openai"}, relaymodel.ProviderMetadataGoogleWire},
		{"lookalike_host", &metalib.Meta{APIType: apitype.OpenAI, ChannelType: channeltype.OpenAICompatible, BaseURL: "https://generativelanguage.googleapis.com.example.com/v1"}, relaymodel.ProviderMetadataNone},
		{"gemini_named_model_elsewhere", &metalib.Meta{APIType: apitype.OpenAI, ChannelType: channeltype.OpenRouter, ActualModelName: "google/gemini-3.8-flash"}, relaymodel.ProviderMetadataNone},
		{"anthropic", &metalib.Meta{APIType: apitype.Anthropic, ChannelType: channeltype.Anthropic}, relaymodel.ProviderMetadataNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, chatProviderMetadataScope(tc.meta))
		})
	}
}

// TestChatProviderMetadataScopeKeepsBillingNormalization proves scoping keeps the caller's request object:
// adaptors normalize it in place (here the default output limit) and quota estimation reads it back.
func TestChatProviderMetadataScopeKeepsBillingNormalization(t *testing.T) {
	m := metalib.Meta{APIType: apitype.OpenAI, ChannelType: channeltype.OpenAICompatible, BaseURL: "https://llm.example.com", ActualModelName: "chat-model", OriginModelName: "chat-model", Mode: relaymode.ChatCompletions}
	limits := map[bool]*int{}
	for _, signed := range []bool{false, true} {
		raw := `{"model":"chat-model","messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"ok"},{"role":"user","content":"again"}]}`
		if signed {
			raw = `{"model":"chat-model","messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"ok","extra_content":{"google":{"thought_signature":"c2ln"}}},{"role":"user","content":"again"}]}`
		}
		var request relaymodel.GeneralOpenAIRequest
		require.NoError(t, json.Unmarshal([]byte(raw), &request))
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(raw))
		metalib.Set2Context(c, &m)
		provider := &openai.Adaptor{ChannelType: channeltype.OpenAICompatible}
		provider.Init(&m)
		reader, err := getRequestBody(c, &m, &request, provider, false)
		require.NoError(t, err)
		body, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.NotContains(t, string(body), "extra_content")
		limits[signed] = request.MaxCompletionTokens
	}
	require.NotNil(t, limits[false], "fixture: the adaptor applies a default output limit in place")
	require.NotNil(t, limits[true], "a replayed carrier must not hide adaptor normalization from billing")
	require.Equal(t, *limits[false], *limits[true])
}

// TestMCPChatRoundProviderMetadataIsolation proves tiered MCP rounds scope carriers before conversion.
func TestMCPChatRoundProviderMetadataIsolation(t *testing.T) {
	var request relaymodel.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal([]byte(strings.ReplaceAll(googleSignedChatHistory, "%MODEL%", "deepseek-chat")), &request))
	m := metalib.Meta{APIType: apitype.DeepSeek, ChannelType: channeltype.DeepSeek, ActualModelName: "deepseek-chat", OriginModelName: "deepseek-chat", Mode: relaymode.ChatCompletions}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader("{}"))
	metalib.Set2Context(c, &m)
	provider := &deepseek.Adaptor{}
	provider.Init(&m)
	body, failure := prepareMCPChatRound(c, &m, provider, &request)
	require.Nil(t, failure)
	require.NotContains(t, string(body), "c2lnL", "signatures must not reach a non-Google MCP round: %s", body)
	require.NotContains(t, string(body), "gemini_", "gateway metadata must not reach a non-Google MCP round: %s", body)
	require.Contains(t, string(body), `\"extra_content\":\"user-data\"`, "argument data is caller content")
}
