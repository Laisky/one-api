package quota

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/model"
	"math"
)

// nativeGoogleAudioSurcharge replaces only the audio portion of the normal
// prompt cost using the configured audio/text multiplier. Native Google Standard
// cached-audio and cached-text prices have the same multiplier. A private receipt
// gates this adjustment, so OpenAI, embeddings and realtime behavior is unchanged.
// Source: https://ai.google.dev/gemini-api/docs/pricing (2026-11-02).
func nativeGoogleAudioSurcharge(usage *model.Usage, cfg adaptor.ModelConfig, inputPrice, cachePrice float64, cached int) (float64, bool) {
	audio := usage.NativeGoogleAudio
	if audio == nil || cfg.Audio == nil || cfg.Audio.PromptRatio == 0 {
		return 0, false
	}
	multiplier := cfg.Audio.PromptRatio
	if multiplier < 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || audio.Tokens < 0 || audio.Tokens > usage.PromptTokens || audio.CachedTokens < 0 || audio.CachedTokens > audio.Tokens || audio.CachedTokens > cached {
		return 0, true
	}
	if cached > 0 && !audio.CachedKnown {
		// Keep the original admission floor on missing cache evidence. Do not
		// guess which cached tokens were audio or make an uncertain receipt final.
		return 0, multiplier != 1
	}
	baseAudio := float64(audio.Tokens-audio.CachedTokens)*inputPrice + float64(audio.CachedTokens)*cachePrice
	return baseAudio * (multiplier - 1), false
}
