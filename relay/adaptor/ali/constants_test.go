package ali

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestQwenTokenModelsDoNotUseZeroRatio verifies supported token-billed Qwen
// models never resolve to a zero internal ratio. Parameters: t is the active
// test handle. Return values: none; the test fails on vulnerable zero pricing.
func TestQwenTokenModelsDoNotUseZeroRatio(t *testing.T) {
	models := []string{
		"qwen-audio-turbo",
		"qwen-audio-chat",
		"qwen2-audio-instruct",
		"qwen2.5-1.5b-instruct",
		"qwen2.5-0.5b-instruct",
		"qwen2.5-math-1.5b-instruct",
	}

	for _, model := range models {
		cfg, ok := ModelRatios[model]
		require.True(t, ok, "model must remain registered: %s", model)
		require.Positive(t, cfg.Ratio, "model must consume quota: %s", model)
	}
}
