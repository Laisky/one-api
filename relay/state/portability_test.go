package state

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClassifyRawItem verifies the portability classification of each item type
// against the Section 5.8 table.
func TestClassifyRawItem(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		want PortabilityClass
	}{
		{"user message", `{"type":"message","role":"user","content":"hi"}`, PortabilityPortable},
		{"assistant message", `{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}`, PortabilityPortable},
		{"function call", `{"type":"function_call","call_id":"call_1","name":"f","arguments":"{}"}`, PortabilityPortable},
		{"function call output", `{"type":"function_call_output","call_id":"call_1","output":"ok"}`, PortabilityPortable},
		{"item reference", `{"type":"item_reference","id":"msg_1"}`, PortabilityPortable},
		{"encrypted reasoning", `{"type":"reasoning","encrypted_content":"opaque","summary":[]}`, PortabilityProviderBound},
		{"summary-only reasoning", `{"type":"reasoning","summary":[{"type":"summary_text","text":"s"}]}`, PortabilityDisplayOnly},
		{"signed thinking", `{"type":"thinking","signature":"sig","thinking":"..."}`, PortabilityProviderBound},
		{"unsigned thinking", `{"type":"thinking","thinking":"..."}`, PortabilityDisplayOnly},
		{"hosted web search", `{"type":"web_search_call","status":"completed"}`, PortabilityProviderBound},
		{"mcp call", `{"type":"mcp_call","server_label":"x"}`, PortabilityProviderBound},
		{"unknown future type", `{"type":"quantum_call","foo":"bar"}`, PortabilityProviderBound},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var probe struct {
				Type string `json:"type"`
			}
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &probe))
			got := classifyRawItem(probe.Type, json.RawMessage(tc.raw))
			require.Equal(t, tc.want, got)
		})
	}
}

// TestNewItemEnvelopePreservesRawAndIndexes verifies that building an envelope
// retains the raw payload byte-for-byte and derives the index fields (I01).
func TestNewItemEnvelopePreservesRawAndIndexes(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"type":"reasoning","id":"rs_1","encrypted_content":"opaque","phase":"analysis","summary":[],"unknown_future":{"a":1}}`)
	env, err := NewItemEnvelope(raw, "openai")
	require.NoError(t, err)

	require.Equal(t, KindReasoning, env.Kind)
	require.Equal(t, "rs_1", env.UpstreamItemID)
	require.Equal(t, "analysis", env.Phase)
	require.Equal(t, PortabilityProviderBound, env.Portability)
	require.NotEmpty(t, env.GatewayItemID)
	require.True(t, LooksLikeGatewayItemID(env.GatewayItemID))

	// Raw is retained byte-for-byte, including the unknown field.
	require.JSONEq(t, string(raw), string(env.Raw))
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(env.Raw, &decoded))
	require.Contains(t, decoded, "unknown_future")
}

// TestNewStringInputEnvelope verifies a bare string input becomes a portable user
// message envelope.
func TestNewStringInputEnvelope(t *testing.T) {
	t.Parallel()

	env, err := NewStringInputEnvelope("hello")
	require.NoError(t, err)
	require.Equal(t, KindMessage, env.Kind)
	require.Equal(t, "user", env.Role)
	require.Equal(t, PortabilityPortable, env.Portability)
	require.JSONEq(t, `{"type":"message","role":"user","content":"hello"}`, string(env.Raw))
}
