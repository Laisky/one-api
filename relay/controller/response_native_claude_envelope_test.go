package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestNormalizeResponseAPIRawBodyStripsGatewayClaudeEnvelopes verifies that a
// session moving from a Claude fallback channel to a native Responses channel
// never forwards gateway Claude thinking envelopes upstream, where they are
// not valid encrypted reasoning, while provider-issued reasoning is preserved.
func TestNormalizeResponseAPIRawBodyStripsGatewayClaudeEnvelopes(t *testing.T) {
	t.Parallel()
	envelope, err := relaymodel.EncodeClaudeThinkingEnvelope(relaymodel.ClaudeThinkingBlock{Type: "thinking", Signature: "sig1"})
	require.NoError(t, err)
	raw := []byte(`{"model":"gpt-5.6-sol","input":[` +
		`{"type":"message","role":"user","content":"hi"},` +
		`{"type":"reasoning","id":"rs_claude","summary":[],"encrypted_content":"` + envelope + `"},` +
		`{"type":"reasoning","id":"rs_openai","summary":[],"encrypted_content":"gAAAAAopaque"},` +
		`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]},` +
		`{"type":"message","role":"user","content":"again"}]}`)
	var request openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal(raw, &request))

	patched, _, changed, err := normalizeResponseAPIRawBody(raw, &request, channeltype.OpenAI)
	require.NoError(t, err)
	require.True(t, changed)
	require.NotContains(t, string(patched), relaymodel.ClaudeThinkingEnvelopePrefix, "gateway envelopes must not reach a native provider")
	require.Contains(t, string(patched), "gAAAAAopaque", "provider-issued encrypted reasoning is preserved")

	var root struct {
		Input []map[string]any `json:"input"`
	}
	require.NoError(t, json.Unmarshal(patched, &root))
	require.Len(t, root.Input, 4)
	for _, item := range root.Input {
		require.NotEqual(t, "rs_claude", item["id"])
	}
}
