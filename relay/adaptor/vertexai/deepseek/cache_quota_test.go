package deepseek_test

import (
	"io"
	"math"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	deepseek "github.com/Laisky/one-api/relay/adaptor/vertexai/deepseek"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	quotautil "github.com/Laisky/one-api/relay/quota"
)

// stubPricingAdaptor exposes a fixed pricing map as an adaptor.Adaptor so quota
// billing can be exercised against the Vertex DeepSeek ModelRatios directly.
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
func (s *stubPricingAdaptor) GetChannelName() string { return "vertex-deepseek-stub" }
func (s *stubPricingAdaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return s.pricing
}
func (s *stubPricingAdaptor) GetModelRatio(modelName string) float64 {
	return s.pricing[modelName].Ratio
}
func (s *stubPricingAdaptor) GetCompletionRatio(modelName string) float64 {
	return s.pricing[modelName].CompletionRatio
}

// TestVertexDeepSeekCachedInputBillsAtDiscount verifies that the cache-supported
// Vertex DeepSeek MaaS models carry a CachedInputRatio equal to 0.1x the input
// ratio (Vertex implicit caching: 90% discount on cached tokens, per
// https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/maas/use-open-models)
// and that quota.Compute reprices the cached portion accordingly. Only
// deepseek-v3.1-maas and deepseek-v3.2-maas are listed as implicit-cache supported.
func TestVertexDeepSeekCachedInputBillsAtDiscount(t *testing.T) {
	t.Parallel()

	provider := &stubPricingAdaptor{pricing: deepseek.ModelRatios}

	for _, modelName := range []string{"deepseek-ai/deepseek-v3.1-maas", "deepseek-ai/deepseek-v3.2-maas"} {
		modelName := modelName
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()

			modelRatio := provider.GetModelRatio(modelName)
			require.Greater(t, modelRatio, 0.0, "unexpected model ratio for %s", modelName)
			groupRatio := 1.0

			const promptTokens = 50_000
			const completionTokens = 1_000
			const cachedPrompt = 30_000

			eff := pricing.ResolveEffectivePricing(modelName, promptTokens, provider)
			require.Greater(t, eff.CachedInputRatio, 0.0,
				"%s must define a positive cached input ratio", modelName)
			require.InDelta(t, 0.1*modelRatio, eff.CachedInputRatio, modelRatio*1e-9,
				"%s cached input ratio should be 0.1x the input ratio", modelName)

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
