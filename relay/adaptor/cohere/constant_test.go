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

	cfg, ok = ModelRatios["rerank-english-v3.0"]
	require.True(t, ok)
	require.InDelta(t, expected, cfg.Ratio, 1e-9)

	cfg, ok = ModelRatios["rerank-multilingual-v3.0"]
	require.True(t, ok)
	require.InDelta(t, expected, cfg.Ratio, 1e-9)
}
