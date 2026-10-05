package pricing

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
)

// TestMediaTariffProvenance rejects unresolved/expired contracts and preserves
// explicit free promotion and administrator overrides. Parameters: t owns the
// test. Returns: none; a source snapshot is not an invented promotion end date.
func TestMediaTariffProvenance(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	for _, state := range []string{"free", "promotional_free", "paid", "unknown", "contract", "unrecognized"} {
		t.Run(state, func(t *testing.T) {
			cfg := adaptor.ModelConfig{PricingProvenance: &adaptor.PricingProvenance{State: state, Unit: "generation", Source: "https://example.test/tariff", VerifiedAt: "2026-10-04"}, PerCall: &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 40}}
			if state == "free" || state == "promotional_free" {
				cfg.PerCall.UsdPerThousandCalls = 0
			}
			err := ValidateTariffProvenance(cfg, at)
			if state == "free" || state == "promotional_free" || state == "paid" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			cfg.PricingProvenance.ValidUntil = at
			require.Error(t, ValidateTariffProvenance(cfg, at))
			provider := &MockAdaptor{pricing: map[string]adaptor.ModelConfig{"music": cfg}}
			_, applies, err := ResolveGenerationTariff("music", nil, provider, at)
			require.True(t, applies)
			require.Error(t, err)
			free, applies, err := ResolveGenerationTariff("music", map[string]model.ModelConfigLocal{"music": {PerCall: &model.PerCallPricingLocal{}}}, provider, at)
			require.True(t, applies)
			require.NoError(t, err)
			require.Zero(t, free.UsdPerThousandCalls)
		})
	}
}

// TestResolveGenerationTariffChannelOverrides checks how persisted channel
// overrides interact with a catalog generation contract. Parameters: t owns the
// assertions. Returns: none; zero ratios are "unset", never an implicit free price.
func TestResolveGenerationTariffChannelOverrides(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	catalog := adaptor.ModelConfig{CompletionRatio: 1, PerCall: &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 40},
		PricingProvenance: &adaptor.PricingProvenance{State: "paid", Unit: "generation", Source: "https://example.test/tariff", VerifiedAt: "2026-10-05"}}
	provider := &MockAdaptor{pricing: map[string]adaptor.ModelConfig{"music": catalog}}

	snapshot, applies, err := ResolveGenerationTariff("music", map[string]model.ModelConfigLocal{"music": {CompletionRatio: 1, MaxTokens: 128}}, provider, at)
	require.True(t, applies)
	require.NoError(t, err)
	require.InDelta(t, 40, snapshot.UsdPerThousandCalls, 1e-12, "a metadata-only override inherits the catalog tariff")

	_, applies, err = ResolveGenerationTariff("music", map[string]model.ModelConfigLocal{"music": {Ratio: 2, CompletionRatio: 1}}, provider, at)
	require.True(t, applies)
	require.Error(t, err, "a token ratio cannot price a generation")

	free, applies, err := ResolveGenerationTariff("music", map[string]model.ModelConfigLocal{"music": {PerCall: &model.PerCallPricingLocal{}}}, provider, at)
	require.True(t, applies)
	require.NoError(t, err)
	require.Zero(t, free.UsdPerThousandCalls, "an explicit per_call zero remains an operator free tariff")

	expired := catalog.Clone()
	expired.PricingProvenance.ValidUntil = at
	expiredProvider := &MockAdaptor{pricing: map[string]adaptor.ModelConfig{"music": expired}}
	_, applies, err = ResolveGenerationTariff("music", map[string]model.ModelConfigLocal{"music": {CompletionRatio: 1}}, expiredProvider, at)
	require.True(t, applies)
	require.Error(t, err, "inheritance must not revive an expired catalog contract")

	_, applies, err = ResolveGenerationTariff("chat", map[string]model.ModelConfigLocal{"chat": {CompletionRatio: 1}}, provider, at)
	require.False(t, applies, "models without a generation contract keep token billing")
	require.NoError(t, err)
}

// TestGenerationQuotaExactUnits checks generation denomination, one-time group
// multiplication and invalid/overflowing values. Parameters: t owns assertions.
// Returns: none; tests use tariff fixtures and do not contact a provider.
func TestGenerationQuotaExactUnits(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		price, group float64
		want         int64
	}{{40, 1, 20000}, {80, 1, 40000}, {40, 1.5, 30000}, {40, 0, 0}, {0, 1, 0}, {0.0001, 1, 1}} {
		amount, err := GenerationQuota(&adaptor.PerCallPricingConfig{UsdPerThousandCalls: tc.price}, tc.group)
		require.NoError(t, err)
		require.Equal(t, tc.want, amount)
	}
	for _, bad := range []float64{-1, math.NaN(), math.Inf(1), math.MaxFloat64} {
		_, err := GenerationQuota(&adaptor.PerCallPricingConfig{UsdPerThousandCalls: bad}, 1)
		require.Error(t, err)
	}
}
