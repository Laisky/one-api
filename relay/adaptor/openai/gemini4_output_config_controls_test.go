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

// TestGemini4OutputControlsIsolation preserves existing controls outside the exact Gemini 4 channel contract.
func TestGemini4OutputControlsIsolation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		channel int
		model   string
	}{
		{"legacy-gemini", channeltype.GeminiOpenAICompatible, "gemini-2.5-pro"},
		{"unrelated-channel", channeltype.OpenAICompatible, "gemini-4-hydrogen"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			meta.Set2Context(ctx, &meta.Meta{ChannelType: tc.channel, ActualModelName: tc.model, Mode: relaymode.ChatCompletions})
			summary, topK := "detailed", 7
			config := json.RawMessage(`{"effort":"high","format":{"type":"json_schema","schema":{"type":"object","additionalProperties":false}}}`)
			request := &model.GeneralOpenAIRequest{
				Model: tc.model, Messages: []model.Message{{Role: "user", Content: "extract"}},
				OutputConfig: config, TopK: &topK,
				Reasoning: &model.OpenAIResponseReasoning{Summary: &summary},
			}
			converted, err := (&Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, request)
			require.NoError(t, err)
			wire, err := json.Marshal(converted)
			require.NoError(t, err)
			var payload map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(wire, &payload))
			require.JSONEq(t, string(config), string(payload["output_config"]))
			require.JSONEq(t, `7`, string(payload["top_k"]))
			require.JSONEq(t, `{"summary":"detailed"}`, string(payload["reasoning"]))
			require.NotContains(t, payload, "response_format")
		})
	}
}

// TestGemini4ClaudeOutputFormatOwnership preserves native schema constraints and the source Claude request.
func TestGemini4ClaudeOutputFormatOwnership(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	meta.Set2Context(ctx, &meta.Meta{
		ChannelType: channeltype.GeminiOpenAICompatible, Mode: relaymode.ClaudeMessages,
		ModelMapping: map[string]string{"tenant-alias": "gemini-4-hydrogen"},
	})
	config := `{"effort":"xhigh","format":{"type":"json_schema","schema":{"type":"object","properties":{"kind":{"type":"string","const":"note"}},"required":["kind"],"additionalProperties":false}}}`
	source := &model.ClaudeRequest{
		Model: "tenant-alias", MaxTokens: 1024,
		Messages:     []model.ClaudeMessage{{Role: "user", Content: "extract"}},
		OutputConfig: json.RawMessage(config),
	}
	converted, err := (&Adaptor{}).ConvertClaudeRequest(ctx, source)
	require.NoError(t, err)
	require.Equal(t, config, string(source.OutputConfig), "source raw bytes remain caller-owned")
	wire, err := json.Marshal(converted)
	require.NoError(t, err)
	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(wire, &payload))
	require.NotContains(t, payload, "output_config")
	require.JSONEq(t, `"xhigh"`, string(payload["reasoning_effort"]))
	var format struct {
		Type   string `json:"type"`
		Schema struct {
			Schema json.RawMessage `json:"schema"`
		} `json:"json_schema"`
	}
	require.NoError(t, json.Unmarshal(payload["response_format"], &format))
	require.Equal(t, "json_schema", format.Type)
	require.JSONEq(t, `{"type":"object","properties":{"kind":{"type":"string","const":"note"}},"required":["kind"],"additionalProperties":false}`, string(format.Schema.Schema))
	result, ok := converted.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, result.ResponseFormat)
	require.NotNil(t, result.ResponseFormat.JsonSchema)
	properties, ok := result.ResponseFormat.JsonSchema.Schema["properties"].(map[string]any)
	require.True(t, ok)
	kind, ok := properties["kind"].(map[string]any)
	require.True(t, ok)
	kind["const"] = "edited"
	require.Equal(t, config, string(source.OutputConfig), "mutating converted nested schema must not alter source raw bytes")
}

// TestGemini4EquivalentOutputFormatCarriers accepts matching constraints while rejecting differing schemas.
func TestGemini4EquivalentOutputFormatCarriers(t *testing.T) {
	for _, tc := range []struct {
		name, otherType string
		conflict        bool
	}{
		{"equivalent", "object", false},
		{"conflicting", "array", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{"model":"gemini-4-hydrogen","input":"extract","output_config":{"format":{"type":"json_schema","name":"extract","strict":true,"schema":{"type":"object"}}},"text":{"format":{"type":"json_schema","name":"extract","strict":true,"schema":{"type":"` + tc.otherType + `"}}}}`
			payload, err := gemini4FormatPayload(t, raw)
			if tc.conflict {
				require.ErrorContains(t, err, "conflicting")
				return
			}
			require.NoError(t, err)
			require.Contains(t, payload, "response_format")
			require.NotContains(t, payload, "output_config")
		})
	}
}

// TestGemini4ExplicitNonStrictFormat preserves an explicit false strict control in the upstream payload.
func TestGemini4ExplicitNonStrictFormat(t *testing.T) {
	payload, err := gemini4FormatPayload(t, `{"model":"gemini-4-hydrogen","input":"extract","output_config":{"format":{"type":"json_schema","strict":false,"schema":{"type":"object","properties":{"kind":{"const":"note"}}}}}}`)
	require.NoError(t, err)
	var format struct {
		Type   string `json:"type"`
		Schema struct {
			Strict *bool           `json:"strict"`
			Schema json.RawMessage `json:"schema"`
		} `json:"json_schema"`
	}
	require.NoError(t, json.Unmarshal(payload["response_format"], &format))
	require.Equal(t, "json_schema", format.Type)
	require.NotNil(t, format.Schema.Strict, "explicit false must remain distinguishable from omission")
	require.False(t, *format.Schema.Strict)
	require.JSONEq(t, `{"type":"object","properties":{"kind":{"const":"note"}}}`, string(format.Schema.Schema))
}

// TestGemini4BlankSummaryAllowsEffort treats blank summaries as absent without discarding reasoning effort.
func TestGemini4BlankSummaryAllowsEffort(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{"empty", ""}, {"whitespace", "   "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := gemini4FormatPayload(t, `{"model":"gemini-4-hydrogen","input":"extract","reasoning":{"effort":"xhigh","summary":"`+tc.value+`"}}`)
			require.NoError(t, err)
			require.NotContains(t, payload, "reasoning")
			require.JSONEq(t, `"xhigh"`, string(payload["reasoning_effort"]))
		})
	}
}

// TestGemini4TopKSuppression removes all fixed sampling controls independently of summary validation.
func TestGemini4TopKSuppression(t *testing.T) {
	for _, modelName := range []string{"gemini-4-argon", "gemini-4-carbon", "gemini-4-hydrogen"} {
		t.Run(modelName, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			meta.Set2Context(ctx, &meta.Meta{ChannelType: channeltype.GeminiOpenAICompatible, ActualModelName: modelName, Mode: relaymode.ChatCompletions})
			topK, temperature, topP := 7, 0.0, 0.9
			request := &model.GeneralOpenAIRequest{
				Model: modelName, Messages: []model.Message{{Role: "user", Content: "extract"}},
				TopK: &topK, Temperature: &temperature, TopP: &topP,
			}
			converted, err := (&Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, request)
			require.NoError(t, err)
			wire, err := json.Marshal(converted)
			require.NoError(t, err)
			var payload map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(wire, &payload))
			require.NotContains(t, payload, "top_k")
			require.NotContains(t, payload, "temperature")
			require.NotContains(t, payload, "top_p")
			require.Equal(t, 7, topK)
			require.Equal(t, 0.0, temperature)
			require.Equal(t, 0.9, topP)
		})
	}
}
