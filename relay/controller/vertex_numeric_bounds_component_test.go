package controller

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestVertexNumericBoundsResponseComponent preserves bounds through preprocessing, trusted fallback, and native serialization without authentication or HTTP calls.
func TestVertexNumericBoundsResponseComponent(t *testing.T) {
	const actual = "gemini-4-hydrogen"
	raw := `{"model":"` + actual + `","input":"Extract values.","tools":[{"type":"function","name":"probe","parameters":` + geminiNumericBoundsSchema + `}],"text":{"format":{"type":"json_schema","name":"bounded","schema":` + geminiNumericBoundsSchema + `}}}`
	var source openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal([]byte(raw), &source))
	info := &metalib.Meta{
		APIType: apitype.VertexAI, ChannelType: channeltype.VertextAI,
		ActualModelName: actual, OriginModelName: "friendly-alias", Mode: relaymode.ChatCompletions,
	}
	// This is the existing preprocessing stage before Vertex's trusted schema-fidelity option.
	sanitizeResponseAPIRequest(&source, info.ChannelType)
	options := responseFallbackConversionOptions(info)
	require.True(t, options.PreserveJSONSchema)
	require.True(t, options.RequireToolResultPairing)
	before, err := json.Marshal(source)
	require.NoError(t, err)
	request, err := openai.ConvertResponseAPIToChatCompletionRequestWithOptions(&source, nil, options)
	require.NoError(t, err)
	request.Model = info.ActualModelName
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	metalib.Set2Context(c, info)
	converted, err := (&vertexai.Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, request)
	require.NoError(t, err)
	body, err := json.Marshal(converted)
	require.NoError(t, err)
	var wire struct {
		Generation struct {
			Schema json.RawMessage `json:"responseJsonSchema"`
		} `json:"generation_config"`
		Tools []struct {
			Functions []struct {
				Schema json.RawMessage `json:"parametersJsonSchema"`
			} `json:"function_declarations"`
		} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(body, &wire))
	require.Len(t, wire.Tools, 1)
	require.Len(t, wire.Tools[0].Functions, 1)
	t.Run("response-schema", func(t *testing.T) { assertGeminiNumericBounds(t, wire.Generation.Schema) })
	t.Run("function-schema", func(t *testing.T) { assertGeminiNumericBounds(t, wire.Tools[0].Functions[0].Schema) })
	after, err := json.Marshal(source)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after), "fallback and native conversion must not mutate their source schema")
}
