package aws

import (
	"bytes"
	"encoding/json"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

// TestClaude431AWSBetweenTools verifies valid small-output requests through the real SDK serializer.
func TestClaude431AWSBetweenTools(t *testing.T) {
	t.Parallel()
	for _, native := range []bool{false, true} {
		h := newClaudeInvokeHarness(t, false, []byte(claudeInvokeReceipt), "us-east-1")
		h.meta.ActualModelName = "claude-sonnet-5-5"
		if native {
			h.prepareNative(t, `{"model":"claude-sonnet-5-5","max_tokens":64,"thinking":{"type":"between_tools"},"output_config":{"effort":"low","id":9007199254740993},"messages":[{"role":"user","content":"Hello"}]}`)
		} else {
			request := &relaymodel.GeneralOpenAIRequest{Model: h.meta.ActualModelName, MaxTokens: 64, Thinking: &relaymodel.Thinking{Type: "between_tools"}, OutputConfig: json.RawMessage(`{"effort":"low","id":9007199254740993}`), Messages: []relaymodel.Message{{Role: "user", Content: "Hello"}}}
			converted, err := h.adaptor.ConvertRequest(h.context, relaymode.ChatCompletions, request)
			require.NoError(t, err)
			body, err := json.Marshal(converted)
			require.NoError(t, err)
			_, err = h.adaptor.DoRequest(h.context, h.meta, bytes.NewReader(body))
			require.NoError(t, err)
		}
		usage, failure := h.adaptor.DoResponse(h.context, nil, h.meta)
		require.Nil(t, failure)
		require.NotNil(t, usage)
		require.Len(t, h.transport.bodies, 1)
		require.Contains(t, h.transport.bodies[0], `"type":"between_tools"`)
		require.Contains(t, h.transport.bodies[0], `"max_tokens":64`)
		require.Contains(t, h.transport.bodies[0], `9007199254740993`)
		require.Equal(t, []string{"/model/global.anthropic.claude-sonnet-5-5/invoke"}, h.transport.paths)
		// Final dispatch refuses forced tool use before starting another paid operation.
		_, err := h.adaptor.DoRequest(h.context, h.meta, strings.NewReader(`{"model":"claude-sonnet-5-5","tool_choice":{"type":"any"},"max_tokens":64,"messages":[]}`))
		require.ErrorContains(t, err, "validation failed")
		require.Len(t, h.transport.bodies, 1)
	}
}

// TestClaude431AWSPartialStreamRetainsEstimate distinguishes an intermediate receipt from final usage.
func TestClaude431AWSPartialStreamRetainsEstimate(t *testing.T) {
	t.Parallel()
	h := newClaudeInvokeHarness(t, true, claudeEventFrames(t, claudeInvokeStart), "us-east-1")
	h.meta.ActualModelName = "claude-sonnet-5-5"
	h.prepareChat(t)
	usage, failure := h.adaptor.DoResponse(h.context, nil, h.meta)
	require.NotNil(t, failure)
	require.NotNil(t, usage)
	require.NotEmpty(t, usage.BillingEstimateReason)
	require.Equal(t, 21, usage.PromptTokens)
	require.Equal(t, 1, usage.CompletionTokens)
	require.NotContains(t, h.recorder.Body.String(), "[DONE]")
}
