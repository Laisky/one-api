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

// TestGemini4RepeatedNativeOutputFormat preserves adapted constraints across retry and shallow-copy conversions.
func TestGemini4RepeatedNativeOutputFormat(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	meta.Set2Context(ctx, &meta.Meta{ChannelType: channeltype.GeminiOpenAICompatible, ActualModelName: "gemini-4-hydrogen"})
	raw := json.RawMessage(`{"format":{"type":"json_schema","schema":{"type":"object","properties":{"count":{"type":"integer","minimum":2,"maximum":7}}}}}`)
	request := &model.GeneralOpenAIRequest{Model: "gemini-4-hydrogen", OutputConfig: raw}
	for round := 0; round < 3; round++ {
		converted, err := (&Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, request)
		require.NoError(t, err)
		wire, err := json.Marshal(converted)
		require.NoError(t, err)
		require.Contains(t, string(wire), `"minimum":2`, "conversion %d", round)
		require.Contains(t, string(wire), `"maximum":7`, "conversion %d", round)
		require.NotContains(t, string(wire), "schemaPreservation")
		require.Empty(t, request.OutputConfig)
		copied := *converted.(*model.GeneralOpenAIRequest)
		request = &copied
	}
	require.Contains(t, string(raw), `"minimum":2`, "caller-owned raw data remains intact")
	// Replacing the actual schema must not inherit the translated schema's policy.
	request.ResponseFormat.JsonSchema = &model.JSONSchema{Name: "ordinary", Schema: map[string]any{"type": "integer", "minimum": 2}}
	_, err := (&Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, request)
	require.NoError(t, err)
	require.NotContains(t, request.ResponseFormat.JsonSchema.Schema, "minimum")
}

// TestGemini4OrdinaryResponseFormatNormalization retains the ordinary and unrelated-provider sanitizer contracts.
func TestGemini4OrdinaryResponseFormatNormalization(t *testing.T) {
	for _, channel := range []int{channeltype.GeminiOpenAICompatible, channeltype.OpenAICompatible} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
		meta.Set2Context(ctx, &meta.Meta{ChannelType: channel, ActualModelName: "gemini-4-hydrogen"})
		req := &model.GeneralOpenAIRequest{Model: "gemini-4-hydrogen", ResponseFormat: &model.ResponseFormat{
			Type: "json_schema", JsonSchema: &model.JSONSchema{Name: "ordinary", Schema: map[string]any{"type": "integer", "minimum": 2, "maximum": 7}},
		}}
		for round := 0; round < 2; round++ {
			_, err := (&Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, req)
			require.NoError(t, err)
			require.NotContains(t, req.ResponseFormat.JsonSchema.Schema, "minimum")
			require.NotContains(t, req.ResponseFormat.JsonSchema.Schema, "maximum")
		}
	}
}

// TestGemini4NativeSchemaProviderIsolation keeps destination changes on their existing normalization policy.
func TestGemini4NativeSchemaProviderIsolation(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	info := &meta.Meta{ChannelType: channeltype.GeminiOpenAICompatible, ActualModelName: "gemini-4-hydrogen"}
	meta.Set2Context(ctx, info)
	request := &model.GeneralOpenAIRequest{Model: "gemini-4-hydrogen", OutputConfig: json.RawMessage(`{"format":{"type":"json_schema","schema":{"type":"integer","minimum":2,"maximum":7}}}`)}
	_, err := (&Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, request)
	require.NoError(t, err)
	require.Contains(t, request.ResponseFormat.JsonSchema.Schema, "minimum")
	info.ChannelType = channeltype.OpenAICompatible
	_, err = (&Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, request)
	require.NoError(t, err)
	require.NotContains(t, request.ResponseFormat.JsonSchema.Schema, "minimum")
	require.NotContains(t, request.ResponseFormat.JsonSchema.Schema, "maximum")
}
