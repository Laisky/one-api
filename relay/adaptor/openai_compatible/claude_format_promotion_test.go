package openai_compatible

import (
	"encoding/json"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

// TestGemini4ExplicitFormatPromotionPolicy checks field recognition and effective destination without mutating caller state.
func TestGemini4ExplicitFormatPromotionPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, actual, caller, raw string
		channel                   int
		want, wantErr             bool
	}{
		{"canonical", "gemini-4-hydrogen", "alias", `{"format":{"type":"json_schema","schema":{}}}`, channeltype.GeminiOpenAICompatible, true, false},
		{"casefold", " GEMINI-4-CARBON ", "alias", `{"FORMAT":{"type":"json_schema","schema":{}}}`, channeltype.GeminiOpenAICompatible, true, false},
		{"alias", "", "alias", `{"Format":{"type":"json_schema","schema":{}}}`, channeltype.GeminiOpenAICompatible, true, false},
		{"null-format", "gemini-4-argon", "alias", `{"format":null}`, channeltype.GeminiOpenAICompatible, true, false},
		{"effort-only", "gemini-4-hydrogen", "alias", `{"effort":"high"}`, channeltype.GeminiOpenAICompatible, false, false},
		{"null-config", "gemini-4-hydrogen", "alias", `null`, channeltype.GeminiOpenAICompatible, false, false},
		{"malformed", "gemini-4-hydrogen", "alias", `[]`, channeltype.GeminiOpenAICompatible, false, true},
		{"authoritative-legacy", "gemini-2.5-pro", "gemini-4-carbon", `{"format":{}}`, channeltype.GeminiOpenAICompatible, false, false},
		{"unknown", "gemini-4-unknown", "alias", `{"format":{}}`, channeltype.GeminiOpenAICompatible, false, false},
		{"unrelated", "gemini-4-hydrogen", "alias", `{"format":{}}`, channeltype.OpenAICompatible, false, false},
		{"deepseek", "deepseek-v4-pro", "alias", `{"format":{}}`, channeltype.DeepSeek, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &meta.Meta{ChannelType: tc.channel, ActualModelName: tc.actual, ModelMapping: map[string]string{"alias": "gemini-4-hydrogen"}}
			before := *info
			request := &model.ClaudeRequest{Model: tc.caller, OutputConfig: json.RawMessage(tc.raw)}
			got, err := hasExplicitGemini4OutputFormat(info, request)
			if tc.wantErr {
				require.ErrorContains(t, err, "validation failed")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, got)
			require.Equal(t, before, *info)
			require.Equal(t, tc.raw, string(request.OutputConfig))
		})
	}
	got, err := hasExplicitGemini4OutputFormat(nil, &model.ClaudeRequest{})
	require.NoError(t, err)
	require.False(t, got)
	got, err = hasExplicitGemini4OutputFormat(&meta.Meta{}, nil)
	require.NoError(t, err)
	require.False(t, got)
}

// TestGemini4SharedClaudeControls preserves controls on the canonical DTO used by Claude MCP planning.
func TestGemini4SharedClaudeControls(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	meta.Set2Context(ctx, &meta.Meta{ChannelType: channeltype.GeminiOpenAICompatible, ModelMapping: map[string]string{"alias": "gemini-4-hydrogen"}})
	raw := json.RawMessage(`{"effort":"xhigh","format":{"type":"json_schema","schema":{"type":"object"}}}`)
	request := &model.ClaudeRequest{Model: "alias", OutputConfig: raw, Messages: []model.ClaudeMessage{{Role: "user", Content: "hello"}}}
	converted, err := ConvertClaudeRequest(ctx, request)
	require.NoError(t, err)
	canonical, ok := converted.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.JSONEq(t, string(raw), string(canonical.OutputConfig))
	canonical.OutputConfig[0] = ' '
	require.Equal(t, byte('{'), request.OutputConfig[0], "canonical raw control bytes must be independently owned")
}
