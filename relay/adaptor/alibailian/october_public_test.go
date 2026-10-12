package alibailian_test

import (
	"encoding/json"
	"github.com/Laisky/one-api/relay/adaptor/ali"
	"github.com/Laisky/one-api/relay/adaptor/alibailian"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

// TestOctoberBailianPublishedRates verifies current CNY metadata and discovery,
// including removal of unsupported cache rates. It accepts t and returns nothing.
func TestOctoberBailianPublishedRates(t *testing.T) {
	t.Parallel()
	a := &openai.Adaptor{ChannelType: channeltype.AliBailian}
	for _, tc := range []struct {
		id                 string
		input, output      float64
		context, maxOutput int32
	}{
		{"qwen-mt-plus", 1.8, 5.4, 16384, 8192},
		{"qwen-vl-ocr", .3, .5, 38192, 8192},
		{"qwen-vl-ocr-2025-11-20", .3, .5, 38192, 8192},
		{"qwen3.5-ocr", .5, 2, 65536, 16384},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			cfg, exists := a.GetDefaultModelPricing()[tc.id]
			require.True(t, exists)
			require.Contains(t, a.GetModelList(), tc.id)
			require.Contains(t, alibailian.ModelList, tc.id)
			require.InDelta(t, tc.input, cfg.Ratio/ratio.MilliTokensRmb, 1e-12)
			require.InDelta(t, tc.output, cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensRmb, 1e-12)
			require.Zero(t, cfg.CachedInputRatio)
			require.Equal(t, tc.context, cfg.ContextLength)
			require.Equal(t, tc.maxOutput, cfg.MaxOutputTokens)
			require.Empty(t, cfg.SupportedFeatures)
			require.Empty(t, cfg.SupportedReasoningEfforts)
			require.Empty(t, cfg.Quantization)
			require.Empty(t, cfg.Tiers)
			require.Zero(t, cfg.MaxTokens)
		})
	}
	require.NotContains(t, ali.ModelRatios, "qwen3.5-ocr", "native Ali cannot silently inherit compatible image support")
	require.NotContains(t, alibailian.ModelRatios, "qwen-mt-flash", "required translation_options are not implemented")
}

// TestOctoberBailianOCRImagePreservation checks actual compatible request
// conversion preserves image/text parts and exact IDs using t. It returns nothing.
func TestOctoberBailianOCRImagePreservation(t *testing.T) {
	t.Parallel()
	const imageURL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aO4sAAAAASUVORK5CYII="
	for _, id := range []string{"qwen3.5-ocr", "qwen-vl-ocr-2025-11-20"} {
		cfg, exists := alibailian.ModelRatios[id]
		require.True(t, exists)
		var req model.GeneralOpenAIRequest
		body := `{"model":"` + id + `","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"` + imageURL + `"}},{"type":"text","text":"Read all text."}]}],"max_completion_tokens":2048}`
		require.NoError(t, json.Unmarshal([]byte(body), &req))
		before, err := json.Marshal(&req)
		require.NoError(t, err)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
		meta.Set2Context(c, &meta.Meta{ChannelType: channeltype.AliBailian, ActualModelName: id, Mode: relaymode.ChatCompletions})
		converted, err := (&openai.Adaptor{ChannelType: channeltype.AliBailian}).ConvertRequest(c, relaymode.ChatCompletions, &req)
		require.NoError(t, err)
		after, err := json.Marshal(converted)
		require.NoError(t, err)
		require.JSONEq(t, string(before), string(after))
		require.Equal(t, []string{"text", "image"}, cfg.InputModalities)
		clone := cfg.Clone()
		clone.InputModalities[0] = "changed"
		require.Equal(t, "text", cfg.InputModalities[0])
	}
}
