package pricing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
)

// operatorTariffMockAdaptor declares every model in operatorOnly as payable only
// through an operator tariff while still publishing provider metadata for it.
type operatorTariffMockAdaptor struct {
	MockAdaptor
	operatorOnly map[string]bool
}

// RequiresOperatorTariff implements adaptor.OperatorTariffPolicy for tests.
func (m *operatorTariffMockAdaptor) RequiresOperatorTariff(modelName string) bool {
	return m.operatorOnly[modelName]
}

// TestOperatorTariffPolicyGatesEveryTariffLayer proves each resolver skips the
// provider and global tariffs for a declared model, keeps the operator layer
// authoritative, and still exposes non-tariff bounds through ResolveModelMetadata.
func TestOperatorTariffPolicyGatesEveryTariffLayer(t *testing.T) {
	const name = "host-unverified-model"
	borrowed := adaptor.ModelConfig{
		Ratio: 20 * billingratio.MilliTokensUsd, CompletionRatio: 5,
		MaxOutputTokens: 128_000, ContextLength: 1_050_000,
		Audio: &adaptor.AudioPricingConfig{PromptRatio: 2, CompletionRatio: 3},
		Image: &adaptor.ImagePricingConfig{PricePerImageUsd: 0.1},
		Video: &adaptor.VideoPricingConfig{PerSecondUsd: 0.2},
	}
	setTestGlobalModelConfigs(t, map[string]adaptor.ModelConfig{name: borrowed})
	at := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		label    string
		provider adaptor.Adaptor
	}{
		{"global_only", &operatorTariffMockAdaptor{MockAdaptor: MockAdaptor{pricing: map[string]adaptor.ModelConfig{}}, operatorOnly: map[string]bool{name: true}}},
		{"provider_default", &operatorTariffMockAdaptor{MockAdaptor: MockAdaptor{pricing: map[string]adaptor.ModelConfig{name: borrowed}}, operatorOnly: map[string]bool{name: true}}},
	} {
		t.Run(tc.label, func(t *testing.T) {
			_, found := ResolveModelConfig(name, nil, tc.provider, at)
			require.False(t, found)
			_, found = ResolveModelConfigRatioOnly(name, nil, tc.provider, at)
			require.False(t, found)
			_, found = ResolveAudioPricing(name, nil, tc.provider, at)
			require.False(t, found)
			_, found = ResolveImagePricing(name, nil, tc.provider, at)
			require.False(t, found)
			require.Nil(t, GetVideoPricingWithThreeLayers(name, nil, tc.provider))
			require.Equal(t, 2.5*billingratio.MilliTokensUsd, GetModelRatioWithThreeLayers(name, nil, tc.provider))
			require.Equal(t, 1.0, GetCompletionRatioWithThreeLayers(name, nil, tc.provider))
			require.Equal(t, 2.5*billingratio.MilliTokensUsd, ResolveModelRatioAt(name, nil, nil, tc.provider, at))

			// Published limits stay available as conservative admission bounds.
			metadata, found := ResolveModelMetadata(name, tc.provider, at)
			require.True(t, found)
			require.EqualValues(t, 128_000, metadata.MaxOutputTokens)
			require.EqualValues(t, 1_050_000, metadata.ContextLength)

			// The operator's tariff remains the first and only price.
			operator := map[string]model.ModelConfigLocal{name: {Ratio: 3 * billingratio.MilliTokensUsd, CompletionRatio: 2}}
			cfg, found := ResolveModelConfigRatioOnly(name, operator, tc.provider, at)
			require.True(t, found)
			require.InDelta(t, 3*billingratio.MilliTokensUsd, cfg.Ratio, 1e-12)
			require.InDelta(t, 3*billingratio.MilliTokensUsd, ResolveModelRatioAt(name, operator, nil, tc.provider, at), 1e-12)
			require.InDelta(t, 2, ResolveCompletionRatioAt(name, operator, nil, tc.provider, at), 1e-12)
		})
	}

	// Control: an undeclared provider keeps the historical three-layer fallback.
	plain := &MockAdaptor{pricing: map[string]adaptor.ModelConfig{}}
	cfg, found := ResolveModelConfigRatioOnly(name, nil, plain, at)
	require.True(t, found)
	require.Equal(t, borrowed.Ratio, cfg.Ratio)
}
