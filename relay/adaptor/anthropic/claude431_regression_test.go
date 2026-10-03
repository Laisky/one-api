package anthropic

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestClaude431ThinkingConversion checks mode, effort, limits, and caller ownership at the public converter.
func TestClaude431ThinkingConversion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ mode, want string }{
		{"between_tools", "between_tools"}, {"disabled", "between_tools"}, {"enabled", "adaptive"}, {"adaptive", "adaptive"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			t.Parallel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			raw := `{"model":"claude-sonnet-5-5","max_tokens":256,"temperature":0.4,"top_p":0.8,"top_k":20,"thinking":{"type":"` + tc.mode + `"},"output_config":{"effort":"medium","future_id":9007199254740993},"messages":[{"role":"user","content":"Hello"}]}`
			var request model.GeneralOpenAIRequest
			require.NoError(t, json.Unmarshal([]byte(raw), &request))
			originalThinking := *request.Thinking
			converted, err := ConvertRequest(c, request)
			require.NoError(t, err)
			body, err := json.Marshal(converted)
			require.NoError(t, err)
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body, &fields))
			require.JSONEq(t, `{"type":"`+tc.want+`"}`, string(fields["thinking"]))
			require.Equal(t, `{"effort":"medium","future_id":9007199254740993}`, string(fields["output_config"]))
			require.Equal(t, "256", string(fields["max_tokens"]))
			for _, key := range []string{"temperature", "top_p", "top_k"} {
				require.NotContains(t, fields, key)
			}
			require.Equal(t, originalThinking, *request.Thinking, "conversion must not mutate shared caller thinking")
		})
	}
}

// TestClaude431InvalidControls rejects unsupported semantics rather than silently weakening a request.
func TestClaude431InvalidControls(t *testing.T) {
	t.Parallel()
	for _, extra := range []string{
		`"thinking":{"type":"between_tools"},"output_config":{"effort":"xhigh"}`,
		`"thinking":{"type":"between_tools"},"output_config":{"effort":"max"}`,
		`"thinking":{"type":"between_tools","display":"summarized"}`,
		`"thinking":{"type":"between_tools","budget_tokens":2048}`,
		`"thinking":{"type":"between_tools","block_binding":{"prefix_mismatch_behavior":"error"}}`,
		`"tool_choice":"required"`, `"tool_choice":"any"`,
		`"tool_choice":{"type":"function","function":{"name":"lookup"}}`,
		`"tool_choice":{"type":"tool","name":"lookup"}`,
		`"output_config":{"effort":"not-an-effort"}`,
	} {
		t.Run(extra, func(t *testing.T) {
			t.Parallel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			raw := `{"model":"claude-sonnet-5-5","max_tokens":4096,"messages":[{"role":"user","content":"Hello"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}],` + extra + `}`
			var request model.GeneralOpenAIRequest
			require.NoError(t, json.Unmarshal([]byte(raw), &request))
			_, err := ConvertRequest(c, request)
			require.ErrorContains(t, err, "validation failed")
		})
	}
}

// TestClaude431ConvertedPortableControls checks supported choices, exact schemas, effort, and output limits.
func TestClaude431ConvertedPortableControls(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"claude-sonnet-5-5", "claude-sonnet-4-6"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			var request model.GeneralOpenAIRequest
			require.NoError(t, json.Unmarshal([]byte(`{"model":"`+name+`","max_completion_tokens":256,"thinking":{"type":"disabled"},"reasoning_effort":"medium","stop":["END"],"tool_choice":"none","tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","additionalProperties":false,"properties":{"x":{"type":"string"}},"required":["x"]}}}],"messages":[{"role":"user","content":"Hello"}]}`), &request))
			converted, err := ConvertRequest(c, request)
			require.NoError(t, err)
			body, err := json.Marshal(converted)
			require.NoError(t, err)
			var got map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body, &got))
			require.Equal(t, "256", string(got["max_tokens"]))
			require.JSONEq(t, `{"type":"none"}`, string(got["tool_choice"]))
			require.JSONEq(t, `["END"]`, string(got["stop_sequences"]))
			require.Contains(t, string(got["tools"]), `"additionalProperties":false`)
			if name == "claude-sonnet-5-5" {
				require.JSONEq(t, `{"effort":"medium"}`, string(got["output_config"]))
			}
		})
	}
}
