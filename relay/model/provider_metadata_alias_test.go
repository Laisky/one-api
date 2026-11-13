package model

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestScopeUntypedContentAliasesParity verifies every accepted carrier spelling follows
// the same destination policy as raw JSON, without mutating caller-owned content.
func TestScopeUntypedContentAliasesParity(t *testing.T) {
	cases := []struct{ name, fields string }{
		{"canonical_control", `"extra_content":{"google":{"thought_signature":"canonical"}}`},
		{"canonical_and_alias", `"extra_content":{"google":{"thought_signature":"canonical"},"oneapi":{"gemini_part_index":0}},"Extra_Content":{"google":{"thought_signature":"alias"}}`},
		{"alias_only", `"Extra_Content":{"google":{"thought_signature":"alias"}}`},
		{"multiple_aliases", `"Extra_Content":{"google":{"thought_signature":"one"}},"EXTRA_CONTENT":{"google":{"thought_signature":"two"}},"eXtra_Content":{"oneapi":{"gemini_part_index":0}}`},
		{"canonical_and_multiple_aliases", `"extra_content":{"google":{"thought_signature":"canonical"}},"Extra_Content":{"google":{"thought_signature":"one"}},"EXTRA_CONTENT":{"google":{"thought_signature":"two"}}`},
		{"malformed_alias", `"Extra_Content":17`},
		{"escaped_canonical", `"extra\u005fcontent":{"google":{"thought_signature":"canonical"}},"Extra_Content":null`},
	}
	for _, tc := range cases {
		for _, scope := range []ProviderMetadataScope{ProviderMetadataNone, ProviderMetadataGoogleWire, ProviderMetadataNative} {
			t.Run(tc.name+"/"+scope.String(), func(t *testing.T) {
				raw := `{"model":"fixture","messages":[{"role":"assistant","content":[{"type":"text","text":"visible",` + tc.fields + `,"literal":{"Extra_Content":{"google":"caller-data"}},"n":9007199254740993},{"type":"text","text":"untouched"}]}]}`
				var request GeneralOpenAIRequest
				decoder := json.NewDecoder(strings.NewReader(raw))
				decoder.UseNumber()
				require.NoError(t, decoder.Decode(&request))
				shared := request.Messages
				original := request.Messages[0].Content.([]any)
				before, err := json.Marshal(request)
				require.NoError(t, err)
				originalJSON, err := json.Marshal(original)
				require.NoError(t, err)
				expected, rawCount, err := ScopeChatBodyProviderMetadata(before, scope)
				require.NoError(t, err)
				count := ScopeChatRequestProviderMetadata(&request, scope)
				after, err := json.Marshal(request)
				require.NoError(t, err)
				require.JSONEq(t, string(expected), string(after), "typed and raw scoping must share the carrier policy")
				require.Equal(t, rawCount, count, "count every scoped alias, not just one matching key or part")
				retained, err := json.Marshal(shared[0].Content)
				require.NoError(t, err)
				require.Equal(t, string(originalJSON), string(retained), "shared caller-owned maps and slice values must not be modified")
				parts := request.Messages[0].Content.([]any)
				part := parts[0].(map[string]any)
				require.Equal(t, json.Number("9007199254740993"), part["n"])
				require.Equal(t, map[string]any{"Extra_Content": map[string]any{"google": "caller-data"}}, part["literal"])
				require.Zero(t, ScopeChatRequestProviderMetadata(&request, scope), "scoping must be idempotent")
				if scope != ProviderMetadataNative {
					for _, key := range []string{"Extra_Content", "EXTRA_CONTENT", "eXtra_Content"} {
						require.NotContains(t, part, key)
					}
					if scope == ProviderMetadataNone {
						require.NotContains(t, part, "extra_content")
					}
				}
				if count == 0 {
					require.Same(t, &shared[0], &request.Messages[0])
					require.Same(t, &original[0], &parts[0])
				} else {
					require.NotSame(t, &shared[0], &request.Messages[0])
					require.NotSame(t, &original[0], &parts[0])
					part["text"] = "changed scoped copy"
					require.Equal(t, "visible", original[0].(map[string]any)["text"], "changed parts must be cloned")
				}
			})
		}
	}
}
