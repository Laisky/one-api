package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConvertResponseAPIPairsCallsByCallID verifies that a tool call and its
// output pair through call_id even when neither the item id nor the call_id
// carries the fc_/call_ prefixes, for function, custom and tool_search calls.
func TestConvertResponseAPIPairsCallsByCallID(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"function":    `[{"type":"function_call","id":"item_a","call_id":"tool_a","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"tool_a","output":"ok"}]`,
		"custom":      `[{"type":"custom_tool_call","id":"ctc_a","call_id":"tool_a","name":"apply_patch","input":"x"},{"type":"custom_tool_call_output","id":"ctco_a","call_id":"tool_a","output":"ok"}]`,
		"tool_search": `[{"type":"tool_search_call","id":"search_item","call_id":"search_1","execution":"client","arguments":{"query":"x"}},{"type":"tool_search_output","call_id":"search_1","execution":"client","tools":[]}]`,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var request ResponseAPIRequest
			require.NoError(t, json.Unmarshal([]byte(`{"model":"m","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}},{"type":"custom","name":"apply_patch"},{"type":"tool_search","execution":"client"}],"input":`+input+`}`), &request))
			converted, err := ConvertResponseAPIToChatCompletionRequest(&request)
			require.NoError(t, err)
			require.Len(t, converted.Messages, 2)
			require.Equal(t, "assistant", converted.Messages[0].Role)
			require.Len(t, converted.Messages[0].ToolCalls, 1)
			require.Equal(t, "tool", converted.Messages[1].Role, "the output must answer its call instead of becoming an orphan user message")
			require.Equal(t, converted.Messages[0].ToolCalls[0].Id, converted.Messages[1].ToolCallId)
		})
	}
}

// TestOutputItemCustomToolCallAlwaysCarriesInput verifies that a custom tool
// call with empty input still serializes the required input field exactly once.
func TestOutputItemCustomToolCallAlwaysCarriesInput(t *testing.T) {
	t.Parallel()
	data, err := json.Marshal(OutputItem{Type: "custom_tool_call", Id: "call_1", CallId: "call_1", Name: "apply_patch", Status: "completed"})
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))
	input, ok := decoded["input"]
	require.True(t, ok, "custom_tool_call must always carry input")
	require.Equal(t, "", input)
	require.Equal(t, 1, countJSONKey(string(data), `"input"`))

	data, err = json.Marshal(OutputItem{Type: "custom_tool_call", CallId: "call_2", Name: "apply_patch", Input: "*** Begin Patch\n"})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, "*** Begin Patch\n", decoded["input"])
	require.Equal(t, 1, countJSONKey(string(data), `"input"`))
}

// countJSONKey counts occurrences of key in the serialized JSON text.
func countJSONKey(data, key string) int {
	count := 0
	for i := 0; i+len(key) <= len(data); i++ {
		if data[i:i+len(key)] == key {
			count++
		}
	}
	return count
}
