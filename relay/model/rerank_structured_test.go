package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRerankStructuredDecode verifies text compatibility, mixed documents and deep copies.
func TestRerankStructuredDecode(t *testing.T) {
	t.Parallel()
	var request RerankRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"jina-reranker-m0","query":{"image":"q.png"},"documents":["text",{"text":"native text"},{"image":"d.png"}],"top_n":2}`), &request))
	require.NoError(t, request.Normalize())
	require.True(t, request.HasStructuredInput())
	require.Equal(t, "[image]", request.Query)
	require.Equal(t, []string{"text", "native text", "[image]"}, request.Documents)
	require.Equal(t, 2, *request.TopN)
	clone := request.Clone()
	clone.Documents[0] = "changed"
	clone.NativeQuery[0] = '['
	clone.NativeDocuments[0][0] = 'x'
	require.Equal(t, "text", request.Documents[0])
	require.Equal(t, byte('{'), request.NativeQuery[0])
	require.Equal(t, byte('"'), request.NativeDocuments[0][0])

	// Reusing a DTO must not carry native fields into a later text-only request.
	require.NoError(t, json.Unmarshal([]byte(`{"model":"text-model","input":" legacy query ","documents":["a","b"]}`), &request))
	require.NoError(t, request.Normalize())
	require.Equal(t, "legacy query", request.Query)
	require.Equal(t, []string{"a", "b"}, request.Documents)
	require.False(t, request.HasStructuredInput())
	encoded, err := json.Marshal(request)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"text-model","query":"legacy query","input":" legacy query ","documents":["a","b"]}`, string(encoded))
}

// TestRerankStructuredPreservesStructuredQuery pins that decoding keeps a
// structured query instead of rejecting it. Which query shapes an upstream
// accepts is model-specific — api.jina.ai takes {"text": ...} / {"image": ...}
// queries on jina-reranker-m0 (HTTP 200) but answers 422 "'query' Input should
// be a valid string" on v3/v3.5/v2/colbert — so the rule belongs to the
// adaptor's model-aware admission contract, not to this provider-agnostic
// decoder. See TestRerankConversionEnforcesModelModalities for the Jina side.
// Parameters: t is the testing handle used for assertions.
// Returns: nothing; the test fails through t when a structured query is lost.
func TestRerankStructuredPreservesStructuredQuery(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ query, wantText string }{
		"text_object":  {`{"text":"refund policy"}`, "refund policy"},
		"image_object": {`{"image":"https://example.com/q.png"}`, "[image]"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var request RerankRequest
			body := `{"model":"jina-reranker-m0","query":` + tc.query + `,"documents":["a"]}`
			require.NoError(t, json.Unmarshal([]byte(body), &request))
			require.NoError(t, request.Normalize())
			require.True(t, request.HasStructuredInput())
			require.Equal(t, tc.wantText, request.Query, "text estimate must survive for accounting")
			require.JSONEq(t, tc.query, string(request.NativeQuery),
				"the native query must be forwarded verbatim: m0 scores {\"text\"} and a bare string differently")
		})
	}
}

// TestRerankStructuredInvalidInput rejects malformed values instead of silently coercing them.
func TestRerankStructuredInvalidInput(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`{"query":4}`,
		`{"query":{"image":""}}`, `{"query":{"image":"q","text":"x"}}`,
		`{"query":"q","documents":[null]}`, `{"query":"q","documents":[3]}`,
		`{"query":"q","documents":[{"image":42}]}`, `{"query":"q","documents":[{"unknown":"x"}]}`,
		`{"query":"q","documents":[{"text":"x","image":"y"}]}`,
	} {
		var request RerankRequest
		require.Error(t, json.Unmarshal([]byte(body), &request), body)
	}
}
