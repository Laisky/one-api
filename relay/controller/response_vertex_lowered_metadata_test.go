package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponsesVertexRewrittenCallMetadata verifies actual custom/search continuation dispatch retains native call metadata.
func TestResponsesVertexRewrittenCallMetadata(t *testing.T) {
	for _, kind := range []string{"custom", "search"} {
		for _, signed := range []bool{false, true} {
			name := kind + "/unsigned"
			if signed {
				name = kind + "/signed"
			}
			t.Run(name, func(t *testing.T) {
				extra := ""
				if signed {
					extra = `,"extra_content":{"google":{"thought_signature":"native-opaque"},"oneapi":{"gemini_part_index":0}}`
				}
				tools, call, result, expected := "", "", "", ""
				if kind == "custom" {
					tools = `[{"type":"custom","name":"apply_patch"}]`
					call = `{"type":"custom_tool_call","call_id":"call_x","name":"apply_patch","input":"patch"` + extra + `}`
					result = `{"type":"custom_tool_call_output","call_id":"call_x","output":"done"}`
					expected = "apply_patch"
				} else {
					tools = `[{"type":"tool_search","execution":"client"}]`
					call = `{"type":"tool_search_call","call_id":"call_x","execution":"client","arguments":{"query":"docs"}` + extra + `}`
					result = `{"type":"tool_search_output","call_id":"call_x","execution":"client","tools":[]}`
					expected = "tool_search"
				}
				p := vertexMixedTextProvider(t)
				body := `{"model":"` + p.actual + `","tools":` + tools + `,"input":[` + call + `,` + result + `]}`
				sent := runProviderMatrix(t, p, false, body, providerMatrixOptions{})
				requireProviderMatrixDispatched(t, sent)
				var wire struct {
					Contents []struct {
						Role  string `json:"role"`
						Parts []struct {
							Call *struct {
								Name string `json:"name"`
							} `json:"functionCall"`
							Signature string `json:"thoughtSignature"`
						} `json:"parts"`
					} `json:"contents"`
				}
				require.NoError(t, json.Unmarshal(sent.got.Body, &wire))
				found := false
				for _, content := range wire.Contents {
					for _, part := range content.Parts {
						if part.Call == nil {
							continue
						}
						require.False(t, found)
						found = true
						require.Equal(t, expected, part.Call.Name)
						signature := ""
						if signed {
							signature = "native-opaque"
						}
						require.Equal(t, signature, part.Signature)
					}
				}
				require.True(t, found)
			})
		}
	}
}
