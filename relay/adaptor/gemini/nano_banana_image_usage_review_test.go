package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
)

// TestGeminiOctober28ImageCandidateReceipt validates that the provider's
// authoritative modality breakdown is copied as private accounting evidence
// without changing public completion_tokens, total_tokens or reasoning tokens.
func TestGeminiOctober28ImageCandidateReceipt(t *testing.T) {
	for _, tt := range []struct {
		name         string
		receipt      UsageMetadata
		imageTokens  int
		completion   int
	}{
		{
			name: "image plus text and reasoning",
			receipt: UsageMetadata{
				PromptTokenCount: 100, CandidatesTokenCount: 1125,
				ThoughtsTokenCount: 10, TotalTokenCount: 1235,
				CandidatesTokensDetails: []PromptTokensDetails{
					{Modality: "TEXT", TokenCount: 5},
					{Modality: "IMAGE", TokenCount: 1120},
				},
			},
			imageTokens: 1120, completion: 1135,
		},
		{
			name: "no breakdown is not guessed",
			receipt: UsageMetadata{
				PromptTokenCount: 100, CandidatesTokenCount: 1125,
				ThoughtsTokenCount: 10, TotalTokenCount: 1235,
			},
			imageTokens: 0, completion: 1135,
		},
		{
			name: "malformed oversized breakdown is clamped",
			receipt: UsageMetadata{
				CandidatesTokenCount: 20, TotalTokenCount: 20,
				CandidatesTokensDetails: []PromptTokensDetails{
					{Modality: "image", TokenCount: 15},
					{Modality: "IMAGE", TokenCount: 100},
					{Modality: "IMAGE", TokenCount: -3},
				},
			},
			imageTokens: 20, completion: 20,
		},
		{
			name: "thinking tokens excluded from image bucket",
			receipt: UsageMetadata{
				CandidatesTokenCount: 0, ThoughtsTokenCount: 13,
				TotalTokenCount: 13,
				CandidatesTokensDetails: []PromptTokensDetails{{Modality: "IMAGE", TokenCount: 1024}},
			},
			imageTokens: 0, completion: 13,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			usage := geminiUsageMetadataToOpenAIUsage(&tt.receipt)
			require.NotNil(t, usage)
			require.Equal(t, tt.imageTokens, usage.CandidateImageTokens)
			require.Equal(t, tt.completion, usage.CompletionTokens)
			require.Equal(t, tt.receipt.TotalTokenCount, usage.TotalTokens)
			require.Zero(t, usage.ImageTokensBilledSeparately, "receipt alone never charges a render")
			encoded, err := json.Marshal(usage)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "candidate_image_tokens")
			require.NotContains(t, string(encoded), "image_tokens_billed_separately")
			require.NotContains(t, string(encoded), "CandidateImageTokens")
			require.NotContains(t, string(encoded), "ImageTokensBilledSeparately")
		})
	}
}

// TestGeminiOctober28ImageStreamReceipt verifies the same image and token
// evidence survives the real streaming handler, which uses cumulative snapshots
// rather than summing each chunk's counters.
func TestGeminiOctober28ImageStreamReceipt(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	first := `{"candidates":[{"content":{"parts":[{"text":"Generating image"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2,"totalTokenCount":12}}`
	last := `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":1125,"thoughtsTokenCount":10,"totalTokenCount":1145,"candidatesTokensDetails":[{"modality":"TEXT","tokenCount":5},{"modality":"IMAGE","tokenCount":1120}]}}`
	stream := "data: " + first + "\n\ndata: " + last + "\n\ndata: [DONE]\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(stream)),
		Header: http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	apiErr, _, usage := StreamHandler(c, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 1135, usage.CompletionTokens)
	require.Equal(t, 1145, usage.TotalTokens)
	require.Equal(t, 1120, usage.CandidateImageTokens)
	require.Equal(t, 1, c.GetInt(ctxkey.OutputImageCount))
}
