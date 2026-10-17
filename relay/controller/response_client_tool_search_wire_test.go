package controller

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/stretchr/testify/require"
)

// TestResponseClientToolSearchWirePreservesDiscovery verifies native and typed
// fallback encoding retain client execution, object arguments and loaded tools.
func TestResponseClientToolSearchWirePreservesDiscovery(t *testing.T) {
	for _, input := range []string{
		`"hello"`,
		`[{"type":"tool_search_call","execution":"client","call_id":"call_search","arguments":{"query":"lookup","limit":3,"exact":9007199254740993},"status":"completed"},{"type":"tool_search_output","execution":"client","call_id":"call_search","status":"completed","tools":[{"type":"namespace","name":"crm","tools":[{"type":"function","name":"lookup","defer_loading":true,"parameters":{"type":"object"}},{"type":"custom","name":"patch","format":{"type":"grammar","syntax":"lark","definition":"start: WORD"}}]}]}]`,
		`[{"type":"additional_tools","role":"developer","tools":[{"type":"namespace","name":"crm","tools":[{"type":"function","name":"lookup"}]},{"type":"tool_search","execution":"client","parameters":{"type":"object"}}]},{"role":"user","content":"hello"}]`,
	} {
		raw := []byte(`{"model":"fixture","input":` + input + `,"tools":[{"type":"tool_search","execution":"client","description":"Local BM25 lookup","parameters":{"type":"object","properties":{"query":{"type":"string"}},"required":["query"],"additionalProperties":false}}]}`)
		var request openai.ResponseAPIRequest
		require.NoError(t, json.Unmarshal(raw, &request))
		for _, body := range [][]byte{raw, nil} {
			wire, _, _, err := normalizeResponseAPIRawBody(body, &request, channeltype.OpenAI)
			require.NoError(t, err)
			var before, after map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(raw, &before))
			require.NoError(t, json.Unmarshal(wire, &after))
			require.JSONEq(t, string(before["tools"]), string(after["tools"]))
			require.JSONEq(t, string(before["input"]), string(after["input"]))
			if bytes.Contains(before["input"], []byte("9007199254740993")) {
				require.Contains(t, string(after["input"]), "9007199254740993")
			}
		}
	}
}

// TestResponseClientToolSearchRejectsHostedDefinitions keeps server discovery
// and dynamically introduced paid tools behind reviewed billing paths.
func TestResponseClientToolSearchRejectsHostedDefinitions(t *testing.T) {
	for _, tool := range []string{
		`{"type":"tool_search"}`, `{"type":"tool_search","execution":"server"}`,
		`{"type":"tool_search","execution":"CLIENT"}`, `{"type":"tool_search","execution":" client "}`,
	} {
		var request openai.ResponseAPIRequest
		raw := []byte(`{"model":"fixture","input":"hello","tools":[` + tool + `]}`)
		require.NoError(t, json.Unmarshal(raw, &request))
		for _, body := range [][]byte{raw, nil} {
			wire, _, _, err := normalizeResponseAPIRawBody(body, &request, channeltype.OpenAI)
			require.Error(t, err, tool)
			require.Empty(t, wire)
		}
	}
	for _, kind := range []string{"tool_search_output", "additional_tools"} {
		for _, tool := range []string{
			`{"type":"web_search"}`, `{"type":"code_interpreter"}`, `{"type":"file_search"}`, `{"type":"image_generation"}`,
			`{"type":"mcp","server_label":"external"}`, `{"type":"future_paid_tool"}`,
			`{"type":"namespace","name":"local","tools":[{"type":"web_search"}]}`,
		} {
			raw := []byte(`{"model":"fixture","input":[{"type":"` + kind + `","role":"developer","execution":"client","call_id":"call_search","status":"completed","tools":[` + tool + `]}]}`)
			var request openai.ResponseAPIRequest
			require.NoError(t, json.Unmarshal(raw, &request))
			for _, body := range [][]byte{raw, nil} {
				wire, _, _, err := normalizeResponseAPIRawBody(body, &request, channeltype.OpenAI)
				require.Error(t, err, string(raw))
				require.Empty(t, wire)
			}
		}
	}
}

