package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
)

// TestConvertAdaptorVideoPricingKeepsRealtimeOutputRatio proves "Load
// Default" keeps a provider default's realtime video-output ratio, including
// a default that carries no per-second video generation price.
func TestConvertAdaptorVideoPricingKeepsRealtimeOutputRatio(t *testing.T) {
	local := convertAdaptorVideoPricing(&adaptor.VideoPricingConfig{OutputTokenRatio: 1.25})
	require.NotNil(t, local)
	require.InDelta(t, 1.25, local.OutputTokenRatio, 1e-12)
	require.Zero(t, local.PerSecondUsd)

	local = convertAdaptorVideoPricing(&adaptor.VideoPricingConfig{PerSecondUsd: 0.1, OutputTokenRatio: 2})
	require.InDelta(t, 0.1, local.PerSecondUsd, 1e-12)
	require.InDelta(t, 2, local.OutputTokenRatio, 1e-12)
	require.Nil(t, convertAdaptorVideoPricing(&adaptor.VideoPricingConfig{}))
	require.Nil(t, convertAdaptorVideoPricing(nil))
}
