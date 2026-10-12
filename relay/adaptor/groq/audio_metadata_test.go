package groq

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGroqAudioModelParameters checks discoverable audio controls and modalities using t and returns no value.
func TestGroqAudioModelParameters(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		modelID    string
		parameters []string
		input      []string
		output     []string
	}{
		{
			modelID:    "whisper-large-v3",
			parameters: []string{"language", "prompt", "response_format", "temperature", "timestamp_granularities"},
			input:      []string{"audio"},
			output:     []string{"text"},
		},
		{
			modelID:    "whisper-large-v3-turbo",
			parameters: []string{"language", "prompt", "response_format", "temperature", "timestamp_granularities"},
			input:      []string{"audio"},
			output:     []string{"text"},
		},
		{
			modelID:    "canopylabs/orpheus-arabic-saudi",
			parameters: []string{"voice", "response_format"},
			input:      []string{"text"},
			output:     []string{"audio"},
		},
		{
			modelID:    "canopylabs/orpheus-v1-english",
			parameters: []string{"voice", "response_format"},
			input:      []string{"text"},
			output:     []string{"audio"},
		},
	} {
		t.Run(tc.modelID, func(t *testing.T) {
			t.Parallel()

			a := &Adaptor{}
			require.Contains(t, a.GetModelList(), tc.modelID)
			config, ok := a.GetDefaultModelPricing()[tc.modelID]
			require.True(t, ok)
			require.ElementsMatch(t, tc.parameters, config.SupportedSamplingParameters)
			require.Equal(t, tc.input, config.InputModalities)
			require.Equal(t, tc.output, config.OutputModalities)
			require.Empty(t, config.SupportedReasoningEfforts)
			require.NotContains(t, config.SupportedFeatures, "tools")
			require.NotContains(t, config.SupportedFeatures, "structured_outputs")
		})
	}
}

// TestGroqAudioMetadataConstraints checks endpoint caveats without conflating billing or token units using t and returns no value.
func TestGroqAudioMetadataConstraints(t *testing.T) {
	t.Parallel()

	for modelID, caveats := range map[string][]string{
		"whisper-large-v3": {
			"translation to English", "word/segment timestamps", "verbose_json", "10-second minimum",
		},
		"whisper-large-v3-turbo": {
			"not translation", "word/segment timestamps", "verbose_json", "10-second minimum",
		},
		"canopylabs/orpheus-arabic-saudi": {
			"Saudi Arabic voice", "200-character maximum input", "WAV audio only", "vocal directions are not supported",
		},
		"canopylabs/orpheus-v1-english": {
			"English voice", "200-character maximum input", "WAV audio only", "bracketed vocal direction tags",
		},
	} {
		t.Run(modelID, func(t *testing.T) {
			t.Parallel()

			config, ok := (&Adaptor{}).GetDefaultModelPricing()[modelID]
			require.True(t, ok)
			for _, caveat := range caveats {
				require.Contains(t, config.Description, caveat)
			}
			require.NotNil(t, config.Audio)
			if config.Audio.InputUnit == "characters" {
				// The catalog publishes token limits separately from the 200-character API limit.
				require.EqualValues(t, 4000, config.ContextLength)
				require.EqualValues(t, 50000, config.MaxOutputTokens)
				require.EqualValues(t, 1_000_000, config.Audio.InputPriceQuantity)
			} else {
				require.Equal(t, "seconds", config.Audio.InputUnit)
				require.EqualValues(t, 3600, config.Audio.InputPriceQuantity)
				require.EqualValues(t, 10, config.Audio.MinimumBillableSeconds)
				require.Zero(t, config.Ratio)
			}
		})
	}
}

// TestGroqAudioParametersStayModelScoped checks all current and retained model controls using t and returns no value.
func TestGroqAudioParametersStayModelScoped(t *testing.T) {
	t.Parallel()

	for modelID, config := range (&Adaptor{}).GetDefaultModelPricing() {
		t.Run(modelID, func(t *testing.T) {
			t.Parallel()

			if modelID != "whisper-large-v3" && modelID != "whisper-large-v3-turbo" {
				require.NotContains(t, config.SupportedSamplingParameters, "timestamp_granularities")
			}
			if modelID != "canopylabs/orpheus-arabic-saudi" && modelID != "canopylabs/orpheus-v1-english" {
				require.NotContains(t, config.SupportedSamplingParameters, "voice")
			}
		})
	}
}
