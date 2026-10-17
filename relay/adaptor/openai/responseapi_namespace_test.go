package openai

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponseAPINamespaceToolRoundTrip preserves group identities and local tool definitions.
func TestResponseAPINamespaceToolRoundTrip(t *testing.T) {
	t.Parallel()
	const grammar = `{"type":"grammar","syntax":"lark","definition":"start: /[a-z]+/"}`
	const payload = `{"type":"namespace","name":"editor","description":"editing tools","tools":[{"type":"function","name":"run","parameters":{"type":"object","properties":{"text":{"type":"string"}}},"strict":false,"defer_loading":true},{"type":"custom","name":"patch","format":` + grammar + `}]} `
	var tool ResponseAPITool
	require.NoError(t, json.Unmarshal([]byte(payload), &tool))
	require.Equal(t, "namespace", tool.Type)
	require.Equal(t, "editor", tool.Name)
	require.Len(t, tool.Tools, 2)
	require.NotNil(t, tool.Tools[0].Strict)
	require.False(t, *tool.Tools[0].Strict)
	require.NotNil(t, tool.Tools[0].DeferLoading)
	require.True(t, *tool.Tools[0].DeferLoading)
	require.JSONEq(t, grammar, string(tool.Tools[1].Format))

	other := ResponseAPITool{Type: "namespace", Name: "shell", Tools: []ResponseAPITool{
		{Type: "function", Name: "run", Parameters: map[string]any{"type": "object"}},
	}}
	wire, err := json.Marshal([]ResponseAPITool{tool, other})
	require.NoError(t, err)
	var roundTrip []ResponseAPITool
	require.NoError(t, json.Unmarshal(wire, &roundTrip))
	require.Len(t, roundTrip, 2)
	for i, group := range []string{"editor", "shell"} {
		require.Equal(t, "namespace", roundTrip[i].Type)
		require.Equal(t, group, roundTrip[i].Name)
		require.Equal(t, "run", roundTrip[i].Tools[0].Name)
		require.NotContains(t, roundTrip[i].Tools[0].Name, ".")
	}
	require.Equal(t, tool.Tools[0].Parameters, roundTrip[0].Tools[0].Parameters)
	require.JSONEq(t, grammar, string(roundTrip[0].Tools[1].Format))
	require.NotNil(t, roundTrip[0].Tools[0].Strict)
	require.False(t, *roundTrip[0].Tools[0].Strict)
	require.NotNil(t, roundTrip[0].Tools[0].DeferLoading)
	require.True(t, *roundTrip[0].Tools[0].DeferLoading)
}

// TestResponseAPINamespaceToolCountBoundary bounds decoding before recursive tool construction.
func TestResponseAPINamespaceToolCountBoundary(t *testing.T) {
	t.Parallel()
	for _, count := range []int{MaxResponseNamespaceTools, MaxResponseNamespaceTools + 1} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			children := make([]map[string]any, count)
			for i := range children {
				children[i] = map[string]any{"type": "function", "name": fmt.Sprintf("tool_%04d", i)}
			}
			wire, err := json.Marshal(map[string]any{"type": "namespace", "name": "group", "tools": children})
			require.NoError(t, err)
			var tool ResponseAPITool
			err = json.Unmarshal(wire, &tool)
			if count > MaxResponseNamespaceTools {
				require.ErrorContains(t, err, "count")
			} else {
				require.NoError(t, err)
				require.Len(t, tool.Tools, count)
			}
		})
	}
}

// TestResponseAPINamespaceRejectsNestedToolShapes rejects recursion before decoding child DTOs.
func TestResponseAPINamespaceRejectsNestedToolShapes(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"nested namespace":          `{"type":"namespace","name":"outer","tools":[{"type":"namespace","name":"inner","tools":[]}]}`,
		"case normalized namespace": `{"type":"namespace","tools":[{"type":" Namespace ","name":"inner"}]}`,
		"function child tools":      `{"type":"namespace","tools":[{"type":"function","name":"run","tools":[]}]}`,
		"null child tools":          `{"type":"namespace","tools":[{"type":"function","name":"run","tools":null}]}`,
		"deep namespace":            strings.Repeat(`{"type":"namespace","name":"group","tools":[`, 128) + `{"type":"function","name":"leaf"}` + strings.Repeat("]}", 128),
	}
	for name, wire := range cases {
		t.Run(name, func(t *testing.T) {
			var tool ResponseAPITool
			require.ErrorContains(t, json.Unmarshal([]byte(wire), &tool), "nested")
		})
	}
}

