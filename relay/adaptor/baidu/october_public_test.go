package baidu_test

import (
	"github.com/Laisky/one-api/relay/adaptor/baidu"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestOctoberBaiduEndpointLifecycle verifies every retained v1 ID and its
// historical tariff through public methods using t. It returns no values.
func TestOctoberBaiduEndpointLifecycle(t *testing.T) {
	t.Parallel()
	a := &baidu.Adaptor{}
	prices := map[string]float64{
		"ERNIE-4.0-8K": 12, "ERNIE-3.5-8K": 1.2, "ERNIE-3.5-8K-0205": 1.2,
		"ERNIE-3.5-8K-1222": 1.2, "ERNIE-Bot-8K": 1.2, "ERNIE-3.5-4K-0205": 1.2,
		"ERNIE-Speed-8K": .4, "ERNIE-Speed-128K": .4, "ERNIE-Lite-8K-0922": .8,
		"ERNIE-Lite-8K-0308": .8, "ERNIE-Tiny-8K": .4, "BLOOMZ-7B": .4,
		"Embedding-V1": .2, "bge-large-zh": .2, "bge-large-en": .2, "tao-8k": .8,
	}
	require.Len(t, a.GetDefaultModelPricing(), len(prices))
	for id, price := range prices {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			cfg, ok := a.GetDefaultModelPricing()[id]
			require.True(t, ok)
			require.Contains(t, a.GetModelList(), id)
			require.Contains(t, cfg.Description, "v1 endpoint retired on 2026-08-31")
			require.Contains(t, cfg.Description, "does not imply that all corresponding models are retired on v2")
			require.InDelta(t, price, a.GetModelRatio(id)/ratio.MilliTokensRmb, 1e-12)
			require.EqualValues(t, 1, a.GetCompletionRatio(id))
		})
	}
}
