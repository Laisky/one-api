package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
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

// TestGeminiStreamImageQuotaFits verifies the real relay accepts an affordable separately billed image receipt.
func TestGeminiStreamImageQuotaFits(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(fmt.Sprint(fallback), func(t *testing.T) {
			const balance int64 = 30000
			xaiVideoSetup(t, balance, false)
			oldPre := config.PreConsumedQuota
			config.PreConsumedQuota = 20
			t.Cleanup(func() { config.PreConsumedQuota = oldPre })
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, `data: {"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"YQ=="}}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1120,"totalTokenCount":1121,"candidatesTokensDetails":[{"modality":"IMAGE","tokenCount":1120}]}}`+"\n\ndata: [DONE]\n\n")
			}))
			t.Cleanup(server.Close)
			oldClient := client.HTTPClient
			client.HTTPClient = server.Client()
			t.Cleanup(func() { client.HTTPClient = oldClient })
			path, body := "/v1/chat/completions", `{"model":"alias","messages":[{"role":"user","content":"draw"}],"max_tokens":1,"stream":true}`
			if fallback {
				path, body = "/v1/responses", `{"model":"alias","input":"draw","max_output_tokens":1,"stream":true}`
			}
			c, w, _ := protocolContext(t, channeltype.Gemini, "gemini-nano-banana-2.1", path, body, server.URL, balance, 1, false, &dbmodel.ModelConfigLocal{Ratio: 100, CompletionRatio: 1})
			var err *model.ErrorWithStatusCode
			if fallback {
				err = RelayResponseAPIHelper(c)
			} else {
				err = RelayTextHelper(c)
			}
			drainCriticalTasks(t)
			require.Nil(t, err, "an affordable image must not be rejected by token-only finalization")
			require.Contains(t, w.Body.String(), "YQ==")
			expected := quota.Compute(quota.ComputeInput{
				Usage:     &model.Usage{PromptTokens: 1, CompletionTokens: 1120, ImageTokensBilledSeparately: 1120, ToolsCost: calculateImageBaseQuota(0.0336, 0, 1, 1, 1)},
				ModelName: "gemini-nano-banana-2.1", ModelRatio: 100, GroupRatio: 1,
				ChannelModelConfigs: map[string]dbmodel.ModelConfigLocal{"gemini-nano-banana-2.1": {Ratio: 100, CompletionRatio: 1}},
			})
			require.Equal(t, balance-expected.TotalQuota, reloadUserQuota(t))
		})
	}
}
