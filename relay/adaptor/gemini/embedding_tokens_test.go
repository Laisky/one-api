package gemini

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

func init() {
	config.ApproximateTokenEnabled = true
}

// TestBuildEmbeddingContents verifies Gemini embedding inputs normalize into the expected content structure.
// Parameters: t coordinates the test case execution. Returns: no values.
func TestBuildEmbeddingContents(t *testing.T) {
	t.Run("mixed top-level entries stay separate", func(t *testing.T) {
		contents, hasMultimodal, err := BuildEmbeddingContents([]any{
			"The dog is cute",
			map[string]any{
				"inline_data": map[string]any{
					"mime_type": "image/png",
					"data":      "ZmFrZQ==",
				},
			},
		})
		require.NoError(t, err)
		require.True(t, hasMultimodal)
		require.Len(t, contents, 2)
		require.Equal(t, "The dog is cute", contents[0].Parts[0].Text)
		require.NotNil(t, contents[1].Parts[0].InlineData)
		require.Equal(t, "image/png", contents[1].Parts[0].InlineData.MimeType)
	})

	t.Run("content map aggregates multiple parts", func(t *testing.T) {
		contents, hasMultimodal, err := BuildEmbeddingContents(map[string]any{
			"parts": []any{
				map[string]any{"text": "An image of a dog"},
				map[string]any{
					"inlineData": map[string]any{
						"mimeType": "image/png",
						"data":     "ZmFrZQ==",
					},
				},
			},
		})
		require.NoError(t, err)
		require.True(t, hasMultimodal)
		require.Len(t, contents, 1)
		require.Len(t, contents[0].Parts, 2)
		require.Equal(t, "An image of a dog", contents[0].Parts[0].Text)
		require.NotNil(t, contents[0].Parts[1].InlineData)
	})
}

// TestEstimateEmbeddingPromptUsageUsesCountTokens verifies Gemini embeddings preflight uses countTokens and preserves modality details.
// Parameters: t coordinates the test case execution. Returns: no values.
func TestEstimateEmbeddingPromptUsageUsesCountTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1beta/models/gemini-embedding-2-preview:countTokens", r.URL.Path)
		require.Equal(t, "test-key", r.Header.Get("x-goog-api-key"))

		var body struct {
			Contents []ChatContent `json:"contents"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Len(t, body.Contents, 1)
		require.Len(t, body.Contents[0].Parts, 2)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"totalTokens": 345,
			"promptTokensDetails": [
				{"modality": "TEXT", "tokenCount": 11},
				{"modality": "IMAGE", "tokenCount": 334}
			]
		}`))
	}))
	t.Cleanup(server.Close)

	prevClient := client.HTTPClient
	client.HTTPClient = server.Client()
	t.Cleanup(func() {
		client.HTTPClient = prevClient
	})

	usage, hasMultimodal, bizErr := EstimateEmbeddingPromptUsage(c, &meta.Meta{
		BaseURL:         server.URL,
		APIKey:          "test-key",
		ActualModelName: "gemini-embedding-2-preview",
	}, &relaymodel.GeneralOpenAIRequest{
		Model: "gemini-embedding-2-preview",
		Input: map[string]any{
			"parts": []any{
				map[string]any{"text": "An image of a dog"},
				map[string]any{
					"inline_data": map[string]any{
						"mime_type": "image/png",
						"data":      "ZmFrZQ==",
					},
				},
			},
		},
	})
	require.Nil(t, bizErr)
	require.True(t, hasMultimodal)
	require.NotNil(t, usage)
	require.Equal(t, 345, usage.PromptTokens)
	require.Equal(t, 345, usage.TotalTokens)
	require.NotNil(t, usage.PromptTokensDetails)
	require.Equal(t, 11, usage.PromptTokensDetails.TextTokens)
	require.Equal(t, 334, usage.PromptTokensDetails.ImageTokens)
}

// TestEstimateEmbeddingPromptUsageFallsBackToText verifies text-only Gemini embeddings still work when countTokens is unavailable.
// Parameters: t coordinates the test case execution. Returns: no values.
func TestEstimateEmbeddingPromptUsageFallsBackToText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"code":503,"message":"temporary failure","status":"UNAVAILABLE"}}`))
	}))
	t.Cleanup(server.Close)

	prevClient := client.HTTPClient
	client.HTTPClient = server.Client()
	t.Cleanup(func() {
		client.HTTPClient = prevClient
	})

	request := &relaymodel.GeneralOpenAIRequest{
		Model: "gemini-embedding-2-preview",
		Input: map[string]any{
			"parts": []any{
				map[string]any{"text": "hello"},
				map[string]any{"text": "world"},
			},
		},
	}
	usage, hasMultimodal, bizErr := EstimateEmbeddingPromptUsage(c, &meta.Meta{
		BaseURL:         server.URL,
		APIKey:          "test-key",
		ActualModelName: "gemini-embedding-2-preview",
	}, request)
	require.Nil(t, bizErr)
	require.False(t, hasMultimodal)
	require.NotNil(t, usage)
	require.Nil(t, usage.PromptTokensDetails)
	require.Equal(t, openai.CountTokenText("hello", request.Model)+openai.CountTokenText("world", request.Model), usage.PromptTokens)
}

// TestEstimateEmbeddingPromptUsageRejectsMultimodalWithoutCountTokens verifies multimodal Gemini embeddings fail closed when preflight counting cannot run.
// Parameters: t coordinates the test case execution. Returns: no values.
func TestEstimateEmbeddingPromptUsageRejectsMultimodalWithoutCountTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"code":503,"message":"temporary failure","status":"UNAVAILABLE"}}`))
	}))
	t.Cleanup(server.Close)

	prevClient := client.HTTPClient
	client.HTTPClient = server.Client()
	t.Cleanup(func() {
		client.HTTPClient = prevClient
	})

	usage, hasMultimodal, bizErr := EstimateEmbeddingPromptUsage(c, &meta.Meta{
		BaseURL:         server.URL,
		APIKey:          "test-key",
		ActualModelName: "gemini-embedding-2-preview",
	}, &relaymodel.GeneralOpenAIRequest{
		Model: "gemini-embedding-2-preview",
		Input: map[string]any{
			"parts": []any{
				map[string]any{
					"inline_data": map[string]any{
						"mime_type": "image/png",
						"data":      "ZmFrZQ==",
					},
				},
			},
		},
	})
	require.Nil(t, usage)
	require.True(t, hasMultimodal)
	require.NotNil(t, bizErr)
	require.Equal(t, http.StatusServiceUnavailable, bizErr.StatusCode)
	require.Contains(t, bizErr.Message, "temporary failure")
}
