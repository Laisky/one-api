package openai

import (
	"encoding/json"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

// claudePromotionFixture returns a fresh single forced tool that normally opts into structured promotion.
func claudePromotionFixture() *model.ClaudeRequest {
	return &model.ClaudeRequest{Model: "caller-alias", MaxTokens: 128,
		Messages: []model.ClaudeMessage{{Role: "user", Content: "Return structured topic and confidence JSON."}},
		Tools: []model.ClaudeTool{{Name: "topic_classifier", Description: "Return structured topic and confidence data",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"topic": map[string]any{"type": "string"}, "confidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1}}, "required": []any{"topic"}, "additionalProperties": false}}},
		ToolChoice: map[string]any{"type": "tool", "name": "topic_classifier"},
	}
}

// TestGemini4ClaudeFormatAndForcedToolWire preserves explicit native output and named tool intent together.
func TestGemini4ClaudeFormatAndForcedToolWire(t *testing.T) {
	for _, name := range []string{"gemini-4-argon", "gemini-4-carbon", "gemini-4-hydrogen"} {
		t.Run(name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/messages", nil)
			meta.Set2Context(ctx, &meta.Meta{Mode: relaymode.ClaudeMessages, ChannelType: channeltype.GeminiOpenAICompatible, ModelMapping: map[string]string{"caller-alias": name}})
			source := claudePromotionFixture()
			source.OutputConfig = json.RawMessage(`{"effort":"high","format":{"type":"json_schema","name":"native_output","schema":{"type":"object","properties":{"count":{"type":"integer","minimum":1,"maximum":7}},"required":["count"],"additionalProperties":false}}}`)
			before, err := json.Marshal(source)
			require.NoError(t, err)
			converted, err := (&Adaptor{}).ConvertClaudeRequest(ctx, source)
			require.NoError(t, err)
			request, ok := converted.(*model.GeneralOpenAIRequest)
			require.True(t, ok)
			for round := 0; round < 2; round++ {
				if round > 0 {
					_, err = (&Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, request)
					require.NoError(t, err)
				}
				wire, err := json.Marshal(request)
				require.NoError(t, err)
				var payload map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(wire, &payload))
				require.Contains(t, payload, "tools")
				require.JSONEq(t, `{"type":"function","function":{"name":"topic_classifier"}}`, string(payload["tool_choice"]))
				require.JSONEq(t, `"high"`, string(payload["reasoning_effort"]))
				require.Contains(t, string(payload["response_format"]), `"name":"native_output"`)
				require.Contains(t, string(payload["response_format"]), `"minimum":1`)
				require.Contains(t, string(payload["response_format"]), `"maximum":7`)
				require.NotContains(t, payload, "output_config")
			}
			after, err := json.Marshal(source)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after), "caller input must not change")
		})
	}
}

// TestGemini4ClaudePromotionIsolation keeps legacy promotion when there is no explicit native format.
func TestGemini4ClaudePromotionIsolation(t *testing.T) {
	for _, tc := range []struct {
		name, actual, config string
		channel              int
	}{
		{"omitted", "gemini-4-hydrogen", "", channeltype.GeminiOpenAICompatible},
		{"effort-only", "gemini-4-hydrogen", `{"effort":"high"}`, channeltype.GeminiOpenAICompatible},
		{"legacy", "gemini-2.5-pro", `{"format":{"type":"json_schema","schema":{"type":"object"}}}`, channeltype.GeminiOpenAICompatible},
		{"unrelated", "gemini-4-hydrogen", `{"format":{"type":"json_schema","schema":{"type":"object"}}}`, channeltype.OpenAICompatible},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/messages", nil)
			meta.Set2Context(ctx, &meta.Meta{ChannelType: tc.channel, ActualModelName: tc.actual})
			source := claudePromotionFixture()
			source.OutputConfig = json.RawMessage(tc.config)
			converted, err := (&Adaptor{}).ConvertClaudeRequest(ctx, source)
			require.NoError(t, err)
			request, ok := converted.(*model.GeneralOpenAIRequest)
			require.True(t, ok)
			require.NotNil(t, request.ResponseFormat)
			require.Equal(t, "topic_classifier", request.ResponseFormat.JsonSchema.Name)
			require.Empty(t, request.Tools)
			require.Nil(t, request.ToolChoice)
		})
	}
}

// TestGemini4ClaudeEquivalentFormatRetainsTool prevents coincidentally equivalent formats from erasing forced tools.
func TestGemini4ClaudeEquivalentFormatRetainsTool(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	meta.Set2Context(ctx, &meta.Meta{ChannelType: channeltype.GeminiOpenAICompatible, ActualModelName: "gemini-4-hydrogen"})
	source := claudePromotionFixture()
	raw, err := json.Marshal(map[string]any{"format": map[string]any{"type": "json_schema", "name": "topic_classifier", "description": source.Tools[0].Description, "strict": true, "schema": source.Tools[0].InputSchema}})
	require.NoError(t, err)
	source.OutputConfig = raw
	converted, err := (&Adaptor{}).ConvertClaudeRequest(ctx, source)
	require.NoError(t, err)
	request := converted.(*model.GeneralOpenAIRequest)
	require.Len(t, request.Tools, 1)
	require.NotNil(t, request.ToolChoice)
}
