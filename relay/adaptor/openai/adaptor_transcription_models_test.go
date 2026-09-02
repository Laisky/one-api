package openai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestCurrentTranscriptionModels verifies the current duration-priced transcription catalog entries.
func TestCurrentTranscriptionModels(t *testing.T) {
	t.Parallel()

	modelList := adaptor.GetModelListFromPricing(ModelRatios)
	modelSet := make(map[string]bool, len(modelList))
	for _, modelName := range modelList {
		modelSet[modelName] = true
	}

	fileModel, ok := ModelRatios["gpt-transcribe"]
	require.True(t, ok, "ModelRatios must contain gpt-transcribe")
	assert.True(t, modelSet["gpt-transcribe"])
	assert.InDelta(t, 7.5*ratio.MilliTokensUsd, fileModel.Ratio, 1e-12)
	require.NotNil(t, fileModel.Audio)
	assert.InDelta(t, 10.0, fileModel.Audio.PromptTokensPerSecond, 1e-12)
	assert.InDelta(t, 0.0045, fileModel.Audio.UsdPerSecond*60.0, 1e-12)
	assert.Equal(t, []string{"audio", "text"}, fileModel.InputModalities)
	assert.Equal(t, []string{"text"}, fileModel.OutputModalities)

	liveModel, ok := ModelRatios["gpt-live-transcribe"]
	require.True(t, ok, "ModelRatios must contain gpt-live-transcribe")
	assert.True(t, modelSet["gpt-live-transcribe"])
	require.NotNil(t, liveModel.Audio)
	assert.InDelta(t, 0.017, liveModel.Audio.UsdPerSecond*60.0, 1e-12)
	assert.Equal(t, []string{"audio", "text"}, liveModel.InputModalities)
	assert.Equal(t, []string{"text"}, liveModel.OutputModalities)
}
