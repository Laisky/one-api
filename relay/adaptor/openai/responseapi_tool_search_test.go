package openai

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClientToolSearchDefinitionRoundTrip pins the client execution discriminator
// emitted by Codex rust-v0.163.0 tool_search_spec.rs. Parameters: t is the test
// handle. Returns: nothing; assertions report lost wire fields.
func TestClientToolSearchDefinitionRoundTrip(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"type":"tool_search","execution":"client","description":"Find deferred tools","parameters":{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"number"}},"required":["query"],"additionalProperties":false}}`)
	var tool ResponseAPITool
	require.NoError(t, json.Unmarshal(raw, &tool))
	wire, err := json.Marshal(tool)
	require.NoError(t, err)
	require.JSONEq(t, string(raw), string(wire))
}

// TestClientToolSearchExecutionRejectsAmbiguity protects the client/server
// discriminator at the DTO boundary. Parameters: t is the test handle. Returns:
// nothing; ambiguous or malformed execution fields must fail decoding.
func TestClientToolSearchExecutionRejectsAmbiguity(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`{"type":"tool_search","execution":"server","Execution":"client"}`,
		`{"type":"tool_search","execution":"server","execution":"client"}`,
		`{"type":"tool_search","execution":"server","execu\u0074ion":"client"}`,
		`{"type":"tool_search","execution":true}`,
	} {
		var tool ResponseAPITool
		require.Error(t, json.Unmarshal([]byte(raw), &tool), raw)
	}
}

// TestClientToolSearchOutputRoundTrip preserves object arguments and discovered
// namespaces per the official tool-search guide and pinned Codex protocol models.
// Parameters: t is the test handle. Returns: nothing; lost protocol data fails.
func TestClientToolSearchOutputRoundTrip(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`{"type":"tool_search_call","call_id":"search_1","execution":"client","status":"completed","arguments":{"query":"calendar","limit":2,"large":9007199254740993}}`,
		`{"type":"tool_search_call","call_id":"search_1","execution":"client","status":"in_progress","arguments":null}`,
		`{"type":"tool_search_output","call_id":"search_1","execution":"client","status":"completed","tools":[]}`,
		`{"type":"tool_search_output","call_id":"search_1","execution":"client","status":"completed","tools":[{"type":"namespace","name":"calendar","tools":[{"type":"function","name":"lookup","strict":false,"defer_loading":true,"parameters":{"type":"object"}},{"type":"custom","name":"patch","format":{"type":"grammar","syntax":"lark","definition":"start: WORD"}}]}]}`,
	} {
		var item OutputItem
		require.NoError(t, json.Unmarshal([]byte(raw), &item), raw)
		wire, err := json.Marshal(item)
		require.NoError(t, err)
		var before, after map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(raw), &before))
		require.NoError(t, json.Unmarshal(wire, &after))
		require.JSONEq(t, string(before["execution"]), string(after["execution"]))
		require.Equal(t, before["call_id"], after["call_id"])
		if arguments, ok := before["arguments"]; ok {
			require.Equal(t, string(arguments), string(after["arguments"]))
		}
		if _, ok := before["tools"]; ok {
			var again OutputItem
			require.NoError(t, json.Unmarshal(wire, &again))
			wireAgain, err := json.Marshal(again)
			require.NoError(t, err)
			require.JSONEq(t, string(wire), string(wireAgain))
			if len(before["tools"]) > 2 {
				require.Contains(t, string(after["tools"]), `"namespace"`)
				require.Contains(t, string(after["tools"]), `"defer_loading":true`)
				require.Contains(t, string(after["tools"]), `"strict":false`)
				require.Contains(t, string(after["tools"]), `"definition":"start: WORD"`)
			} else {
				require.JSONEq(t, `[]`, string(after["tools"]))
			}
		}
	}
}

// TestClientToolSearchStreamTerminalReceipt verifies object-valued search calls
// do not hide a valid terminal usage receipt. Parameters: t is the test handle.
// Returns: nothing; parsing and terminal-receipt assertions report regressions.
func TestClientToolSearchStreamTerminalReceipt(t *testing.T) {
	t.Parallel()
	const call = `{"type":"tool_search_call","call_id":"search_1","execution":"client","arguments":{"query":"calendar","limit":1}}`
	for _, raw := range []string{
		`{"type":"response.output_item.added","item":` + call + `}`,
		`{"type":"response.output_item.done","item":` + call + `}`,
		`{"type":"response.completed","response":{"id":"resp_search","status":"completed","output":[` + call + `],"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}}}`,
	} {
		_, event, err := ParseResponseAPIStreamEvent([]byte(raw))
		require.NoError(t, err)
		require.NotNil(t, event)
		wire, err := json.Marshal(event)
		require.NoError(t, err)
		require.Contains(t, string(wire), `"execution":"client"`)
		require.Contains(t, string(wire), `"arguments":{"query":"calendar","limit":1}`)
		if event.Response != nil {
			converted := ConvertStreamEventToResponse(event)
			require.True(t, responseStreamHasTerminalUsage(nil, event, &converted))
			require.Equal(t, 18, event.Response.Usage.TotalTokens)
		}
	}
}

