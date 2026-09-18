package jina

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// requestContext returns an isolated context with a reusable JSON request body.
func requestContext(body string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

// jsonPayload serializes a converted request and decodes its actual wire fields.
func jsonPayload(t *testing.T, payload any) map[string]any {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(data, &wire))
	return wire
}

// TestEmbeddingNativeOptions verifies mapped models, multimodal input and false values.
func TestEmbeddingNativeOptions(t *testing.T) {
	t.Parallel()
	raw := `{"model":"public-alias","input":[{"text":"hello"},{"image":"data:image/png;base64,AA=="}],"task":"retrieval.passage","normalized":false,"truncate":false,"late_chunking":true,"return_multivector":true,"return_tokenized_input":true,"embedding_type":["float","binary"]}`
	var request model.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal([]byte(raw), &request))
	request.Model = "jina-embeddings-v4"
	request.EncodingFormat = "base64"
	request.ExtraBody = map[string]any{"normalized": true, "task": "retrieval.query", "model": "forbidden", "ignored_option": true}
	converted, err := (&Adaptor{}).ConvertRequest(requestContext(raw), relaymode.Embeddings, &request)
	require.NoError(t, err)
	wire := jsonPayload(t, converted)
	require.Equal(t, "jina-embeddings-v4", wire["model"])
	require.Equal(t, "retrieval.passage", wire["task"])
	require.Equal(t, false, wire["normalized"])
	require.Equal(t, false, wire["truncate"])
	require.Equal(t, true, wire["late_chunking"])
	require.Equal(t, true, wire["return_multivector"])
	require.Equal(t, true, wire["return_tokenized_input"])
	require.Equal(t, []any{"float", "binary"}, wire["embedding_type"])
	require.NotContains(t, wire, "dimensions")
	require.Equal(t, request.Input, wire["input"])
	require.NotContains(t, wire, "encoding_format")
	require.NotContains(t, wire, "ignored_option")
	require.Equal(t, true, request.ExtraBody["normalized"], "conversion must not mutate retry input")
}

// TestEmbeddingEncodingAlias verifies OpenAI encoding_format and extra_body precedence.
func TestEmbeddingEncodingAlias(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		extra map[string]any
		want  any
	}{
		{"alias", nil, "base64"},
		{"native", map[string]any{"embedding_type": "float"}, "float"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := &model.GeneralOpenAIRequest{Model: "jina-embeddings-v3", Input: "hello", Dimensions: 512, EncodingFormat: "base64", ExtraBody: tc.extra}
			converted, err := (&Adaptor{}).ConvertRequest(nil, relaymode.Embeddings, request)
			require.NoError(t, err)
			wire := jsonPayload(t, converted)
			require.Equal(t, tc.want, wire["embedding_type"])
			require.Equal(t, float64(512), wire["dimensions"])
		})
	}
}

// TestRerankNativeOptions verifies structured inputs and native boolean options survive.
func TestRerankNativeOptions(t *testing.T) {
	t.Parallel()
	raw := `{"model":"alias","query":{"image":"https://example.com/q.png"},"documents":["plain",{"text":"a document"},{"image":"data:image/png;base64,AA=="}],"return_documents":false,"top_n":2}`
	var request model.RerankRequest
	require.NoError(t, json.Unmarshal([]byte(raw), &request))
	require.NoError(t, request.Normalize())
	request.Model = "jina-reranker-m0"
	converted, err := (&Adaptor{}).ConvertRerankRequest(requestContext(raw), request.Clone())
	require.NoError(t, err)
	wire := jsonPayload(t, converted)
	require.Equal(t, "jina-reranker-m0", wire["model"])
	require.Equal(t, map[string]any{"image": "https://example.com/q.png"}, wire["query"])
	require.Equal(t, []any{"plain", map[string]any{"text": "a document"}, map[string]any{"image": "data:image/png;base64,AA=="}}, wire["documents"])
	require.Equal(t, false, wire["return_documents"])
	require.Equal(t, float64(2), wire["top_n"])
}

// TestRerankLengthAlias verifies legacy length limits map only to supporting models.
func TestRerankLengthAlias(t *testing.T) {
	t.Parallel()
	limit := 123
	for _, name := range []string{"jina-reranker-v3", "jina-reranker-v3.5", "jina-reranker-m0"} {
		request := &model.RerankRequest{Model: name, Query: "q", Documents: []string{"d"}, MaxTokensPerDoc: &limit}
		converted, err := (&Adaptor{}).ConvertRerankRequest(nil, request)
		require.NoError(t, err)
		wire := jsonPayload(t, converted)
		require.NotContains(t, wire, "max_tokens_per_doc")
		if name == "jina-reranker-m0" {
			require.NotContains(t, wire, "max_doc_length")
		} else {
			require.Equal(t, float64(limit), wire["max_doc_length"])
		}
	}
}

