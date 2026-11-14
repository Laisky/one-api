package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponseFallbackSchemaPolicyPreservesCatalogs verifies provider-selected preservation includes root, namespaced and discovered functions.
func TestResponseFallbackSchemaPolicyPreservesCatalogs(t *testing.T) {
	const schema = `{"type":"object","description":"schema description","additionalProperties":false,"properties":{"additionalProperties":{"type":"string","const":"note"}}}`
	raw := `{"model":"fixture","tools":[{"type":"function","name":"root","parameters":` + schema + `},{"type":"namespace","name":"group","tools":[{"type":"function","name":"nested","parameters":` + schema + `}]},{"type":"function","name":"parameterless"},{"type":"tool_search","execution":"client","parameters":` + schema + `}],"input":[{"type":"tool_search_output","call_id":"search","execution":"client","tools":[{"type":"function","name":"discovered","parameters":` + schema + `}]}],"text":{"format":{"type":"json_schema","name":"result","schema":` + schema + `}}}`
	var request ResponseAPIRequest
	require.NoError(t, json.Unmarshal([]byte(raw), &request))
	before, err := json.Marshal(request)
	require.NoError(t, err)
	for _, preserve := range []bool{false, true} {
		name := "legacy_sanitizer"
		if preserve {
			name = "vertex_fidelity"
		}
		t.Run(name, func(t *testing.T) {
			catalog, err := BuildResponseFallbackToolCatalog(&request, preserve)
			require.NoError(t, err)
			chat, err := ConvertResponseAPIToChatCompletionRequestWithOptions(&request, catalog, ResponseFallbackConversionOptions{PreserveJSONSchema: preserve})
			require.NoError(t, err)
			require.Len(t, chat.Tools, 5)
			for _, tool := range chat.Tools {
				if tool.Function.Name == "parameterless" {
					if preserve {
						require.Nil(t, tool.Function.Parameters)
					}
					continue
				}
				params := tool.Function.Parameters.(map[string]any)
				if preserve {
					encoded, err := json.Marshal(params)
					require.NoError(t, err)
					require.JSONEq(t, schema, string(encoded))
				} else {
					require.NotContains(t, params, "additionalProperties")
				}
			}
			output := chat.ResponseFormat.JsonSchema.Schema
			if preserve {
				encoded, err := json.Marshal(output)
				require.NoError(t, err)
				require.JSONEq(t, schema, string(encoded))
				// Mutation after conversion must not change either ingress schema or catalog source.
				output["description"] = "changed"
				chat.Tools[0].Function.Parameters.(map[string]any)["description"] = "changed"
			} else {
				require.NotContains(t, output, "additionalProperties")
			}
		})
	}
	after, err := json.Marshal(request)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}

// TestResponseFallbackSchemaPolicyRejectsMismatchedCatalog prevents reuse of a sanitized catalog for a schema-preserving route.
func TestResponseFallbackSchemaPolicyRejectsMismatchedCatalog(t *testing.T) {
	request := &ResponseAPIRequest{Model: "fixture"}
	for _, preserve := range []bool{false, true} {
		catalog, err := BuildResponseFallbackToolCatalog(request, preserve)
		require.NoError(t, err)
		_, err = ConvertResponseAPIToChatCompletionRequestWithOptions(request, catalog, ResponseFallbackConversionOptions{PreserveJSONSchema: !preserve})
		require.ErrorContains(t, err, "schema policy")
	}
}
