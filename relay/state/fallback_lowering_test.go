package state

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFallbackLowering pins the Section 5.8 fallback-column decisions: portable
// items carry, reasoning/thinking degrade to a display-only sidecar (drop), and
// hosted/built-in tool-call state plus unknown future item types fail closed.
func TestFallbackLowering(t *testing.T) {
	cases := []struct {
		name       string
		raw        string
		wantAction string
	}{
		{"message", `{"type":"message","role":"user","content":"hi"}`, FallbackActionCarry},
		{"function_call", `{"type":"function_call","call_id":"c1","name":"f","arguments":"{}"}`, FallbackActionCarry},
		{"function_call_output", `{"type":"function_call_output","call_id":"c1","output":"ok"}`, FallbackActionCarry},
		{"item_reference", `{"type":"item_reference","id":"item_1"}`, FallbackActionCarry},
		{"reasoning_encrypted", `{"type":"reasoning","encrypted_content":"opaque"}`, FallbackActionDrop},
		{"reasoning_summary", `{"type":"reasoning","summary":[]}`, FallbackActionDrop},
		{"thinking_signed", `{"type":"thinking","signature":"sig","thinking":"x"}`, FallbackActionDrop},
		{"web_search_call", `{"type":"web_search_call","id":"ws_1","status":"completed"}`, FallbackActionFail},
		{"code_interpreter_call", `{"type":"code_interpreter_call","id":"ci_1"}`, FallbackActionFail},
		{"computer_call", `{"type":"computer_call","id":"cc_1"}`, FallbackActionFail},
		{"mcp_call", `{"type":"mcp_call","id":"m_1"}`, FallbackActionFail},
		{"unknown_future_type", `{"type":"some_future_item","id":"x"}`, FallbackActionFail},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			action, _ := FallbackLowering(json.RawMessage(tc.raw))
			require.Equal(t, tc.wantAction, action)
		})
	}
}

// TestFallbackLoweringNonObject verifies a bare string (user message) carries.
func TestFallbackLoweringNonObject(t *testing.T) {
	action, kind := FallbackLowering(json.RawMessage(`"just text"`))
	require.Equal(t, FallbackActionCarry, action)
	require.Equal(t, "", kind)
}
