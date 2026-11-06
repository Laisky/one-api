package state

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestFallbackLoweringForClaudeCarriesGatewayEnvelopes verifies only a valid
// gateway Claude envelope is carried onto a Claude target; foreign or malformed
// encrypted reasoning, plaintext reasoning and the Chat table keep their rows.
func TestFallbackLoweringForClaudeCarriesGatewayEnvelopes(t *testing.T) {
	t.Parallel()
	envelope, err := model.EncodeClaudeThinkingEnvelope(model.ClaudeThinkingBlock{Type: "thinking", Signature: "sig1"})
	require.NoError(t, err)
	item := func(encrypted string) json.RawMessage {
		raw, err := json.Marshal(map[string]any{"type": "reasoning", "id": "rs_1", "summary": []any{}, "encrypted_content": encrypted})
		require.NoError(t, err)
		return raw
	}

	action, kind := FallbackLoweringForClaude(item(envelope))
	require.Equal(t, FallbackActionCarry, action)
	require.Equal(t, KindReasoning, kind)
	// The item remains provider-bound; only its Claude-target lowering changed.
	require.Equal(t, PortabilityProviderBound, classifyRawItem(kind, item(envelope)))

	chatAction, _ := FallbackLowering(item(envelope))
	require.Equal(t, FallbackActionDrop, chatAction, "non-Claude targets still drop gateway envelopes")

	for _, foreign := range []string{"opaque-openai", model.ClaudeThinkingEnvelopePrefix + "%%%", ""} {
		action, _ := FallbackLoweringForClaude(item(foreign))
		require.Equal(t, FallbackActionDrop, action)
	}
	action, _ = FallbackLoweringForClaude(json.RawMessage(`{"type":"thinking","thinking":"t","signature":"s"}`))
	require.Equal(t, FallbackActionDrop, action)
	action, _ = FallbackLoweringForClaude(json.RawMessage(`{"type":"web_search_call","id":"ws"}`))
	require.Equal(t, FallbackActionFail, action)
}
