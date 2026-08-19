package stepfun_test

import (
	"io"
	"math"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/stepfun"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	quotautil "github.com/Laisky/one-api/relay/quota"
)

// stubPricingAdaptor exposes a fixed pricing map as an adaptor.Adaptor so quota
// billing can be exercised against stepfun's published ModelRatios directly.
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
func (s *stubPricingAdaptor) GetChannelName() string { return "stepfun-stub" }
func (s *stubPricingAdaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return s.pricing
}
func (s *stubPricingAdaptor) GetModelRatio(modelName string) float64 {
	return s.pricing[modelName].Ratio
}
func (s *stubPricingAdaptor) GetCompletionRatio(modelName string) float64 {
	return s.pricing[modelName].CompletionRatio
}

// TestStepFunCachedInputBillsAtDiscount verifies that StepFun's cache-supported
// models carry a CachedInputRatio equal to 0.2x the input ratio (StepFun Prompt
// Cache: cached portion billed at 20% of the model's input price, per
// https://platform.stepfun.com/docs/zh/guides/developer/prompt-cache) and that
// quota.Compute reprices the cached portion accordingly.
func TestStepFunCachedInputBillsAtDiscount(t *testing.T) {
	t.Parallel()

	provider := &stubPricingAdaptor{pricing: stepfun.ModelRatios}

	// Only models documented as cache-supported. step-3.5-flash and
	// step-1o-turbo-vision are the two such models present in ModelRatios.
	for _, modelName := range []string{"step-3.5-flash", "step-1o-turbo-vision"} {
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

			// StepFun reports cached tokens at the TOP LEVEL of usage; the shared
			// NormalizeCachedTokens promotion (exercised in the openai_compatible
			// handler tests) lands the value in PromptTokensDetails.CachedTokens,
			// which is what quota.Compute consumes here.
			cachedUsage := &relaymodel.Usage{
				PromptTokens:     promptTokens,
				CompletionTokens: completionTokens,
				CachedTokens:     cachedPrompt,
			}
			cachedUsage.NormalizeCachedTokens()
			require.NotNil(t, cachedUsage.PromptTokensDetails)
			require.Equal(t, cachedPrompt, cachedUsage.PromptTokensDetails.CachedTokens)

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
