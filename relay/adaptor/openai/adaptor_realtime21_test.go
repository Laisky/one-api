package openai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// gpt-realtime-2.1 and gpt-realtime-2.1-mini were added from the live OpenAI
// catalog (2026-07-10). This locks in their presence and token pricing so a
// future refactor of the realtime map cannot silently drop or mis-price them.
func TestRealtime21ModelsPricing(t *testing.T) {
	t.Parallel()

	modelList := adaptor.GetModelListFromPricing(ModelRatios)
	modelSet := make(map[string]bool, len(modelList))
	for _, m := range modelList {
		modelSet[m] = true
	}

	type expectation struct {
		textRatio        float64 // text input $/1M
		textCompletion   float64 // text output/input multiplier
		textCached       float64 // text cached input $/1M
		audioPromptRatio float64 // audio input / text input multiplier
		audioCompletion  float64 // audio output / audio input multiplier
	}

	expectations := map[string]expectation{
		// text $4/$24 cached $0.40; audio $32/$64.
		"gpt-realtime-2.1": {4.0, 6.0, 0.4, 8.0, 2.0},
		// text $0.60/$2.40 cached $0.06; audio $10/$20 (prompt ratio = 10/0.6).
		"gpt-realtime-2.1-mini": {0.6, 4.0, 0.06, 10.0 / 0.6, 2.0},
	}

	for name, exp := range expectations {
		assert.Truef(t, modelSet[name], "model list must include %q", name)

		cfg, ok := ModelRatios[name]
		require.Truef(t, ok, "ModelRatios must contain %q", name)

		assert.InDeltaf(t, exp.textRatio*ratio.MilliTokensUsd, cfg.Ratio, 1e-12,
			"%s text input ratio", name)
		assert.InDeltaf(t, exp.textCompletion, cfg.CompletionRatio, 1e-9,
			"%s text completion ratio", name)
		assert.InDeltaf(t, exp.textCached*ratio.MilliTokensUsd, cfg.CachedInputRatio, 1e-12,
			"%s text cached input ratio", name)

		require.NotNilf(t, cfg.Audio, "%s must carry an audio pricing config", name)
		assert.InDeltaf(t, exp.audioPromptRatio, cfg.Audio.PromptRatio, 1e-9,
			"%s audio prompt ratio", name)
		assert.InDeltaf(t, exp.audioCompletion, cfg.Audio.CompletionRatio, 1e-9,
			"%s audio completion ratio", name)

		assert.Equalf(t, int32(128000), cfg.ContextLength, "%s context length", name)
		assert.Equalf(t, int32(32000), cfg.MaxOutputTokens, "%s max output tokens", name)
	}
}
