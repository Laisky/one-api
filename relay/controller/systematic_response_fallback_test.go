package controller

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// TestSystematicResponseFallbackNormalizesTypedPayload checks that rebuilding
// an unavailable raw body does not bypass content and extension normalization.
// Parameters: t runs each fallback shape. Returns: none; assertions inspect JSON.
func TestSystematicResponseFallbackNormalizesTypedPayload(t *testing.T) {
	const typed = `{"model":"custom","input":[{"role":"assistant","content":[{"type":"input_text","text":"history"}]}],"extra_body":{"enable_thinking":true,"chat_template_kwargs":{"enable_thinking":true,"counter":9007199254740993},"model":"wrong-model","quota":0}}`
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"whitespace", " \n\t"},
		{"invalid_json", "not JSON"},
		{"partial_object", `{"untrusted_partial_field":true,`},
		{"array", `[]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var request openai.ResponseAPIRequest
			decoder := json.NewDecoder(strings.NewReader(typed))
			decoder.UseNumber()
			require.NoError(t, decoder.Decode(&request))

			body, stats, changed, err := normalizeResponseAPIRawBody([]byte(tc.raw), &request, channeltype.OpenAICompatible)
			require.NoError(t, err)
			require.True(t, changed)
			var root map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body, &root))
			require.Equal(t, `"custom"`, string(root["model"]))
			require.NotContains(t, root, "extra_body")
			require.NotContains(t, root, "quota")
			require.NotContains(t, root, "untrusted_partial_field")
			require.Equal(t, "true", string(root["enable_thinking"]))
			require.Contains(t, string(root["chat_template_kwargs"]), "9007199254740993")
			require.JSONEq(t, `[{"role":"assistant","content":[{"type":"output_text","text":"history"}]}]`, string(root["input"]))
			require.Equal(t, 1, stats.AssistantInputTextFixed)

			again, _, changed, err := normalizeResponseAPIRawBody(body, &request, channeltype.OpenAICompatible)
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, body, again)
		})
	}
}

// TestSystematicResponseFallbackRetainsQueryThinking exercises actual Qwen
// query injection before fallback serialization, including explicit false.
// Parameters: t runs both query values. Returns: none; no network call is made.
func TestSystematicResponseFallbackRetainsQueryThinking(t *testing.T) {
	for _, value := range []string{"true", "false"} {
		t.Run(value, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/responses?thinking="+value, nil)
			request := &openai.ResponseAPIRequest{Model: "Qwen/Qwen3.5-35B-A3B"}
			meta := &metalib.Meta{
				ActualModelName: request.Model,
				ChannelType:     channeltype.OpenAICompatible,
				APIType:         channeltype.ToAPIType(channeltype.OpenAICompatible),
			}
			applyThinkingQueryToResponseRequest(ctx, request, meta)

			body, _, changed, err := normalizeResponseAPIRawBody(nil, request, meta.ChannelType)
			require.NoError(t, err)
			require.True(t, changed)
			var root map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body, &root))
			require.NotContains(t, root, "extra_body")
			require.JSONEq(t, `{"enable_thinking":`+value+`}`, string(root["chat_template_kwargs"]))
		})
	}
}
