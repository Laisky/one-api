package openai

import (
	"bytes"
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

// TestNumericBoundsProviderWire compares documented provider/model capabilities through actual adaptation.
func TestNumericBoundsProviderWire(t *testing.T) {
	for _, tc := range []struct {
		name, actual         string
		channel              int
		inclusive, exclusive bool
	}{
		{"gemini", "gemini-4-hydrogen", channeltype.GeminiOpenAICompatible, true, false},
		{"openai-standard", "gpt-6.1-sol", channeltype.OpenAI, true, true},
		{"vertex-google", "gemini-4-hydrogen", channeltype.VertextAI, true, false},
		{"vertex-partner", "claude-sonnet-4-6", channeltype.VertextAI, false, false},
		{"azure", "gpt-6.1-sol", channeltype.Azure, false, false},
		{"fine-tuned", "ft:gpt-4o:local:fixture", channeltype.OpenAI, false, false},
		{"unknown-model", "unknown-fixture", channeltype.GeminiOpenAICompatible, false, false},
		{"unverified-compatible", "gpt-6.1-sol", channeltype.OpenAICompatible, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			meta.Set2Context(ctx, &meta.Meta{ChannelType: tc.channel, ActualModelName: tc.actual, Mode: relaymode.ChatCompletions, ResponseAPIFallback: true})
			schema := map[string]any{"type": "object", "properties": map[string]any{"n": map[string]any{"type": "number", "minimum": json.Number("9007199254740993"), "maximum": json.Number("9007199254740997"), "exclusiveMinimum": json.Number("9007199254740994"), "exclusiveMaximum": json.Number("9007199254740996")}}, "required": []any{"n"}, "additionalProperties": false}
			strict := true
			before, err := json.Marshal(schema)
			require.NoError(t, err)
			req := &model.GeneralOpenAIRequest{Model: "caller-alias", ResponseFormat: &model.ResponseFormat{Type: "json_schema", JsonSchema: &model.JSONSchema{Name: "bounded", Schema: schema, Strict: &strict}}}
			converted, err := (&Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, req)
			require.NoError(t, err)
			wire, err := json.Marshal(converted)
			require.NoError(t, err)
			for _, key := range []string{"minimum", "maximum"} {
				require.Equal(t, tc.inclusive, containsJSONKey(wire, key), "%s: %s", key, wire)
			}
			for _, key := range []string{"exclusiveMinimum", "exclusiveMaximum"} {
				require.Equal(t, tc.exclusive, containsJSONKey(wire, key), "%s: %s", key, wire)
			}
			if tc.inclusive {
				require.Contains(t, string(wire), "9007199254740993")
			}
			after, err := json.Marshal(schema)
			require.NoError(t, err)
			require.Equal(t, string(before), string(after), "normalization must not modify caller maps")
		})
	}
}

// containsJSONKey reports a literal serialized schema key without rounding numeric values.
func containsJSONKey(wire []byte, key string) bool {
	return bytes.Contains(wire, []byte(`"`+key+`":`))
}

// TestNumericSanitizerSchemaContext retains property/definition names and literal instance data.
func TestNumericSanitizerSchemaContext(t *testing.T) {
	source := map[string]any{"type": "object", "properties": map[string]any{
		"minimum": map[string]any{"type": "number", "minimum": 2, "maximum": 7},
		"literal": map[string]any{"const": map[string]any{"minimum": 13, "maximum": 20}},
	}, "$defs": map[string]any{"maximum": map[string]any{"type": "number", "maximum": 11}}}
	before, err := json.Marshal(source)
	require.NoError(t, err)
	got, changed := NormalizeStructuredJSONSchema(source, channeltype.Azure)
	require.True(t, changed)
	props := got["properties"].(map[string]any)
	require.Contains(t, props, "minimum")
	require.NotContains(t, props["minimum"], "minimum")
	require.NotContains(t, props["minimum"], "maximum")
	require.Equal(t, map[string]any{"minimum": 13, "maximum": 20}, props["literal"].(map[string]any)["const"])
	require.Contains(t, got["$defs"], "maximum")
	require.NotContains(t, got["$defs"].(map[string]any)["maximum"], "maximum")
	after, err := json.Marshal(source)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))
}