// TestClientToolSearchPreservesFunctionAndMCPContracts prevents structured search
// arguments from widening ordinary function/MCP decoding. Parameters: t is the
// test handle. Returns: nothing; legacy wire shapes must remain unchanged.
func TestClientToolSearchPreservesFunctionAndMCPContracts(t *testing.T) {
	t.Parallel()
	var invalid OutputItem
	require.Error(t, json.Unmarshal([]byte(`{"type":"function_call","arguments":{"query":"invalid"}}`), &invalid))
	for _, raw := range []string{
		`{"type":"function_call","name":"lookup","call_id":"call_1","arguments":"{\"query\":\"calendar\"}"}`,
		`{"type":"mcp_list_tools","server_label":"calendar","tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]}`,
		`{"type":"message","role":"assistant","content":[]}`,
	} {
		var item OutputItem
		require.NoError(t, json.Unmarshal([]byte(raw), &item))
		wire, err := json.Marshal(item)
		require.NoError(t, err)
		require.JSONEq(t, raw, string(wire))
	}
}

// TestClientToolSearchCatalogStateFidelity preserves original loaded definitions
// through state cycles without letting retained raw bytes override typed edits.
// Parameters: t is the test handle. Returns: nothing; wire mismatches fail.
func TestClientToolSearchCatalogStateFidelity(t *testing.T) {
	t.Parallel()
	const catalog = `[{"type":"function","name":"lookup","vendor_scope":{"fixture":"opaque"},"parameters":{"type":"object","properties":{"count":{"const":9007199254740993}}}}]`
	raw := []byte(`{"type":"tool_search_output","execution":"client","call_id":"search_1","status":"completed","tools":` + catalog + `}`)
	var item OutputItem
	for cycle := 0; cycle < 2; cycle++ {
		require.NoError(t, json.Unmarshal(raw, &item))
		wire, err := json.Marshal(item)
		require.NoError(t, err)
		var envelope map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(wire, &envelope))
		require.Contains(t, string(envelope["tools"]), "9007199254740993", "state must preserve the exact integer token")
		require.JSONEq(t, catalog, string(envelope["tools"]), "state cycle %d must preserve unknown fields and exact literals", cycle)
		raw = wire
	}
	// Mutating a decoded field must defeat its retained raw snapshot.
	require.NotNil(t, item.ToolSearchTools[0].Function)
	item.ToolSearchTools[0].Function.Name = "renamed"
	wire, err := json.Marshal(item)
	require.NoError(t, err)
	var mutated struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(wire, &mutated))
	require.Equal(t, "renamed", mutated.Tools[0].Name)

	// Constructed typed catalogs have no raw snapshot and must still serialize.
	created := OutputItem{Type: "tool_search_output", Execution: "client", ToolSearchTools: []ResponseAPITool{{Type: "custom", Name: "patch", Format: json.RawMessage(`{"type":"text"}`)}}}
	wire, err = json.Marshal(created)
	require.NoError(t, err)
	require.Contains(t, string(wire), `"name":"patch"`)
	require.Contains(t, string(wire), `"format":{"type":"text"}`)
}

// TestClientToolSearchOutputCatalogBound checks the catalog bound before child
// DTO decoding. Parameters: t is the test handle. Returns: nothing; a too-large
// provider output must fail with the count error, not decode arbitrary children.
func TestClientToolSearchOutputCatalogBound(t *testing.T) {
	t.Parallel()
	for _, count := range []int{MaxResponseNamespaceTools, MaxResponseNamespaceTools + 1} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			definitions := make([]map[string]any, count)
			for i := range definitions {
				definitions[i] = map[string]any{"type": "function", "name": fmt.Sprintf("tool_%04d", i)}
			}
			if count > MaxResponseNamespaceTools {
				definitions[len(definitions)-1]["type"] = 123
			}
			raw, err := json.Marshal(map[string]any{"type": "tool_search_output", "execution": "client", "call_id": "search_1", "tools": definitions})
			require.NoError(t, err)
			var item OutputItem
			err = json.Unmarshal(raw, &item)
			if count > MaxResponseNamespaceTools {
				require.ErrorContains(t, err, "count exceeds limit")
			} else {
				require.NoError(t, err)
				require.Len(t, item.ToolSearchTools, count)
			}
		})
	}
}
