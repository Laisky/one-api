package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
)

// TestGeminiOctober28ImageReceiptNoDoubleCharge ensures the real native Gemini
// response and final quota settlement never bill measured image output tokens
// twice. It also checks the conservative legacy behavior without a breakdown.
func TestGeminiOctober28ImageReceiptNoDoubleCharge(t *testing.T) {
	for _, tt := range []struct {
		name           string
		candidateParts string
		imageCount     int
		billableTokens int
	}{
		{"measured_image", `,"candidatesTokensDetails":[{"modality":"TEXT","tokenCount":5},{"modality":"IMAGE","tokenCount":1120}]`, 1, 15},
		{"missing_breakdown", "", 1, 1135},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			c.Set(ctxkey.ChannelRatio, 1.0)
			raw := `{"candidates":[{"content":{"parts":[{"text":"done"},{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}]}}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":1125,"thoughtsTokenCount":10,"totalTokenCount":1235` + tt.candidateParts + `}}`
			upstream := &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(raw)),
				Header:     make(http.Header),
			}
			apiErr, usageValue := gemini.Handler(c, upstream, 100, "gemini-nano-banana-2.1")
			require.Nil(t, apiErr)
			require.NotNil(t, usageValue)
			require.Equal(t, 1135, usageValue.CompletionTokens, "preserve upstream receipt and public usage")
			require.Equal(t, 1235, usageValue.TotalTokens)
			require.Equal(t, tt.imageCount, c.GetInt(ctxkey.OutputImageCount))

			requestMeta := &meta.Meta{
				ActualModelName: "gemini-nano-banana-2.1",
				ChannelType:     channeltype.VertextAI,
				APIType:         apitype.VertexAI,
				PromptTokens:    100,
			}
			applyOutputImageCharges(c, &usageValue, requestMeta)
			require.Positive(t, usageValue.ToolsCost)

			provider := &vertexai.Adaptor{}
			cfg := provider.GetDefaultModelPricing()[requestMeta.ActualModelName]
			input := quota.ComputeInput{
				Usage:          usageValue,
				ModelName:      requestMeta.ActualModelName,
				ModelRatio:     cfg.Ratio,
				PricingAdaptor: provider,
				GroupRatio:     1,
			}
			charged := quota.Compute(input)
			// Expected includes prompt + text + thinking and one render fee, but
			// not the IMAGE portion a second time at the text-output token rate.
			expectedUsage := &model.Usage{
				PromptTokens:     100,
				CompletionTokens: tt.billableTokens,
				ToolsCost:        usageValue.ToolsCost,
			}
			input.Usage = expectedUsage
			expected := quota.Compute(input)
			require.Equal(t, expected.TotalQuota, charged.TotalQuota)
			require.Equal(t, 1135, usageValue.CompletionTokens, "billing must not rewrite provider reported usage")
			require.Equal(t, 1235, usageValue.TotalTokens)
		})
	}
}
