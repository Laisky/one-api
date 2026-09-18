package jina

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestRerankConversionEnforcesModelModalities calls the adaptor directly, without
// the controller's admission check, and encodes the request shapes api.jina.ai
// actually accepts (verified live against /v1/rerank):
//
//	shape                    | v3, v3.5, v2, colbert | m0
//	query: "string"          | 200                   | 200
//	query: {"text": ...}     | 422                   | 200
//	query: {"image": ...}    | 422                   | 200
//	documents: [{"text"...}] | 200                   | 200
//	documents: [{"image"..}] | 422                   | 200
//
// So a structured query of ANY shape is multimodal-only, while a {"text": ...}
// document is accepted everywhere. Text-only models answer 422 "'query' Input
// should be a valid string", so the gateway must reject those before spending an
// upstream call.
func TestRerankConversionEnforcesModelModalities(t *testing.T) {
	t.Parallel()
	for _, modelName := range []string{"jina-reranker-v3", "jina-reranker-v3.5", "jina-reranker-m0"} {
		for _, tc := range []struct {
			name, query, document string
			// multimodalOnly marks shapes upstream accepts only on a multimodal model.
			multimodalOnly bool
		}{
			{"string_query_text_document", `"query"`, `{"text":"document"}`, false},
			{"text_object_query", `{"text":"query"}`, `{"text":"document"}`, true},
			{"image_query", `{"image":"https://example.com/q.png"}`, `{"text":"document"}`, true},
			{"image_document", `"query"`, `{"image":"https://example.com/d.png"}`, true},
		} {
			t.Run(modelName+"/"+tc.name, func(t *testing.T) {
				var request model.RerankRequest
				raw := `{"model":"` + modelName + `","query":` + tc.query + `,"documents":[` + tc.document + `]}`
				require.NoError(t, json.Unmarshal([]byte(raw), &request))
				require.NoError(t, request.Normalize())
				before := request.Clone()
				converted, err := (&Adaptor{}).ConvertRerankRequest(nil, &request)
				if tc.multimodalOnly && modelName != "jina-reranker-m0" {
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
