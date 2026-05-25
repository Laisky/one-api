package cohere

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

func TestRerankModelPricing(t *testing.T) {
	t.Parallel()

	expected := (2.0 / 1000.0) * ratio.QuotaPerUsd

	cfg, ok := ModelRatios["rerank-v3.5"]
	require.True(t, ok)
	require.InDelta(t, expected, cfg.Ratio, 1e-9)
	require.NotNil(t, cfg.PerCall, "rerank-v3.5 must carry PerCallPricingConfig so the display layer surfaces per-call pricing")
	require.InDelta(t, 2.0, cfg.PerCall.UsdPerThousandCalls, 1e-9)

	cfg, ok = ModelRatios["rerank-english-v3.0"]
	require.True(t, ok)
	require.InDelta(t, expected, cfg.Ratio, 1e-9)
	require.NotNil(t, cfg.PerCall)
	require.InDelta(t, 2.0, cfg.PerCall.UsdPerThousandCalls, 1e-9)

	cfg, ok = ModelRatios["rerank-multilingual-v3.0"]
	require.True(t, ok)
	require.InDelta(t, expected, cfg.Ratio, 1e-9)
	require.NotNil(t, cfg.PerCall)
	require.InDelta(t, 2.0, cfg.PerCall.UsdPerThousandCalls, 1e-9)

	// v4.0 family prices: pro at $2.50/1K, fast at $2.00/1K.
	cfg, ok = ModelRatios["rerank-v4.0-pro"]
	require.True(t, ok)
	require.InDelta(t, (2.5/1000.0)*ratio.QuotaPerUsd, cfg.Ratio, 1e-9)
	require.NotNil(t, cfg.PerCall)
	require.InDelta(t, 2.5, cfg.PerCall.UsdPerThousandCalls, 1e-9)

	cfg, ok = ModelRatios["rerank-v4.0-fast"]
	require.True(t, ok)
	require.InDelta(t, expected, cfg.Ratio, 1e-9)
	require.NotNil(t, cfg.PerCall)
	require.InDelta(t, 2.0, cfg.PerCall.UsdPerThousandCalls, 1e-9)
}
