package vertexai

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// vertexIngressSchema returns a JSON Schema fixture for ingress regression tests.
func vertexIngressSchema(t *testing.T) map[string]any {
	t.Helper()
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"type":"object","additionalProperties":false,"properties":{"entries":{"type":"array","items":{"$ref":"#/$defs/entry"}},"optional":{"anyOf":[{"type":"string"},{"type":"null"}]}},"required":["entries"],"$defs":{"entry":{"type":"object","properties":{"kind":{"type":"string","const":"note"},"payload":{"oneOf":[{"type":"string"},{"type":"integer"}]}},"required":["kind"]}}}`), &schema))
	return schema
}

// vertexIngressContext returns a request context for Vertex Gemini dispatch.
func vertexIngressContext(path string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", path, nil)
	meta.Set2Context(c, &meta.Meta{ActualModelName: "gemini-3.8-flash", OriginModelName: "gemini-3.8-flash"})
	return c
}

// vertexIngressWire returns the serialized request object for assertions.
func vertexIngressWire(t *testing.T, value any) map[string]any {
	t.Helper()
	body, err := json.Marshal(value)
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(body, &wire))
	return wire
}

// TestVertexResponsesIngressPreservesJSONSchema checks Responses lowering, Vertex dispatch, output schemas, and source ownership.
func TestVertexResponsesIngressPreservesJSONSchema(t *testing.T) {
	schema := vertexIngressSchema(t)
	strict := true
	toolSchema := vertexIngressSchema(t)
	delete(toolSchema, "additionalProperties")
	request := &openai.ResponseAPIRequest{
		Model: "gemini-3.8-flash",
		Tools: []openai.ResponseAPITool{{Type: "function", Name: "store_note", Parameters: toolSchema, Strict: &strict}},
		Text:  &openai.ResponseTextConfig{Format: &openai.ResponseTextFormat{Type: "json_schema", Name: "result", Schema: schema, Strict: &strict}},
	}
	require.NoError(t, json.Unmarshal([]byte(`"Save this note"`), &request.Input))
	before, err := json.Marshal(request)
	require.NoError(t, err)
	chat, err := openai.ConvertResponseAPIToChatCompletionRequest(request, true)
	require.NoError(t, err)
	converted, err := (&Adaptor{}).ConvertRequest(vertexIngressContext("/v1/responses"), relaymode.ChatCompletions, chat)
	require.NoError(t, err)
	wire := vertexIngressWire(t, converted)
	tools := wire["tools"].([]any)
	declarations := tools[0].(map[string]any)["function_declarations"].([]any)
	declaration := declarations[0].(map[string]any)
	require.NotContains(t, declaration, "parameters")
	require.Equal(t, toolSchema, declaration["parametersJsonSchema"])
	generation := wire["generation_config"].(map[string]any)
	require.NotContains(t, generation, "responseSchema")
	require.Equal(t, schema, generation["responseJsonSchema"])
	require.Equal(t, "application/json", generation["responseMimeType"])
	after, err := json.Marshal(request)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}

// TestVertexClaudeIngressPreservesJSONSchema checks Claude native conversion and Vertex dispatch retain the caller input schema.
func TestVertexClaudeIngressPreservesJSONSchema(t *testing.T) {
	schema := vertexIngressSchema(t)
	request := &model.ClaudeRequest{
		Model: "gemini-3.8-flash", MaxTokens: 64,
		Messages: []model.ClaudeMessage{{Role: "user", Content: "Save this note"}},
		Tools:    []model.ClaudeTool{{Name: "store_note", Description: "Store a note", InputSchema: schema}},
	}
	before, err := json.Marshal(request)
	require.NoError(t, err)
	converted, err := (&Adaptor{}).ConvertClaudeRequest(vertexIngressContext("/v1/messages"), request)
	require.NoError(t, err)
	wire := vertexIngressWire(t, converted)
	tools := wire["tools"].([]any)
	declarations := tools[0].(map[string]any)["function_declarations"].([]any)
	declaration := declarations[0].(map[string]any)
	require.NotContains(t, declaration, "parameters")
	require.Equal(t, schema, declaration["parametersJsonSchema"])
	after, err := json.Marshal(request)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}

// TestVertexResponsesOmittedStrictRetainsSanitizerBoundary records the existing cross-provider sanitizer before Vertex conversion, including unconditional tool sanitation.
func TestVertexResponsesOmittedStrictRetainsSanitizerBoundary(t *testing.T) {
	schema := vertexIngressSchema(t)
	strict := true
	request := &openai.ResponseAPIRequest{
		Model: "gemini-3.8-flash",
		Tools: []openai.ResponseAPITool{{Type: "function", Name: "store_note", Parameters: schema, Strict: &strict}},
		Text:  &openai.ResponseTextConfig{Format: &openai.ResponseTextFormat{Type: "json_schema", Name: "result", Schema: schema}},
	}
	require.NoError(t, json.Unmarshal([]byte(`"Save this note"`), &request.Input))
	before, err := json.Marshal(request)
	require.NoError(t, err)
	chat, err := openai.ConvertResponseAPIToChatCompletionRequest(request, true)
	require.NoError(t, err)
	require.Len(t, chat.Tools, 1)
	require.NotNil(t, chat.Tools[0].Function)
	toolSchema, ok := chat.Tools[0].Function.Parameters.(map[string]any)
	require.True(t, ok)
	require.NotContains(t, toolSchema, "additionalProperties")
	require.NotNil(t, chat.ResponseFormat)
	require.NotNil(t, chat.ResponseFormat.JsonSchema)
	require.NotContains(t, chat.ResponseFormat.JsonSchema.Schema, "additionalProperties")
	require.Equal(t, false, schema["additionalProperties"], "the source schema must retain its constraint")
	require.Equal(t, schema["$defs"], toolSchema["$defs"], "const and references survive this separate sanitizer")
	after, err := json.Marshal(request)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}
