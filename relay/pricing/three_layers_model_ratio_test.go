package pricing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/adaptor"
)

// TestGetModelRatioWithThreeLayers_AllLayers verifies model-ratio precedence and corner cases.
// Parameters: t is the current test handle.
// Returns: nothing; assertions fail the test on mismatch.
func TestGetModelRatioWithThreeLayers_AllLayers(t *testing.T) {
	const modelName = "three-layer-model"
	setTestGlobalModelConfigs(t, map[string]adaptor.ModelConfig{
		modelName: {Ratio: 0.11},
	})

	provider := &MockAdaptor{
		name: "provider",
		pricing: map[string]adaptor.ModelConfig{
			modelName: {Ratio: 0.22},
		},
	}

	channelOverrides := map[string]float64{modelName: 0.33}
	require.InDelta(t, 0.33, GetModelRatioWithThreeLayers(modelName, channelOverrides, provider), 1e-12)
	require.InDelta(t, 0.22, GetModelRatioWithThreeLayers(modelName, nil, provider), 1e-12)
	require.InDelta(t, 0.11, GetModelRatioWithThreeLayers(modelName, nil, &MockAdaptor{name: "empty", pricing: map[string]adaptor.ModelConfig{}}), 1e-12)

	// Explicit zero override must be respected.
	channelZero := map[string]float64{modelName: 0}
	require.InDelta(t, 0.0, GetModelRatioWithThreeLayers(modelName, channelZero, provider), 1e-12)

	// Unknown model falls back to built-in default.
	require.InDelta(t, 2.5*0.000001, GetModelRatioWithThreeLayers("unknown-model", nil, nil), 1e-12)
}

// TestGetCompletionRatioWithThreeLayers_AllLayers verifies completion-ratio precedence and corner cases.
// Parameters: t is the current test handle.
// Returns: nothing; assertions fail the test on mismatch.
func TestGetCompletionRatioWithThreeLayers_AllLayers(t *testing.T) {
	const modelName = "three-layer-model"
	setTestGlobalModelConfigs(t, map[string]adaptor.ModelConfig{
		modelName: {CompletionRatio: 1.1},
	})

	provider := &MockAdaptor{
		name: "provider",
		pricing: map[string]adaptor.ModelConfig{
			modelName: {CompletionRatio: 2.2},
		},
	}

	channelOverrides := map[string]float64{modelName: 3.3}
	require.InDelta(t, 3.3, GetCompletionRatioWithThreeLayers(modelName, channelOverrides, provider), 1e-12)
	require.InDelta(t, 2.2, GetCompletionRatioWithThreeLayers(modelName, nil, provider), 1e-12)
	require.InDelta(t, 1.1, GetCompletionRatioWithThreeLayers(modelName, nil, &MockAdaptor{name: "empty", pricing: map[string]adaptor.ModelConfig{}}), 1e-12)

	// Explicit zero override must be respected.
	channelZero := map[string]float64{modelName: 0}
	require.InDelta(t, 0.0, GetCompletionRatioWithThreeLayers(modelName, channelZero, provider), 1e-12)

	// Unknown model falls back to built-in default.
	require.InDelta(t, 1.0, GetCompletionRatioWithThreeLayers("unknown-model", nil, nil), 1e-12)
}