// TestRerankNativeLengthPrecedence verifies v3.5 options override legacy length aliases.
func TestRerankNativeLengthPrecedence(t *testing.T) {
	t.Parallel()
	limit := 123
	request := &model.RerankRequest{Model: "jina-reranker-v3.5", Query: "q", Documents: []string{"d"}, MaxTokensPerDoc: &limit}
	converted, err := (&Adaptor{}).ConvertRerankRequest(requestContext(`{"max_doc_length":4096,"return_embeddings":false}`), request)
	require.NoError(t, err)
	wire := jsonPayload(t, converted)
	require.Equal(t, float64(4096), wire["max_doc_length"])
	require.Equal(t, false, wire["return_embeddings"])
}

// TestOCRConversion verifies token-limit precedence, usage streaming, and immutability.
func TestOCRConversion(t *testing.T) {
	t.Parallel()
	request := &model.GeneralOpenAIRequest{Model: "jina-ocr-v1", MaxTokens: 64, Stream: true}
	converted, err := (&Adaptor{}).ConvertRequest(nil, relaymode.ChatCompletions, request)
	require.NoError(t, err)
	chat := converted.(*model.GeneralOpenAIRequest)
	require.Equal(t, 64, *chat.MaxCompletionTokens)
	require.Zero(t, chat.MaxTokens)
	require.True(t, chat.StreamOptions.IncludeUsage)
	require.Nil(t, request.MaxCompletionTokens)
	require.Nil(t, request.StreamOptions)
	limit := 128
	request.MaxCompletionTokens = &limit
	converted, err = (&Adaptor{}).ConvertRequest(nil, relaymode.ChatCompletions, request)
	require.NoError(t, err)
	require.Equal(t, 128, *converted.(*model.GeneralOpenAIRequest).MaxCompletionTokens)
}

// TestClaudeOCRConversion verifies shared Messages conversion plus Jina streaming options.
func TestClaudeOCRConversion(t *testing.T) {
	t.Parallel()
	stream := true
	request := &model.ClaudeRequest{Model: "jina-ocr-v1", MaxTokens: 64, Stream: &stream}
	converted, err := (&Adaptor{}).ConvertClaudeRequest(requestContext(`{}`), request)
	require.NoError(t, err)
	chat, ok := converted.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, 64, *chat.MaxCompletionTokens)
	require.True(t, chat.StreamOptions.IncludeUsage)
}

// TestNativeRoutingAndAuthentication verifies supported URLs and channel-key replacement.
func TestNativeRoutingAndAuthentication(t *testing.T) {
	t.Parallel()
	a := &Adaptor{}
	for mode, path := range map[int]string{
		relaymode.Embeddings: "/v1/embeddings", relaymode.Rerank: "/v1/rerank",
		relaymode.ChatCompletions: "/v1/chat/completions", relaymode.ClaudeMessages: "/v1/chat/completions",
	} {
		for _, base := range []string{"", "https://api.jina.ai", "https://api.jina.ai/", "https://api.jina.ai/v1", "https://api.jina.ai/v1/"} {
			m := &meta.Meta{Mode: mode, ChannelType: channeltype.Jina, BaseURL: base, APIKey: "jina-channel-key"}
			url, err := a.GetRequestURL(m)
			require.NoError(t, err)
			require.Equal(t, "https://api.jina.ai"+path, url)
			c := requestContext(`{}`)
			c.Request.Header.Set("Authorization", "Bearer caller-key")
			req, err := http.NewRequest(http.MethodPost, url, nil)
			require.NoError(t, err)
			require.NoError(t, a.SetupRequestHeader(c, req, m))
			require.Equal(t, "Bearer jina-channel-key", req.Header.Get("Authorization"))
		}
	}
	_, err := a.GetRequestURL(&meta.Meta{Mode: relaymode.ImagesGenerations})
	require.Error(t, err)
	_, err = a.GetRequestURL(nil)
	require.Error(t, err)
	_, err = a.ConvertRequest(nil, relaymode.Embeddings, nil)
	require.Error(t, err)
	_, err = a.ConvertRequest(nil, relaymode.Completions, &model.GeneralOpenAIRequest{})
	require.Error(t, err)
	_, err = a.ConvertRerankRequest(nil, nil)
	require.Error(t, err)
	_, err = a.ConvertImageRequest(nil, &model.ImageRequest{})
	require.Error(t, err)
	_, err = a.ConvertRequest(requestContext(`{`), relaymode.Embeddings, &model.GeneralOpenAIRequest{})
	require.Error(t, err)
}