// TestResponseClientToolSearchRejectsAmbiguousHistory checks raw discriminators
// before map decoding can hide a hosted execution or tools array.
func TestResponseClientToolSearchRejectsAmbiguousHistory(t *testing.T) {
	for _, item := range []string{
		`{"type":"tool_search_output","Type":"message","execution":"client","call_id":"c","tools":[{"type":"web_search"}]}`,
		`{"type":"tool_search_output","execution":"server","Execution":"client","call_id":"c","tools":[]}`,
		`{"type":"tool_search_output","execution":"client","call_id":"c","tools":[{"type":"web_search"}],"tools":[]}`,
		`{"type":"tool_search_output","execution":"client","call_id":"c","tools":[{"type":"web_search","Type":"function","name":"f"}]}`,
		`{"type":"additional_tools","role":"developer","tools":[{"type":"web_search"}],"tool\u017f":[]}`,
	} {
		var request openai.ResponseAPIRequest
		require.Error(t, json.Unmarshal([]byte(`{"model":"fixture","input":[`+item+`]}`), &request), item)
	}
}

// TestResponseClientToolSearchCatalogBounds covers empty lookup results and the
// reviewed per-catalog bound without any provider or quota effects.
func TestResponseClientToolSearchCatalogBounds(t *testing.T) {
	for _, count := range []int{0, openai.MaxResponseNamespaceTools, openai.MaxResponseNamespaceTools + 1} {
		definitions := make([]string, count)
		for i := range definitions {
			definitions[i] = `{"type":"function","name":"lookup"}`
		}
		raw := []byte(`{"model":"fixture","input":[{"type":"tool_search_output","execution":"client","call_id":"search","tools":[` + strings.Join(definitions, ",") + `]}]}`)
		var request openai.ResponseAPIRequest
		err := json.Unmarshal(raw, &request)
		if count > openai.MaxResponseNamespaceTools {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		wire, _, _, err := normalizeResponseAPIRawBody(raw, &request, channeltype.OpenAI)
		require.NoError(t, err)
		require.NotEmpty(t, wire)
	}
}

// TestResponseClientToolSearchInvalidHistory rejects missing identity, hosted
// execution and malformed catalogs rather than forwarding incomplete semantics.
func TestResponseClientToolSearchInvalidHistory(t *testing.T) {
	for _, item := range []string{
		`{"type":"tool_search_call","execution":"server","call_id":"c","arguments":{}}`,
		`{"type":"tool_search_call","execution":"client","arguments":{}}`,
		`{"type":"tool_search_call","execution":"client","call_id":"c","arguments":"query"}`,
		`{"type":"tool_search_output","execution":"client","call_id":"c"}`,
		`{"type":"tool_search_output","execution":"client","call_id":"c","tools":null}`,
		`{"type":"tool_search_output","execution":"server","call_id":"c","tools":[]}`,
		`{"type":"additional_tools","role":"user","tools":[]}`,
		`{"type":"additional_tools","role":"developer","tools":[{"type":"tool_search"}]}`,
		`{"type":"tool_search_output","execution":"client","call_id":"c","tools":[{"type":"tool_search","execution":"client"}]}`,
	} {
		raw := []byte(`{"model":"fixture","input":[` + item + `]}`)
		var request openai.ResponseAPIRequest
		require.NoError(t, json.Unmarshal(raw, &request))
		wire, _, _, err := normalizeResponseAPIRawBody(raw, &request, channeltype.OpenAI)
		require.Error(t, err, item)
		require.Empty(t, wire)
	}
}

// TestResponseClientToolSearchRawHistoryCannotBypassTypedPolicy checks the wire
// boundary independently when the typed request was bound from another source.
func TestResponseClientToolSearchRawHistoryCannotBypassTypedPolicy(t *testing.T) {
	var request openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"fixture","input":"hello"}`), &request))
	for _, input := range []string{
		`[{"type":"tool_search_output","execution":"client","call_id":"c","tools":[{"type":"image_generation"}]}]`,
		`[{"type":"tool_search_output","execution":"server","Execution":"client","call_id":"c","tools":[]}]`,
	} {
		wire, _, _, err := normalizeResponseAPIRawBody([]byte(`{"model":"fixture","input":`+input+`}`), &request, channeltype.OpenAI)
		require.Error(t, err)
		require.Empty(t, wire)
	}
}
