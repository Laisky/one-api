package controller

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestSystematicTypedExtraBodyWire covers server-added defaults, caller
// precedence and denied transport fields. Parameters: t controls the tests.
// Returns: none; assertions inspect the final JSON rather than mutable DTOs.
func TestSystematicTypedExtraBodyWire(t *testing.T) {
	for _, tc := range []struct {
		name, original, updated, want string
	}{
		{"typed_default", `{"model":"custom"}`, `{"model":"custom","extra_body":{"enable_thinking":true}}`, `{"model":"custom","enable_thinking":true}`},
		{"caller_extra_wins", `{"extra_body":{"enable_thinking":false}}`, `{"extra_body":{"enable_thinking":true,"top_k":10}}`, `{"enable_thinking":false,"top_k":10}`},
		{"root_wins", `{"enable_thinking":false}`, `{"enable_thinking":false,"extra_body":{"enable_thinking":true}}`, `{"enable_thinking":false}`},
		{"deny_route_and_price_override", `{}`, `{"model":"custom","extra_body":{"model":"other","quota":0,"enable_thinking":false}}`, `{"model":"custom","enable_thinking":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _, _, err := mergeControlledPassthroughJSON([]byte(tc.original), []byte(tc.updated), false)
			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(got))
			again, _, changed, err := mergeControlledPassthroughJSON(got, got, false)
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, got, again)
		})
	}
}

// TestSystematicResponseThinkingWire verifies query values survive raw JSON
// forwarding, with mapped-model selection and explicit body precedence.
// Parameters: t controls cases. Returns: none; no inference request is made.
func TestSystematicResponseThinkingWire(t *testing.T) {
	for _, tc := range []struct {
		name, model, raw, query, expected string
		channel                           int
	}{
		{"grok_extended", "grok-4.7", `{"model":"alias","input":"test","opaque":9007199254740993}`, "thinking=true&reasoning_effort=xhigh", `{"effort":"xhigh"}`, channeltype.XAI},
		{"body_wins", "grok-4.7", `{"model":"alias","input":"test","reasoning":{"effort":"low"}}`, "thinking=true&reasoning_effort=xhigh", `{"effort":"low"}`, channeltype.XAI},
		{"qwen_disable", "Qwen/Qwen3.5-35B-A3B", `{"model":"alias","input":"test"}`, "thinking=false", `{"enable_thinking":false}`, channeltype.OpenAICompatible},
		{"qwen_enable", "Qwen/Qwen3.5-35B-A3B", `{"model":"alias","input":"test"}`, "thinking=true", `{"enable_thinking":true}`, channeltype.OpenAICompatible},
		{"qwen_body_wins", "Qwen/Qwen3.5-35B-A3B", `{"model":"alias","input":"test","extra_body":{"chat_template_kwargs":{"enable_thinking":false}}}`, "thinking=true", `{"enable_thinking":false}`, channeltype.OpenAICompatible},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/responses?"+tc.query, strings.NewReader(tc.raw))
			ctx.Request.Header.Set("Content-Type", "application/json")
			meta := &metalib.Meta{ActualModelName: tc.model, ChannelType: tc.channel, APIType: channeltype.ToAPIType(tc.channel)}
			var req openai.ResponseAPIRequest
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &req))
			applyThinkingQueryToResponseRequest(ctx, &req, meta)
			req.Model = tc.model
			body, _, _, err := normalizeResponseAPIRawBody([]byte(tc.raw), &req, tc.channel)
			require.NoError(t, err)
			var root map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body, &root))
			key := "reasoning"
			if strings.HasPrefix(tc.name, "qwen_") {
				key = "chat_template_kwargs"
			}
			require.JSONEq(t, tc.expected, string(root[key]))
			require.NotContains(t, root, "extra_body")
			if tc.name == "grok_extended" {
				require.Equal(t, "9007199254740993", string(root["opaque"]))
			}
			again, _, changed, err := normalizeResponseAPIRawBody(body, &req, tc.channel)
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, body, again)
		})
	}
}

// TestSystematicQueryDoesNotRestrictBody verifies that query defaults neither
// impose a foreign provider's vocabulary nor overwrite explicit user fields.
// Parameters: t controls cases. Returns: none; the request stays in memory.
func TestSystematicQueryDoesNotRestrictBody(t *testing.T) {
	for _, name := range []string{"tenant-experimental", "grok-future"} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest("POST", "/?thinking=true&reasoning_effort=low", nil)
		meta := &metalib.Meta{ActualModelName: name, ChannelType: channeltype.OpenAICompatible, APIType: apitype.OpenAI}
		effort := "provider-specific-effort"
		req := &relaymodel.GeneralOpenAIRequest{Model: name, ReasoningEffort: &effort}
		applyThinkingQueryToChatRequest(ctx, req, meta)
		require.Equal(t, "provider-specific-effort", *req.ReasoningEffort)
	}
}

// TestSystematicNullResponseBodyDoesNotPanic covers malformed object shape.
// Parameters: t controls assertions. Returns: none; invalid input is rejected.
func TestSystematicNullResponseBodyDoesNotPanic(t *testing.T) {
	require.NotPanics(t, func() {
		_, _, _, err := normalizeResponseAPIRawBody([]byte("null"), &openai.ResponseAPIRequest{Model: "test"}, channeltype.OpenAICompatible)
		require.Error(t, err)
	})
}
