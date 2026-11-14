package vertexai

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/relaymode"
)

// TestVertexToolChoiceModes checks modern and legacy selection retain the caller's requested constraints.
func TestVertexToolChoiceModes(t *testing.T) {
	for _, tc := range []struct {
		name           string
		choice, legacy any
		mode           string
		names          []string
	}{
		{"required", "required", nil, "ANY", nil}, {"none", "none", nil, "NONE", nil}, {"auto", "auto", nil, "AUTO", nil},
		{"named", map[string]any{"type": "function", "function": map[string]any{"name": "record"}}, nil, "ANY", []string{"record"}},
		{"responses_named", map[string]any{"type": "function", "name": "record"}, nil, "ANY", []string{"record"}},
		{"claude_any", map[string]any{"type": "any"}, nil, "ANY", nil},
		{"claude_tool", map[string]any{"type": "tool", "name": "record"}, nil, "ANY", []string{"record"}},
		{"legacy_named", nil, map[string]any{"name": "record"}, "ANY", []string{"record"}},
		{"legacy_none", nil, "none", "NONE", nil}, {"legacy_auto", nil, "auto", "AUTO", nil},
		{"modern_wins", "none", map[string]any{"name": "ignored"}, "NONE", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := vertexSchemaRequest(map[string]any{"type": "object"})
			request.ToolChoice = tc.choice
			request.FunctionCall = tc.legacy
			var wire struct {
				Config *struct {
					Function struct {
						Mode  string   `json:"mode"`
						Names []string `json:"allowed_function_names"`
					} `json:"function_calling_config"`
				} `json:"tool_config"`
			}
			require.NoError(t, json.Unmarshal(vertexSchemaWire(t, request), &wire))
			require.NotNil(t, wire.Config)
			require.Equal(t, tc.mode, wire.Config.Function.Mode)
			require.Equal(t, tc.names, wire.Config.Function.Names)
		})
	}
}

// TestVertexToolChoiceInvalid rejects malformed or unavailable named choices rather than silently using AUTO.
func TestVertexToolChoiceInvalid(t *testing.T) {
	for _, choice := range []any{"unsupported", 123, map[string]any{"type": "function"}, map[string]any{"type": "function", "function": map[string]any{"name": "missing"}}} {
		request := vertexSchemaRequest(map[string]any{"type": "object"})
		request.ToolChoice = choice
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		converted, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, request)
		require.ErrorContains(t, err, "validation failed")
		require.Nil(t, converted)
	}
}
