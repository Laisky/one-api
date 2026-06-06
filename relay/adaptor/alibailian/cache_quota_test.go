package alibailian_test

import (
	"io"
	"math"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/alibailian"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	quotautil "github.com/Laisky/one-api/relay/quota"
)

// stubPricingAdaptor exposes a fixed pricing map as an adaptor.Adaptor so quota
// billing can be exercised against alibailian's published ModelRatios directly.
// Alibailian has no dedicated relay adaptor (it is served through the
// OpenAI-compatible path), so the constants map is the authoritative source
// under test here.
type stubPricingAdaptor struct {
	pricing map[string]adaptor.ModelConfig
}

func (s *stubPricingAdaptor) Init(*metalib.Meta) {}
func (s *stubPricingAdaptor) GetRequestURL(*metalib.Meta) (string, error) {
	return "", nil
}
func (s *stubPricingAdaptor) SetupRequestHeader(*gin.Context, *http.Request, *metalib.Meta) error {
	return nil
}
func (s *stubPricingAdaptor) ConvertRequest(*gin.Context, int, *relaymodel.GeneralOpenAIRequest) (any, error) {
	return nil, nil
}
func (s *stubPricingAdaptor) ConvertImageRequest(*gin.Context, *relaymodel.ImageRequest) (any, error) {
	return nil, nil
}
func (s *stubPricingAdaptor) ConvertClaudeRequest(*gin.Context, *relaymodel.ClaudeRequest) (any, error) {
	return nil, nil
}
func (s *stubPricingAdaptor) DoRequest(*gin.Context, *metalib.Meta, io.Reader) (*http.Response, error) {
	return nil, nil
}
func (s *stubPricingAdaptor) DoResponse(*gin.Context, *http.Response, *metalib.Meta) (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
	return nil, nil
}
func (s *stubPricingAdaptor) GetModelList() []string { return nil }
func (s *stubPricingAdaptor) GetChannelName() string { return "alibailian-stub" }
func (s *stubPricingAdaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return s.pricing
}
func (s *stubPricingAdaptor) GetModelRatio(modelName string) float64 {
	return s.pricing[modelName].Ratio
}
func (s *stubPricingAdaptor) GetCompletionRatio(modelName string) float64 {
	return s.pricing[modelName].CompletionRatio
}

// TestAlibailianQwenCachedInputBillsAtDiscount verifies that alibailian's Qwen
// models carry a CachedInputRatio equal to 0.2x the input ratio (Alibaba Bailian
// implicit context cache: cache-hit input billed at 20% of standard input price,
// per https://help.aliyun.com/zh/model-studio/context-cache) and that
// quota.Compute reprices the cached portion accordingly rather than at full input.
func TestAlibailianQwenCachedInputBillsAtDiscount(t *testing.T) {
	t.Parallel()

	provider := &stubPricingAdaptor{pricing: alibailian.ModelRatios}

	// Cover a representative spread: base flagship, a tiered model, a coder, and a
	// model that exists only on Bailian (not the DashScope-native ali map).
	for _, modelName := range []string{"qwen-plus", "qwen3-max", "qwen3.5-plus", "qwen3-coder-plus", "qwen-vl-max"} {
		modelName := modelName
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()

			modelRatio := provider.GetModelRatio(modelName)
			require.Greater(t, modelRatio, 0.0, "unexpected model ratio for %s", modelName)
			groupRatio := 1.0

			const promptTokens = 100_000
			const completionTokens = 1_000
			const cachedPrompt = 60_000

			eff := pricing.ResolveEffectivePricing(modelName, promptTokens, provider)
			require.Greater(t, eff.CachedInputRatio, 0.0,
				"%s must define a positive cached input ratio", modelName)
			require.InDelta(t, 0.2*modelRatio, eff.CachedInputRatio, modelRatio*1e-9,
				"%s cached input ratio should be 0.2x the input ratio", modelName)

			baseUsage := &relaymodel.Usage{PromptTokens: promptTokens, CompletionTokens: completionTokens}
			base := quotautil.Compute(quotautil.ComputeInput{
				Usage:          baseUsage,
				ModelName:      modelName,
				ModelRatio:     modelRatio,
				GroupRatio:     groupRatio,
				PricingAdaptor: provider,
			})

			cachedUsage := &relaymodel.Usage{
				PromptTokens:     promptTokens,
				CompletionTokens: completionTokens,
				PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{
					CachedTokens: cachedPrompt,
				},
			}
			cached := quotautil.Compute(quotautil.ComputeInput{
				Usage:          cachedUsage,
				ModelName:      modelName,
				ModelRatio:     modelRatio,
				GroupRatio:     groupRatio,
				PricingAdaptor: provider,
			})

			require.Equal(t, cachedPrompt, cached.CachedPromptTokens)
			require.Less(t, cached.TotalQuota, base.TotalQuota,
				"cache hits must reduce quota for %s", modelName)

			normalInputPrice := base.UsedModelRatio * groupRatio
			cachedInputPrice := eff.CachedInputRatio * groupRatio
			expectedDelta := int64(math.Ceil(float64(cachedPrompt) * (cachedInputPrice - normalInputPrice)))
			actualDelta := cached.TotalQuota - base.TotalQuota
			require.InDelta(t, expectedDelta, actualDelta, 2,
				"unexpected quota delta for %s: got %d want ~%d (base=%d cached=%d)",
				modelName, actualDelta, expectedDelta, base.TotalQuota, cached.TotalQuota)
		})
	}
}
