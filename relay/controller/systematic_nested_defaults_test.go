package controller

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// TestSystematicNestedThinkingDefaults verifies that unrelated template options
// cannot swallow a query-injected default. Parameters: t runs root/extra-body
// cases. Returns: none; explicit settings and opaque integer precision survive.
func TestSystematicNestedThinkingDefaults(t *testing.T) {
	for _, raw := range []string{
		`{"model":"Qwen/Qwen3.5-35B-A3B","input":"test","extra_body":{"chat_template_kwargs":{"counter":9007199254740993}}}`,
		`{"model":"Qwen/Qwen3.5-35B-A3B","input":"test","chat_template_kwargs":{"counter":9007199254740993}}`,
	} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest("POST", "/v1/responses?thinking=true", nil)
		var req openai.ResponseAPIRequest
		require.NoError(t, json.Unmarshal([]byte(raw), &req))
		meta := &metalib.Meta{ActualModelName: req.Model, ChannelType: channeltype.OpenAICompatible, APIType: channeltype.ToAPIType(channeltype.OpenAICompatible)}
		applyThinkingQueryToResponseRequest(ctx, &req, meta)
		body, _, _, err := normalizeResponseAPIRawBody([]byte(raw), &req, meta.ChannelType)
		require.NoError(t, err)
		var root map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(body, &root))
		var kwargs map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(root["chat_template_kwargs"], &kwargs))
		require.Equal(t, "true", string(kwargs["enable_thinking"]))
		require.Equal(t, "9007199254740993", string(kwargs["counter"]))
		again, _, changed, err := normalizeResponseAPIRawBody(body, &req, meta.ChannelType)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, body, again)
	}
}

// TestSystematicMalformedExtensionDiagnostic counts one rejected transport field
// across its raw and converted representations. Parameters: t runs the matrix.
// Returns: none; malformed payloads must never appear in the forwarded JSON.
func TestSystematicMalformedExtensionDiagnostic(t *testing.T) {
	for _, converted := range []string{`"not-an-object"`, `"different-invalid-object"`, `[]`} {
		original := []byte(`{"model":"custom","extra_body":"not-an-object"}`)
		updated := []byte(`{"model":"custom","extra_body":` + converted + `}`)
		body, stats, _, err := mergeControlledPassthroughJSON(original, updated, false)
		require.NoError(t, err)
		require.Equal(t, 1, stats.ExtraBodyRejected)
		require.JSONEq(t, `{"model":"custom"}`, string(body))
	}
}
