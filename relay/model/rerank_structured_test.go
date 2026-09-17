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

// TestRerankStructuredInvalidInput rejects malformed values instead of silently coercing them.
func TestRerankStructuredInvalidInput(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`{"query":4}`, `{"query":{"text":"not an image query"}}`,
		`{"query":{"image":""}}`, `{"query":{"image":"q","text":"x"}}`,
		`{"query":"q","documents":[null]}`, `{"query":"q","documents":[3]}`,
		`{"query":"q","documents":[{"image":42}]}`, `{"query":"q","documents":[{"unknown":"x"}]}`,
		`{"query":"q","documents":[{"text":"x","image":"y"}]}`,
	} {
		var request RerankRequest
		require.Error(t, json.Unmarshal([]byte(body), &request), body)
	}
}
