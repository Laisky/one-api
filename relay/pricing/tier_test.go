package pricing

import (
	"io"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// localMockAdaptor implements adaptor.Adaptor for tests
type localMockAdaptor struct {
	pricing map[string]adaptor.ModelConfig
}

func (m *localMockAdaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig { return m.pricing }
func (m *localMockAdaptor) GetModelRatio(modelName string) float64 {
	if p, ok := m.pricing[modelName]; ok {
		return p.Ratio
	}
	return 2.5 * billingratio.MilliTokensUsd
}
func (m *localMockAdaptor) GetCompletionRatio(modelName string) float64 {
	if p, ok := m.pricing[modelName]; ok {
		return p.CompletionRatio
	}
	return 1.0
}
func (m *localMockAdaptor) Init(meta *meta.Meta)                          {}
func (m *localMockAdaptor) GetRequestURL(meta *meta.Meta) (string, error) { return "", nil }
func (m *localMockAdaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	return nil
}
func (m *localMockAdaptor) ConvertRequest(c *gin.Context, relayMode int, request *relaymodel.GeneralOpenAIRequest) (any, error) {
	return nil, nil
}
func (m *localMockAdaptor) ConvertImageRequest(c *gin.Context, request *relaymodel.ImageRequest) (any, error) {
	return nil, nil
}
func (m *localMockAdaptor) ConvertClaudeRequest(c *gin.Context, request *relaymodel.ClaudeRequest) (any, error) {
	return nil, nil
}
func (m *localMockAdaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return nil, nil
}
func (m *localMockAdaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
	return nil, nil
}
func (m *localMockAdaptor) GetModelList() []string { return nil }
func (m *localMockAdaptor) GetChannelName() string { return "mock" }

func TestResolveEffectivePricing_BaseNoTiers(t *testing.T) {
	a := &localMockAdaptor{pricing: map[string]adaptor.ModelConfig{
		"m": {Ratio: 1.0, CompletionRatio: 2.0},
	}}

	eff := ResolveEffectivePricing("m", 10, a)
	require.Equal(t, 1.0, eff.InputRatio, "expected input ratio 1.0")
	require.Equal(t, 2.0, eff.OutputRatio, "expected output ratio 2.0")
	require.Equal(t, 0, eff.AppliedTierThreshold, "expected base tier threshold 0")
}

func TestResolveEffectivePricing_TierSelection(t *testing.T) {
	a := &localMockAdaptor{pricing: map[string]adaptor.ModelConfig{
		"m": {
			Ratio:            1.0,
			CompletionRatio:  2.0,
			CachedInputRatio: 0.4,
			Tiers: []adaptor.ModelRatioTier{
				{InputTokenThreshold: 1000, Ratio: 0.5, CompletionRatio: 3.0},
				{InputTokenThreshold: 5000, Ratio: 0.2},
			},
		},
	}}

	// Select first tier (>=1000)
	eff := ResolveEffectivePricing("m", 1500, a)
	require.Equal(t, 0.5, eff.InputRatio, "unexpected input ratio")
	require.Equal(t, 1.5, eff.OutputRatio, "unexpected output ratio")
	require.Equal(t, 1000, eff.AppliedTierThreshold, "expected threshold 1000")

	// Select second tier (>=5000)
	eff = ResolveEffectivePricing("m", 6000, a)
	require.Equal(t, 0.2, eff.InputRatio, "expected input ratio 0.2")
	// Expect inherited completion ratio 3.0 from first tier since second does not set it
	require.InDelta(t, 0.6, eff.OutputRatio, 1e-8, "expected output ratio 0.6 (0.2*3.0)")
}

// TestResolveEffectivePricing_OutputTierSelection verifies that output-only and
// combined token thresholds are selected from raw usage counts.
func TestResolveEffectivePricing_OutputTierSelection(t *testing.T) {
	base := adaptor.ModelConfig{
		Ratio:            2.0,
		CompletionRatio:  4.0,
		CachedInputRatio: 0.4,
		Tiers: []adaptor.ModelRatioTier{
			{OutputTokenThreshold: 200, Ratio: 3.0, CompletionRatio: 14.0 / 3.0, CachedInputRatio: 0.6},
			{InputTokenThreshold: 32_000, Ratio: 4.0, CompletionRatio: 4.0, CachedInputRatio: 0.8},
		},
	}

	short := ResolveEffectivePricingForUsageFromConfig(1_000, 199, base)
	require.InDelta(t, 2.0, short.InputRatio, 1e-9)
	require.InDelta(t, 8.0, short.OutputRatio, 1e-9)
	require.Zero(t, short.AppliedOutputTierThreshold)

	longOutput := ResolveEffectivePricingForUsageFromConfig(1_000, 200, base)
	require.InDelta(t, 3.0, longOutput.InputRatio, 1e-9)
	require.InDelta(t, 14.0, longOutput.OutputRatio, 1e-9)
	require.Equal(t, 200, longOutput.AppliedOutputTierThreshold)

	longInput := ResolveEffectivePricingForUsageFromConfig(32_000, 50, base)
	require.InDelta(t, 4.0, longInput.InputRatio, 1e-9)
	require.InDelta(t, 16.0, longInput.OutputRatio, 1e-9)
	require.Equal(t, 32_000, longInput.AppliedTierThreshold)
}

func TestResolveEffectivePricing_CachedNegativeFree(t *testing.T) {
	a := &localMockAdaptor{pricing: map[string]adaptor.ModelConfig{
		"m": {
			Ratio:            1.0,
			CompletionRatio:  2.0,
			CachedInputRatio: -1, // free cached input
		},
	}}

	eff := ResolveEffectivePricing("m", 10, a)
	require.Less(t, eff.CachedInputRatio, 0.0, "expected negative cached input (free)")
	// No cached output pricing; nothing to assert here
}

// New: ensure output price remains input*completion regardless of cached input and across tier transitions
func TestResolveEffectivePricing_TierTransitionCacheIndependence(t *testing.T) {
	a := &localMockAdaptor{pricing: map[string]adaptor.ModelConfig{
		"tm": {
			Ratio:            1.0,
			CompletionRatio:  2.0,
			CachedInputRatio: 0.5,
			Tiers: []adaptor.ModelRatioTier{
				{InputTokenThreshold: 1000, Ratio: 0.8},
				{InputTokenThreshold: 5000, Ratio: 0.6},
			},
		},
	}}

	eff := ResolveEffectivePricing("tm", 6000, a) // tier2
	require.Equal(t, 0.6, eff.InputRatio, "expected tier2 input ratio 0.6")
	require.InDelta(t, 1.2, eff.OutputRatio, 1e-8, "expected output ratio 1.2 (0.6 * 2.0)")

	// Cached input ratio should not affect output ratio
	require.InDelta(t, 0.5, eff.CachedInputRatio, 1e-9, "expected cached input 0.5")
}
