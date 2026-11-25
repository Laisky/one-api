package adaptor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// fallbackFixtureAdaptor is an Adaptor whose provider catalog differs from the
// catalog it would contribute to the global fallback layer.
type fallbackFixtureAdaptor struct {
	Adaptor
	own, shared map[string]ModelConfig
}

// GetDefaultModelPricing returns the fixture's own channel catalog.
func (a fallbackFixtureAdaptor) GetDefaultModelPricing() map[string]ModelConfig { return a.own }

// contributingFixtureAdaptor additionally declares its global contribution.
type contributingFixtureAdaptor struct{ fallbackFixtureAdaptor }

// GetGlobalFallbackModelPricing returns the fixture's shared catalog.
func (a contributingFixtureAdaptor) GetGlobalFallbackModelPricing() map[string]ModelConfig {
	return a.shared
}

// TestGlobalFallbackPricingHonorsExplicitContribution verifies the global
// fallback catalog an adaptor contributes: nil for no adaptor, the provider
// catalog by default, and the declared shared catalog for a
// GlobalPricingContributor. Parameter t reports a wrong source.
func TestGlobalFallbackPricingHonorsExplicitContribution(t *testing.T) {
	t.Parallel()

	own := map[string]ModelConfig{"model": {Ratio: 2, TimeWindows: []TimeWindow{{Name: "native-only"}}}}
	shared := map[string]ModelConfig{"model": {Ratio: 1}}
	require.Nil(t, GlobalFallbackPricing(nil))
	require.Equal(t, own, GlobalFallbackPricing(fallbackFixtureAdaptor{own: own, shared: shared}))
	require.Equal(t, shared, GlobalFallbackPricing(contributingFixtureAdaptor{fallbackFixtureAdaptor{own: own, shared: shared}}))
}
