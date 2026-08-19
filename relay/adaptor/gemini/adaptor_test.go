package gemini

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

func TestAdaptorGetRequestURLGeminiVersions(t *testing.T) {
	t.Parallel()
	adaptor := &Adaptor{}
	baseURL := "https://generativelanguage.googleapis.com"
	originalVersion := config.GeminiVersion
	config.GeminiVersion = "v1"
	defer func() {
		config.GeminiVersion = originalVersion
	}()

	testCases := []struct {
		name            string
		model           string
		expectedVersion string
	}{
		{name: "GeminiThree", model: "gemini-3-pro-preview", expectedVersion: "v1beta"},
		{name: "GeminiTwoFive", model: "gemini-2.5-flash", expectedVersion: "v1beta"},
		{name: "GeminiTwoZero", model: "gemini-2.0-flash", expectedVersion: "v1beta"},
		{name: "GeminiLegacy", model: "gemini-1.0-pro", expectedVersion: "v1"},
		{name: "GemmaThree", model: "gemma-3-8b-it", expectedVersion: "v1beta"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			metaInfo := &meta.Meta{
				ActualModelName: tc.model,
				Mode:            relaymode.ChatCompletions,
				BaseURL:         baseURL,
			}

			url, err := adaptor.GetRequestURL(metaInfo)
			require.NoError(t, err)
			expected := fmt.Sprintf("%s/%s/models/%s:%s", baseURL, tc.expectedVersion, tc.model, "generateContent")
			require.Equal(t, expected, url)
		})
	}
}

// TestAdaptorConvertRequestEmbeddingsSupportsMultimodal verifies Gemini embedding conversion preserves multimodal parts.
// Parameters: t coordinates the test case execution. Returns: no values.
func TestAdaptorConvertRequestEmbeddingsSupportsMultimodal(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}

	converted, err := adaptor.ConvertRequest(nil, relaymode.Embeddings, &relaymodel.GeneralOpenAIRequest{
		Model: "gemini-embedding-2-preview",
		Input: []any{"hello", "world"},
	})
	require.NoError(t, err)

	batch, ok := converted.(*BatchEmbeddingRequest)
	require.True(t, ok, "expected BatchEmbeddingRequest")
	require.Len(t, batch.Requests, 2)

	converted, err = adaptor.ConvertRequest(nil, relaymode.Embeddings, &relaymodel.GeneralOpenAIRequest{
		Model: "gemini-embedding-2-preview",
		Input: map[string]any{
			"parts": []any{
				map[string]any{"text": "An image of a cat"},
				map[string]any{
					"inline_data": map[string]any{
						"mime_type": "image/png",
						"data":      "ZmFrZQ==",
					},
				},
			},
		},
	})
	require.NoError(t, err)

	batch, ok = converted.(*BatchEmbeddingRequest)
	require.True(t, ok, "expected BatchEmbeddingRequest")
	require.Len(t, batch.Requests, 1)
	require.Len(t, batch.Requests[0].Content.Parts, 2)
	require.Equal(t, "An image of a cat", batch.Requests[0].Content.Parts[0].Text)
	require.NotNil(t, batch.Requests[0].Content.Parts[1].InlineData)
}
