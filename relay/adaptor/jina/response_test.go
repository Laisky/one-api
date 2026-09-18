package jina

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/relaymode"
)

// TestSearchResponsePreservation verifies every opaque vector shape and native results.
func TestSearchResponsePreservation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		mode  int
		field string
		value string
	}{
		{"dense", relaymode.Embeddings, "data", `[{"index":0,"embedding":[0.12345678912345678,-0.5]}]`},
		{"base64", relaymode.Embeddings, "data", `[{"index":0,"embedding":"AAAAAA=="}]`},
		{"sparse", relaymode.Embeddings, "data", `[{"index":0,"embedding":{"hello":0.8}}]`},
		{"multivector", relaymode.Embeddings, "data", `[{"index":0,"embeddings":[[0.1,0.2],[0.3,0.4]],"tokens":["hello","world"]}]`},
		{"rerank", relaymode.Rerank, "results", `[{"index":1,"relevance_score":0.98,"document":{"image":"image.png"},"embedding":[0.1]}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"` + tc.field + `":` + tc.value + `,"usage":{"total_tokens":321,"provider_extension":7},"model":"jina-model","extension":{"keep":true}}`
			encoded, usage, err := normalizeSearchResponse([]byte(body), tc.mode)
			require.NoError(t, err)
			require.Equal(t, 321, usage.PromptTokens)
			require.Equal(t, 321, usage.TotalTokens)
			require.Zero(t, usage.CompletionTokens)
			var wire map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(encoded, &wire))
			require.JSONEq(t, tc.value, string(wire[tc.field]))
			require.JSONEq(t, `{"keep":true}`, string(wire["extension"]))
			require.JSONEq(t, `{"total_tokens":321,"prompt_tokens":321,"completion_tokens":0,"provider_extension":7}`, string(wire["usage"]))
		})
	}
}

// TestSearchResponseUsageValidation prevents malformed or missing usage becoming free calls.
func TestSearchResponseUsageValidation(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`not json`, `null`, `{}`, `{"error":{"message":"failed"}}`,
		`{"data":[]}`, `{"data":[],"usage":{}}`, `{"data":[],"usage":null}`,
		`{"data":[],"usage":{"total_tokens":-1}}`,
		`{"data":[],"usage":{"total_tokens":1.5}}`,
		`{"data":[],"usage":{"total_tokens":"12"}}`,
		`{"data":[],"usage":{"total_tokens":9223372036854775808}}`,
		`{"data":[],"usage":{"total_tokens":100,"total_tokens":1}}`,
		`{"data":[],"usage":{"total_tokens":100},"usage":{"total_tokens":1}}`,
	} {
		_, usage, err := normalizeSearchResponse([]byte(body), relaymode.Embeddings)
		require.Error(t, err, body)
		require.Nil(t, usage)
	}
	_, usage, err := normalizeSearchResponse([]byte(`{"data":[],"usage":{"total_tokens":0}}`), relaymode.Embeddings)
	require.NoError(t, err)
	require.Zero(t, usage.TotalTokens)
}

// observedBody records closure while exposing a normal upstream response reader.
type observedBody struct {
	io.Reader
	closed bool
}

// Close records that the response handler released the upstream body.
func (b *observedBody) Close() error { b.closed = true; return nil }

// TestSearchResponseHandler verifies lifecycle, normalized JSON and invalid-body rejection.
func TestSearchResponseHandler(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := &observedBody{Reader: strings.NewReader(`{"results":[],"usage":{"total_tokens":12}}`)}
	usage, err := handleSearchResponse(c, &http.Response{StatusCode: http.StatusOK, Body: body}, relaymode.Rerank)
	require.Nil(t, err)
	require.True(t, body.closed)
	require.Equal(t, 12, usage.PromptTokens)
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"results":[],"usage":{"total_tokens":12,"prompt_tokens":12,"completion_tokens":0}}`, w.Body.String())
	_, err = handleSearchResponse(c, nil, relaymode.Rerank)
	require.NotNil(t, err)
	require.Equal(t, http.StatusBadGateway, err.StatusCode)
}

// TestSearchReceiptSurvivesInvalidResults ensures delivered-work evidence is
// retained even when the results payload cannot be returned to the caller.
func TestSearchReceiptSurvivesInvalidResults(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"null", "{}"} {
		_, usage, err := normalizeSearchResponse([]byte(`{"data":`+field+`,"usage":{"total_tokens":123}}`), relaymode.Embeddings)
		require.Error(t, err)
		require.NotNil(t, usage)
		require.Equal(t, 123, usage.PromptTokens)
	}
}