// TestResponseAPINamespaceMalformedJSON rejects malformed types rather than coercing definitions.
func TestResponseAPINamespaceMalformedJSON(t *testing.T) {
	t.Parallel()
	for name, wire := range map[string]string{
		"tools object":      `{"type":"namespace","tools":{}}`,
		"tools string":      `{"type":"namespace","tools":"tool"}`,
		"child string":      `{"type":"namespace","tools":["tool"]}`,
		"child number":      `{"type":"namespace","tools":[1]}`,
		"child array":       `{"type":"namespace","tools":[[]]}`,
		"child type number": `{"type":"namespace","tools":[{"type":1}]}`,
		"strict string":     `{"type":"namespace","tools":[{"type":"function","name":"run","strict":"false"}]}`,
		"defer string":      `{"type":"namespace","tools":[{"type":"function","name":"run","defer_loading":"true"}]}`,
		"truncated":         `{"type":"namespace","tools":[`,
	} {
		t.Run(name, func(t *testing.T) {
			var tool ResponseAPITool
			require.Error(t, json.Unmarshal([]byte(wire), &tool))
		})
	}
}

// TestResponseAPINamespaceOutputJSONRoundTrip preserves separate call namespace and custom input.
func TestResponseAPINamespaceOutputJSONRoundTrip(t *testing.T) {
	t.Parallel()
	const customInput = "*** Begin Patch\n*** End Patch\n"
	response := ResponseAPIResponse{Id: "resp_namespace", Output: []OutputItem{
		{Type: "function_call", Id: "fc_editor", CallId: "call_editor", Name: "run", Namespace: "editor", Arguments: `{"command":"inspect"}`},
		{Type: "custom_tool_call", Id: "ct_shell", CallId: "call_shell", Name: "run", Namespace: "shell", Input: customInput},
	}}
	wire, err := json.Marshal(response)
	require.NoError(t, err)
	var decoded ResponseAPIResponse
	require.NoError(t, json.Unmarshal(wire, &decoded))
	require.Len(t, decoded.Output, 2)
	require.Equal(t, response.Output, decoded.Output)
	require.Equal(t, "run", decoded.Output[0].Name)
	require.Equal(t, "run", decoded.Output[1].Name)
	require.Equal(t, "editor", decoded.Output[0].Namespace)
	require.Equal(t, "shell", decoded.Output[1].Namespace)
	require.Equal(t, customInput, decoded.Output[1].Input)

	ordinary, err := json.Marshal(OutputItem{Type: "function_call", Name: "ordinary"})
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(ordinary, &fields))
	require.NotContains(t, fields, "namespace")
	require.NotContains(t, fields, "input")
}

// TestResponseAPINamespaceOutputStreamRoundTrip preserves typed item and completed event calls.
func TestResponseAPINamespaceOutputStreamRoundTrip(t *testing.T) {
	t.Parallel()
	const functionCall = `{"type":"function_call","id":"fc_editor","call_id":"call_editor","name":"run","namespace":"editor","arguments":"{}"}`
	const customCall = `{"type":"custom_tool_call","id":"ct_shell","call_id":"call_shell","name":"run","namespace":"shell","input":"patch\ntext"}`
	cases := []struct {
		name  string
		wire  string
		group string
		input string
	}{
		{"function added", `{"type":"response.output_item.added","output_index":0,"item":` + functionCall + "}", "editor", ""},
		{"custom done", `{"type":"response.output_item.done","output_index":1,"item":` + customCall + "}", "shell", "patch\ntext"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response, event, err := ParseResponseAPIStreamEvent([]byte(tc.wire))
			require.NoError(t, err)
			require.Nil(t, response)
			require.NotNil(t, event)
			require.NotNil(t, event.Item)
			require.Equal(t, "run", event.Item.Name)
			require.Equal(t, tc.group, event.Item.Namespace)
			require.Equal(t, tc.input, event.Item.Input)
			wire, err := json.Marshal(event)
			require.NoError(t, err)
			var roundTrip ResponseAPIStreamEvent
			require.NoError(t, json.Unmarshal(wire, &roundTrip))
			require.Equal(t, event.Item, roundTrip.Item)
		})
	}
	completed := `{"type":"response.completed","response":{"id":"resp_namespace","output":[` + functionCall + "," + customCall + "]}}"
	response, event, err := ParseResponseAPIStreamEvent([]byte(completed))
	require.NoError(t, err)
	require.Nil(t, response)
	require.NotNil(t, event)
	require.NotNil(t, event.Response)
	require.Len(t, event.Response.Output, 2)
	require.Equal(t, "editor", event.Response.Output[0].Namespace)
	require.Equal(t, "shell", event.Response.Output[1].Namespace)
	require.Equal(t, "run", event.Response.Output[0].Name)
	require.Equal(t, "run", event.Response.Output[1].Name)
	require.Equal(t, "patch\ntext", event.Response.Output[1].Input)
	wire, err := json.Marshal(event)
	require.NoError(t, err)
	var roundTrip ResponseAPIStreamEvent
	require.NoError(t, json.Unmarshal(wire, &roundTrip))
	require.Equal(t, event.Response.Output, roundTrip.Response.Output)
}
