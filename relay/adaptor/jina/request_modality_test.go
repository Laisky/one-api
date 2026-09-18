package jina

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestRerankConversionEnforcesModelModalities calls the adaptor directly, without
// the controller's admission check. Structured text is preserved for text-only
// models, while image queries and documents require the multimodal m0 model.
func TestRerankConversionEnforcesModelModalities(t *testing.T) {
	t.Parallel()
	for _, modelName := range []string{"jina-reranker-v3", "jina-reranker-v3.5", "jina-reranker-m0"} {
		for _, tc := range []struct {
			name, query, document string
			image                 bool
		}{
			{"text", `{"text":"query"}`, `{"text":"document"}`, false},
			{"image_query", `{"image":"https://example.com/q.png"}`, `{"text":"document"}`, true},
			{"image_document", `{"text":"query"}`, `{"image":"https://example.com/d.png"}`, true},
		} {
			t.Run(modelName+"/"+tc.name, func(t *testing.T) {
				var request model.RerankRequest
				raw := `{"model":"` + modelName + `","query":` + tc.query + `,"documents":[` + tc.document + `]}`
				require.NoError(t, json.Unmarshal([]byte(raw), &request))
				require.NoError(t, request.Normalize())
				before := request.Clone()
				converted, err := (&Adaptor{}).ConvertRerankRequest(nil, &request)
				if tc.image && modelName != "jina-reranker-m0" {
					require.Error(t, err)
					require.Nil(t, converted)
				} else {
					require.NoError(t, err)
					wire, err := json.Marshal(converted)
					require.NoError(t, err)
					require.JSONEq(t, raw, string(wire))
				}
				require.Equal(t, before, request.Clone(), "validation/conversion cannot mutate a retry payload")
			})
		}
	}
}
